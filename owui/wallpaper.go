package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	ndl "github.com/mechakotik/ndl/lib"
)

type processOutput struct {
	mu           sync.Mutex
	buffer       bytes.Buffer
	nextListener int
	listeners    map[int]func(string)
}

type wallpaperProcess struct {
	pid           int
	display       string
	path          string
	output        *processOutput
	ignored       atomic.Bool
	logDialogOpen atomic.Bool
}

func autorun() {
	currentSettings := loadSettings()
	_ = exec.Command("killall", "wallpaperd").Run()

	for display, path := range currentSettings.AutorunWallpapers {
		runWallpaper(nil, currentSettings, path, display)
	}
}

func runWallpaper(state *appState, currentSettings settings, path string, display string) {
	launchPath := wallpaperLaunchTarget(path)
	if launchPath == "" {
		fmt.Fprintf(os.Stderr, "unrecognized wallpaper path: %s\n", path)
		return
	}

	oldPIDs := getRunningPids(display)
	args := wallpaperdArgs(currentSettings, launchPath, display)

	output := &processOutput{}
	fmt.Fprintf(output, "> %s\n", commandLine("wallpaperd", args))
	child := exec.Command("wallpaperd", args...)
	child.Stdout = output
	child.Stderr = output
	if err := child.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start wallpaperd: %v\n", err)
		reportWallpaperdFailure(state, display, err, output.String())
		return
	}

	process := &wallpaperProcess{
		pid:     child.Process.Pid,
		display: display,
		path:    launchPath,
		output:  output,
	}
	if state != nil {
		state.trackWallpaperProcess(process)
	}

	done := make(chan error, 1)
	go func() {
		err := child.Wait()
		done <- err
		handleWallpaperProcessExit(state, process, err)
	}()

	readyFile := filepath.Join(os.TempDir(), fmt.Sprintf("wallpaperd-%d.ready", child.Process.Pid))
waitReady:
	for {
		if _, err := os.Stat(readyFile); err == nil {
			break
		}

		select {
		case err := <-done:
			if err != nil {
				return
			}
			break waitReady
		default:
		}

		time.Sleep(100 * time.Millisecond)
	}

	if state != nil {
		state.ignoreWallpaperPids(oldPIDs)
	}
	for _, pid := range oldPIDs {
		_ = exec.Command("kill", "-TERM", pid).Run()
	}
}

func stopWallpaper(state *appState, display string) {
	pids := getRunningPids(display)
	if state != nil {
		state.ignoreWallpaperPids(pids)
	}
	for _, pid := range pids {
		_ = exec.Command("kill", "-TERM", pid).Run()
	}
}

func wallpaperdArgs(currentSettings settings, path string, display string) []string {
	args := []string{
		"--owui-tag",
		fmt.Sprintf("--display=%s", display),
	}
	options := wallpaperOptionsForPath(currentSettings, path)

	if currentSettings.PauseHidden {
		args = append(args, "--pause-hidden")
	}
	if currentSettings.PauseOnBat {
		args = append(args, "--pause-on-bat")
	}
	if options.Speed != 1.0 {
		args = append(args, fmt.Sprintf("--speed=%s", formatSpeed(options.Speed)))
	}

	if isSceneFile(path) {
		sceneOptions := sceneOptionsForPath(currentSettings, path)
		if !sceneOptions.VSync {
			args = append(args, fmt.Sprintf("--fps=%d", sceneOptions.FPSLimit))
		}
		if sceneOptions.PreferDiscreteGPU {
			args = append(args, "--prefer-dgpu")
		}
		if !sceneOptions.AudioVisualization {
			args = append(args, "--no-audio")
		} else {
			switch sceneOptions.AudioBackend {
			case 1:
				args = append(args, "--audio-backend=pipewire")
			case 2:
				args = append(args, "--audio-backend=pulse")
			case 3:
				args = append(args, "--audio-backend=portaudio")
			}
			if strings.TrimSpace(sceneOptions.AudioSource) != "" {
				args = append(args, fmt.Sprintf("--audio-source=%s", sceneOptions.AudioSource))
			}
		}
	}

	if isVideoFile(path) {
		videoOptions := videoOptionsForPath(currentSettings, path)
		scaleMode := normalizedScaleMode(videoOptions.ScaleMode)
		if scaleMode != "aspect-crop" {
			args = append(args, fmt.Sprintf("--scale-mode=%s", scaleMode))
		}
		if strings.TrimSpace(videoOptions.Filter) != "" {
			args = append(args, fmt.Sprintf("--filter=%s", videoOptions.Filter))
		}
	}

	args = append(args, path)
	return args
}

func getRunningPids(display string) []string {
	pattern := fmt.Sprintf("wallpaperd --owui-tag --display=%s", display)
	output, _ := exec.Command("pgrep", "-f", pattern).Output()

	return nonEmptyLines(output)
}

func (state *appState) trackWallpaperProcess(process *wallpaperProcess) {
	state.processMu.Lock()
	if state.processes == nil {
		state.processes = map[int]*wallpaperProcess{}
	}
	state.processes[process.pid] = process
	state.processMu.Unlock()
	state.notifyDisplayMappingsChangedLater()
}

func (state *appState) processForDisplayMapping(display string, path string) *wallpaperProcess {
	state.processMu.Lock()
	defer state.processMu.Unlock()

	for _, process := range state.processes {
		if process.display == display && process.path == path && !process.ignored.Load() {
			return process
		}
	}
	return nil
}

func (state *appState) ignoreWallpaperPids(pids []string) {
	state.processMu.Lock()
	defer state.processMu.Unlock()

	for _, pidText := range pids {
		pid, err := strconv.Atoi(strings.TrimSpace(pidText))
		if err != nil {
			continue
		}
		if process := state.processes[pid]; process != nil {
			process.ignored.Store(true)
		}
	}
}

func (state *appState) disownWallpaperProcesses() {
	state.exiting.Store(true)

	state.processMu.Lock()
	defer state.processMu.Unlock()
	for _, process := range state.processes {
		process.ignored.Store(true)
	}
}

func handleWallpaperProcessExit(state *appState, process *wallpaperProcess, err error) {
	process.output.AppendExitCode(err)

	if state != nil {
		state.untrackWallpaperProcess(process.pid)
	}
	if err == nil || process.ignored.Load() || process.logDialogOpen.Load() {
		return
	}
	if state == nil || state.exiting.Load() {
		return
	}

	reportWallpaperdFailure(state, process.display, err, process.output.String())
}

func (state *appState) untrackWallpaperProcess(pid int) {
	state.processMu.Lock()
	delete(state.processes, pid)
	state.processMu.Unlock()
	state.notifyDisplayMappingsChangedLater()
}

func (state *appState) notifyDisplayMappingsChangedLater() {
	if state == nil {
		return
	}

	glib.IdleAdd(func() {
		if !state.exiting.Load() {
			state.notifyDisplayMappingsChanged()
		}
	})
}

func reportWallpaperdFailure(state *appState, display string, err error, output string) {
	if state == nil {
		fmt.Fprintf(os.Stderr, "wallpaperd failed on %s: %v\n%s\n", display, err, output)
		return
	}

	glib.IdleAdd(func() {
		if state.exiting.Load() || state.dialogParent == nil {
			return
		}
		dialog, _ := rendererLogDialog(fmt.Sprintf("%s renderer crashed", display), output)
		dialog.Present(state.dialogParent)
	})
}

func showRendererLogsDialog(parent gtk.Widgetter, process *wallpaperProcess) {
	if process.logDialogOpen.Swap(true) {
		return
	}

	dialog, textView := rendererLogDialog(fmt.Sprintf("%s renderer logs", process.display), "")
	closed := atomic.Bool{}
	text, unsubscribe := process.output.Subscribe(func(text string) {
		glib.IdleAdd(func() {
			if !closed.Load() {
				setLogText(textView, text)
			}
		})
	})
	setLogText(textView, text)

	dialog.ConnectResponse(func(response string) {
		closed.Store(true)
		unsubscribe()
		process.logDialogOpen.Store(false)
	})
	dialog.Present(parent)
}

func rendererLogDialog(title string, output string) (*adw.AlertDialog, *gtk.TextView) {
	output = strings.TrimRight(output, "\n")
	if output == "" {
		output = "(no output)"
	}

	dialog := adw.NewAlertDialog(title, "")
	dialog.AddResponse("close", "Close")
	dialog.SetDefaultResponse("close")
	dialog.SetCloseResponse("close")

	textView := gtk.NewTextView()
	textView.SetEditable(false)
	textView.SetMonospace(true)
	textView.SetWrapMode(gtk.WrapWordChar)
	textView.SetTopMargin(12)
	textView.SetBottomMargin(12)
	textView.SetLeftMargin(12)
	textView.SetRightMargin(12)
	setLogText(textView, output)

	scrolled := gtk.NewScrolledWindow()
	scrolled.SetSizeRequest(640, 320)
	scrolled.SetPolicy(gtk.PolicyAutomatic, gtk.PolicyAutomatic)
	scrolled.SetChild(textView)
	dialog.SetExtraChild(scrolled)
	return dialog, textView
}

func setLogText(textView *gtk.TextView, output string) {
	output = strings.TrimRight(output, "\n")
	if output == "" {
		output = "(no output)"
	}
	textView.Buffer().SetText(output)
}

func (output *processOutput) Write(chunk []byte) (int, error) {
	output.mu.Lock()
	written, err := output.buffer.Write(chunk)
	text := output.buffer.String()
	listeners := output.snapshotListenersLocked()
	output.mu.Unlock()

	output.notifyListeners(listeners, text)
	return written, err
}

func (output *processOutput) String() string {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.buffer.String()
}

func (output *processOutput) Subscribe(listener func(string)) (string, func()) {
	output.mu.Lock()
	if output.listeners == nil {
		output.listeners = map[int]func(string){}
	}
	id := output.nextListener
	output.nextListener++
	output.listeners[id] = listener
	text := output.buffer.String()
	output.mu.Unlock()

	return text, func() {
		output.mu.Lock()
		defer output.mu.Unlock()
		delete(output.listeners, id)
	}
}

func (output *processOutput) AppendExitCode(err error) {
	code, ok := exitCode(err)
	if !ok {
		return
	}

	output.mu.Lock()
	if output.buffer.Len() > 0 {
		bytes := output.buffer.Bytes()
		if bytes[len(bytes)-1] != '\n' {
			output.buffer.WriteByte('\n')
		}
	}
	fmt.Fprintf(&output.buffer, "[exit code %d]\n", code)
	text := output.buffer.String()
	listeners := output.snapshotListenersLocked()
	output.mu.Unlock()

	output.notifyListeners(listeners, text)
}

func exitCode(err error) (int, bool) {
	if err == nil {
		return 0, true
	}
	exitError, ok := err.(*exec.ExitError)
	if !ok {
		return 0, false
	}
	return exitError.ExitCode(), true
}

func (output *processOutput) snapshotListenersLocked() []func(string) {
	listeners := make([]func(string), 0, len(output.listeners))
	for _, listener := range output.listeners {
		listeners = append(listeners, listener)
	}
	return listeners
}

func (output *processOutput) notifyListeners(listeners []func(string), text string) {
	for _, listener := range listeners {
		listener(text)
	}
}

func commandLine(command string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, command)
	for _, arg := range args {
		parts = append(parts, quoteCommandArg(arg))
	}
	return strings.Join(parts, " ")
}

func quoteCommandArg(arg string) string {
	if arg == "" {
		return "''"
	}
	if strings.ContainsAny(arg, " \t\n\"'\\$`!#&*()[]{};<>?|") {
		return strconv.Quote(arg)
	}
	return arg
}

func loadWallpapers() []wallpaper {
	dataDir := dataPath()
	_ = os.MkdirAll(dataDir, 0o755)

	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return nil
	}

	wallpapers := make([]wallpaper, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		wallpaperPath := filepath.Join(dataDir, entry.Name())
		launchPath := wallpaperLaunchTarget(wallpaperPath)
		if launchPath == "" {
			continue
		}

		wallpapers = append(wallpapers, loadWallpaper(wallpaperPath, launchPath, entry.Name()))
	}

	sort.Slice(wallpapers, func(left, right int) bool {
		return strings.ToLower(wallpapers[left].title) < strings.ToLower(wallpapers[right].title)
	})

	return wallpapers
}

func wallpaperLaunchTarget(path string) string {
	if regularFileExists(path) && (isSceneFile(path) || isVideoFile(path)) {
		return path
	}

	scenePath := filepath.Join(path, "scene.wasm")
	if regularFileExists(scenePath) {
		return scenePath
	}

	videoPath := filepath.Join(path, "video.mp4")
	if regularFileExists(videoPath) {
		return videoPath
	}
	return ""
}

func isSceneFile(path string) bool {
	return filepath.Base(path) == "scene.wasm"
}

func isVideoFile(path string) bool {
	return filepath.Base(path) == "video.mp4"
}

func loadWallpaper(path string, launchPath string, fallbackTitle string) wallpaper {
	title, description := loadWallpaperMetadata(path, fallbackTitle)
	return wallpaper{
		title:       title,
		description: description,
		path:        path,
		launchPath:  launchPath,
		previewPath: previewPath(path),
	}
}

func loadWallpaperMetadata(path string, fallbackTitle string) (string, string) {
	title := fallbackTitle
	description := ""

	data, err := os.ReadFile(filepath.Join(path, "metadata.ndl"))
	if err != nil {
		return title, description
	}

	metadata := wallpaperMetadata{}
	if err := ndl.Unmarshal(string(data), &metadata); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse wallpaper metadata %s: %v\n", filepath.Join(path, "metadata.ndl"), err)
		return title, description
	}

	if strings.TrimSpace(metadata.Title) != "" {
		title = metadata.Title
	}
	description = metadata.Description
	return title, description
}

func previewPath(path string) string {
	preview := filepath.Join(path, "preview.webp")
	if regularFileExists(preview) {
		return preview
	}
	return ""
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func loadDisplays() []string {
	output, err := exec.Command("wallpaperd", "--list-displays").Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to start wallpaperd --list-displays")
		return nil
	}

	return nonEmptyLines(output)
}

func nonEmptyLines(output []byte) []string {
	lines := strings.Split(string(output), "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}
