package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
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

type wallpaperEngineProject struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	File        string `json:"file"`
	Preview     string `json:"preview"`
	Type        string `json:"type"`
}

type wallpaperEngineObject struct {
	Index   int                     `ndl:"index"`
	ID      int                     `ndl:"id"`
	Parent  int                     `ndl:"parent"`
	Type    string                  `ndl:"type"`
	Name    string                  `ndl:"name"`
	Effects []wallpaperEngineEffect `ndl:"effects"`
}

type wallpaperEngineEffect struct {
	Index  int    `ndl:"index"`
	ID     string `ndl:"id"`
	Name   string `ndl:"name"`
	Passes int    `ndl:"passes"`
}

type wallpaperEngineImportOptions struct {
	skipObjects     string
	skipEffects     string
	hiddenObjectIDs []int
	hiddenEffects   map[int][]string
}

type importEffectControl struct {
	effect wallpaperEngineEffect
	check  *gtk.CheckButton
}

type importObjectNode struct {
	object   wallpaperEngineObject
	check    *gtk.CheckButton
	effects  []importEffectControl
	children []*importObjectNode
}

var compileProgressPattern = regexp.MustCompile(`\[(\d+)/(\d+)\]`)

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
	args := wallpaperdArgs(currentSettings, launchPath, path, display)

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
		path:    path,
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

func wallpaperdArgs(currentSettings settings, launchPath string, settingsPath string, display string) []string {
	args := []string{
		"--owui-tag",
		fmt.Sprintf("--display=%s", display),
	}
	options := wallpaperOptionsForPath(currentSettings, settingsPath)

	if currentSettings.PauseHidden {
		args = append(args, "--pause-hidden")
	}
	if currentSettings.PauseOnBat {
		args = append(args, "--pause-on-bat")
	}
	if options.Speed != 1.0 {
		args = append(args, fmt.Sprintf("--speed=%s", formatSpeed(options.Speed)))
	}

	if isSceneFile(launchPath) {
		sceneOptions := sceneOptionsForPath(currentSettings, settingsPath)
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

	if isVideoFile(launchPath) {
		videoOptions := videoOptionsForPath(currentSettings, settingsPath)
		scaleMode := normalizedScaleMode(videoOptions.ScaleMode)
		if scaleMode != "aspect-crop" {
			args = append(args, fmt.Sprintf("--scale-mode=%s", scaleMode))
		}
		if strings.TrimSpace(videoOptions.Filter) != "" {
			args = append(args, fmt.Sprintf("--filter=%s", videoOptions.Filter))
		}
	}

	args = append(args, launchPath)
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

func showWallpaperEngineImportOptions(parent gtk.Widgetter, wallpaper wallpaper, savedOptions wallpaperOptions, done func(*wallpaperEngineImportOptions)) {
	go func() {
		objects, logText, err := loadWallpaperEngineObjects(wallpaper)
		glib.IdleAdd(func() {
			if err != nil {
				dialog, _ := rendererLogDialog("Import failed", logText)
				dialog.Present(parent)
				done(nil)
				return
			}
			presentWallpaperEngineImportOptions(parent, objects, savedOptions, done)
		})
	}()
}

func loadWallpaperEngineObjects(wallpaper wallpaper) ([]wallpaperEngineObject, string, error) {
	args := []string{"--list-objects-ndl", wallpaper.path}
	output := &processOutput{}
	fmt.Fprintf(output, "> WPE_COMPILE_ASSETS=%s %s\n", quoteCommandArg(wallpaper.assetsDir), commandLine("wpe-compile", args))

	command := exec.Command("wpe-compile", args...)
	command.Env = append(os.Environ(), "WPE_COMPILE_ASSETS="+wallpaper.assetsDir)
	raw, err := command.CombinedOutput()
	_, _ = output.Write(raw)
	if err != nil {
		output.AppendExitCode(err)
		return nil, output.String(), err
	}

	objects := []wallpaperEngineObject{}
	if err := ndl.Unmarshal(string(raw), &objects); err != nil {
		fmt.Fprintf(output, "error: parse object list failed: %v\n", err)
		return nil, output.String(), err
	}
	return objects, output.String(), nil
}

func presentWallpaperEngineImportOptions(parent gtk.Widgetter, objects []wallpaperEngineObject, savedOptions wallpaperOptions, done func(*wallpaperEngineImportOptions)) {
	dialog := adw.NewAlertDialog("Select objects and effects", "")
	dialog.AddResponse("cancel", "Cancel")
	dialog.AddResponse("import", "Import")
	dialog.SetDefaultResponse("import")
	dialog.SetCloseResponse("cancel")
	dialog.SetResponseAppearance("import", adw.ResponseSuggested)
	dialog.SetPreferWideLayout(true)

	list, roots := importOptionsList(objects, savedOptions)
	dialog.SetExtraChild(list)
	dialog.ConnectResponse(func(response string) {
		if response != "import" {
			done(nil)
			return
		}
		options := importOptionsFromTree(roots)
		done(&options)
	})
	dialog.Present(parent)
}

func importOptionsList(objects []wallpaperEngineObject, savedOptions wallpaperOptions) (gtk.Widgetter, []*importObjectNode) {
	list := boxedList()
	list.SetSizeRequest(520, -1)

	roots := importObjectTree(objects)
	hiddenObjectIDs := intSet(savedOptions.HiddenObjectIDs)
	hiddenEffects := savedOptions.HiddenEffects
	for _, root := range roots {
		appendImportObjectRows(list, root, 0, hiddenObjectIDs, hiddenEffects)
	}
	connectImportObjectToggles(roots)
	updateImportObjectSensitivity(roots)

	if len(objects) == 0 {
		label := gtk.NewLabel("No objects")
		label.SetXAlign(0)
		list.Append(plainWidgetRow(rowContent(label)))
	}

	scrolled := gtk.NewScrolledWindow()
	scrolled.SetMinContentHeight(260)
	scrolled.SetMaxContentHeight(420)
	scrolled.SetMinContentWidth(520)
	scrolled.SetPropagateNaturalHeight(true)
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scrolled.SetChild(list)
	return scrolled, roots
}

func importObjectTree(objects []wallpaperEngineObject) []*importObjectNode {
	nodes := make([]*importObjectNode, 0, len(objects))
	nodesByID := map[int]*importObjectNode{}
	for _, object := range objects {
		node := &importObjectNode{object: object}
		nodes = append(nodes, node)
		nodesByID[object.ID] = node
	}

	roots := []*importObjectNode{}
	for _, node := range nodes {
		parent := nodesByID[node.object.Parent]
		if parent == nil || parent == node {
			roots = append(roots, node)
		} else {
			parent.children = append(parent.children, node)
		}
	}
	return roots
}

func appendImportObjectRows(list *gtk.ListBox, node *importObjectNode, indent int, hiddenObjectIDs map[int]bool, hiddenEffects map[int][]string) {
	objectCheck := gtk.NewCheckButton()
	objectCheck.SetActive(!hiddenObjectIDs[node.object.ID])
	node.check = objectCheck
	list.Append(importCheckRow(objectCheck, objectLabel(node.object), indent))

	hiddenEffectIDs := stringSet(hiddenEffects[node.object.ID])
	for _, effect := range node.object.Effects {
		effectCheck := gtk.NewCheckButton()
		effectCheck.SetActive(!hiddenEffectIDs[wallpaperEngineEffectID(effect)])
		node.effects = append(node.effects, importEffectControl{
			effect: effect,
			check:  effectCheck,
		})
		list.Append(importCheckRow(effectCheck, effectLabel(effect), indent+32))
	}

	for _, child := range node.children {
		appendImportObjectRows(list, child, indent+32, hiddenObjectIDs, hiddenEffects)
	}
}

func connectImportObjectToggles(roots []*importObjectNode) {
	for _, root := range roots {
		connectImportObjectToggle(root, roots)
	}
}

func connectImportObjectToggle(node *importObjectNode, roots []*importObjectNode) {
	node.check.ConnectToggled(func() {
		updateImportObjectSensitivity(roots)
	})
	for _, child := range node.children {
		connectImportObjectToggle(child, roots)
	}
}

func updateImportObjectSensitivity(roots []*importObjectNode) {
	for _, root := range roots {
		updateImportObjectNodeSensitivity(root, true)
	}
}

func updateImportObjectNodeSensitivity(node *importObjectNode, parentActive bool) {
	node.check.SetSensitive(parentActive)
	active := parentActive && node.check.Active()
	for _, effect := range node.effects {
		effect.check.SetSensitive(active)
	}
	for _, child := range node.children {
		updateImportObjectNodeSensitivity(child, active)
	}
}

func importCheckRow(check *gtk.CheckButton, labelText string, indent int) *gtk.ListBoxRow {
	label := gtk.NewLabel(labelText)
	label.SetXAlign(0)
	label.SetWrap(true)
	label.SetHExpand(true)

	content := rowContent(check, label)
	if indent > 0 {
		content.SetMarginStart(12 + indent)
	}
	return plainWidgetRow(content)
}

func objectLabel(object wallpaperEngineObject) string {
	name := strings.TrimSpace(object.Name)
	if name == "" {
		name = "(unnamed)"
	}
	return fmt.Sprintf("Object %d: %s (%s)", object.Index, name, object.Type)
}

func effectLabel(effect wallpaperEngineEffect) string {
	name := strings.TrimSpace(effect.Name)
	if name == "" {
		name = "(unnamed)"
	}
	return fmt.Sprintf("Effect %d: %s", effect.Index, name)
}

func importOptionsFromTree(roots []*importObjectNode) wallpaperEngineImportOptions {
	skippedObjects := []string{}
	effectRules := []string{}
	hiddenObjectIDs := []int{}
	hiddenEffects := map[int][]string{}
	for _, root := range roots {
		appendImportOptionsFromNode(root, true, &skippedObjects, &effectRules, &hiddenObjectIDs, hiddenEffects)
	}

	return wallpaperEngineImportOptions{
		skipObjects:     strings.Join(skippedObjects, ","),
		skipEffects:     strings.Join(effectRules, ";"),
		hiddenObjectIDs: normalizeIntList(hiddenObjectIDs),
		hiddenEffects:   normalizeHiddenEffects(hiddenEffects),
	}
}

func appendImportOptionsFromNode(node *importObjectNode, parentActive bool, skippedObjects *[]string, effectRules *[]string, hiddenObjectIDs *[]int, hiddenEffects map[int][]string) {
	active := parentActive && node.check.Active()
	if !active {
		if parentActive {
			*skippedObjects = append(*skippedObjects, strconv.Itoa(node.object.Index))
			*hiddenObjectIDs = append(*hiddenObjectIDs, node.object.ID)
		}
		return
	}

	skippedEffects := []string{}
	for _, effectControl := range node.effects {
		if !effectControl.check.Active() {
			skippedEffects = append(skippedEffects, strconv.Itoa(effectControl.effect.Index))
			hiddenEffects[node.object.ID] = append(hiddenEffects[node.object.ID], wallpaperEngineEffectID(effectControl.effect))
		}
	}
	if len(skippedEffects) > 0 {
		*effectRules = append(*effectRules, fmt.Sprintf("%d:%s", node.object.Index, strings.Join(skippedEffects, ",")))
	}

	for _, child := range node.children {
		appendImportOptionsFromNode(child, active, skippedObjects, effectRules, hiddenObjectIDs, hiddenEffects)
	}
}

func wallpaperEngineImportOptionsFromSavedSettings(wallpaper wallpaper, savedOptions wallpaperOptions) (wallpaperEngineImportOptions, string, error) {
	if len(savedOptions.HiddenObjectIDs) == 0 && len(savedOptions.HiddenEffects) == 0 {
		return wallpaperEngineImportOptions{}, "", nil
	}

	objects, logText, err := loadWallpaperEngineObjects(wallpaper)
	if err != nil {
		return wallpaperEngineImportOptions{}, logText, err
	}
	return importOptionsFromSavedSettings(objects, savedOptions), "", nil
}

func importOptionsFromSavedSettings(objects []wallpaperEngineObject, savedOptions wallpaperOptions) wallpaperEngineImportOptions {
	hiddenObjectIDs := intSet(savedOptions.HiddenObjectIDs)
	hiddenEffects := savedOptions.HiddenEffects
	skippedObjects := []string{}
	effectRules := []string{}

	for _, object := range objects {
		if hiddenObjectIDs[object.ID] {
			skippedObjects = append(skippedObjects, strconv.Itoa(object.Index))
			continue
		}

		hiddenEffectIDs := stringSet(hiddenEffects[object.ID])
		skippedEffects := []string{}
		for _, effect := range object.Effects {
			if hiddenEffectIDs[wallpaperEngineEffectID(effect)] {
				skippedEffects = append(skippedEffects, strconv.Itoa(effect.Index))
			}
		}
		if len(skippedEffects) > 0 {
			effectRules = append(effectRules, fmt.Sprintf("%d:%s", object.Index, strings.Join(skippedEffects, ",")))
		}
	}

	return wallpaperEngineImportOptions{
		skipObjects:     strings.Join(skippedObjects, ","),
		skipEffects:     strings.Join(effectRules, ";"),
		hiddenObjectIDs: savedOptions.HiddenObjectIDs,
		hiddenEffects:   savedOptions.HiddenEffects,
	}
}

func intSet(values []int) map[int]bool {
	set := map[int]bool{}
	for _, value := range values {
		set[value] = true
	}
	return set
}

func stringSet(values []string) map[string]bool {
	set := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = true
		}
	}
	return set
}

func wallpaperEngineEffectID(effect wallpaperEngineEffect) string {
	if id := strings.TrimSpace(effect.ID); id != "" {
		return id
	}
	return strconv.Itoa(effect.Index)
}

func importWallpaperEngineScene(parent gtk.Widgetter, wallpaper wallpaper, options wallpaperEngineImportOptions, done func(error)) {
	output := &processOutput{}

	tempDir, err := createTemporaryImportDir(wallpaper.importDir)
	if err != nil {
		fmt.Fprintf(output, "error: create temporary import directory failed: %v\n", err)
		dialog, _ := rendererLogDialog("Importing scene", output.String())
		dialog.Present(parent)
		if done != nil {
			done(err)
		}
		return
	}

	args := wallpaperEngineImportArgs(wallpaper, tempDir, options)
	fmt.Fprintf(output, "> WPE_COMPILE_ASSETS=%s %s\n", quoteCommandArg(wallpaper.assetsDir), commandLine("wpe-compile", args))

	importWindow := importLogDialog(output.String())
	importWindow.dialog.SetResponseLabel("close", "Abort")
	closed := atomic.Bool{}
	aborted := atomic.Bool{}
	finished := atomic.Bool{}
	command := exec.Command("wpe-compile", args...)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Env = append(os.Environ(), "WPE_COMPILE_ASSETS="+wallpaper.assetsDir)
	command.Stdout = output
	command.Stderr = output
	text, unsubscribe := output.Subscribe(func(text string) {
		glib.IdleAdd(func() {
			if !closed.Load() {
				importWindow.setOutput(text)
			}
		})
	})
	importWindow.setOutput(text)

	importWindow.dialog.ConnectResponse(func(response string) {
		closed.Store(true)
		unsubscribe()
		if !finished.Load() {
			aborted.Store(true)
			terminateCommand(command)
		}
	})

	if err := command.Start(); err != nil {
		finished.Store(true)
		_ = os.RemoveAll(tempDir)
		fmt.Fprintf(output, "error: %v\n", err)
		importWindow.finish(err, output.String())
		importWindow.dialog.Present(parent)
		if done != nil {
			done(err)
		}
		return
	}

	importWindow.dialog.Present(parent)
	go func() {
		err := command.Wait()
		finished.Store(true)

		if aborted.Load() {
			err = fmt.Errorf("import aborted")
		} else if err == nil {
			err = finishWallpaperEngineImport(tempDir, wallpaper.importDir)
		}

		if err != nil {
			_ = os.RemoveAll(tempDir)
			if !aborted.Load() {
				if _, ok := exitCode(err); !ok {
					fmt.Fprintf(output, "error: %v\n", err)
				}
			}
		}
		output.AppendExitCode(err)
		glib.IdleAdd(func() {
			if !closed.Load() {
				importWindow.finish(err, output.String())
			}
			if done != nil {
				done(err)
			}
		})
	}()
}

func wallpaperEngineImportArgs(wallpaper wallpaper, outputDir string, options wallpaperEngineImportOptions) []string {
	args := []string{}
	if options.skipObjects != "" {
		args = append(args, fmt.Sprintf("--skip-objects=%s", options.skipObjects))
	}
	if options.skipEffects != "" {
		args = append(args, fmt.Sprintf("--skip-effects=%s", options.skipEffects))
	}
	return append(args, wallpaper.path, outputDir)
}

func createTemporaryImportDir(finalDir string) (string, error) {
	parent := filepath.Dir(finalDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	return os.MkdirTemp(parent, "."+filepath.Base(finalDir)+".tmp-")
}

func terminateCommand(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}

	pid := command.Process.Pid
	if pid <= 0 {
		return
	}
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		_ = command.Process.Signal(syscall.SIGTERM)
	}
}

func finishWallpaperEngineImport(tempDir string, finalDir string) error {
	if wallpaperLaunchTarget(tempDir) == "" {
		return fmt.Errorf("import output does not contain a supported wallpaper")
	}
	return replaceDirectory(finalDir, tempDir)
}

func replaceDirectory(dst string, src string) error {
	backup := ""
	if _, err := os.Stat(dst); err == nil {
		var backupErr error
		backup, backupErr = temporarySiblingPath(dst, "old")
		if backupErr != nil {
			return backupErr
		}
		if err := os.Rename(dst, backup); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.Rename(src, dst); err != nil {
		if backup != "" {
			_ = os.Rename(backup, dst)
		}
		return err
	}
	if backup != "" {
		_ = os.RemoveAll(backup)
	}
	return nil
}

func temporarySiblingPath(path string, suffix string) (string, error) {
	tempPath, err := os.MkdirTemp(filepath.Dir(path), "."+filepath.Base(path)+"."+suffix+"-")
	if err != nil {
		return "", err
	}
	if err := os.Remove(tempPath); err != nil {
		return "", err
	}
	return tempPath, nil
}

type importDialog struct {
	dialog      *adw.AlertDialog
	textView    *gtk.TextView
	progressBar *gtk.ProgressBar
	details     *gtk.Expander
	progress    float64
}

func importLogDialog(output string) *importDialog {
	dialog := adw.NewAlertDialog("Importing scene", "")
	dialog.AddResponse("close", "Close")
	dialog.SetDefaultResponse("close")
	dialog.SetCloseResponse("close")

	progressBar := gtk.NewProgressBar()
	setImportProgress(progressBar, 0)

	textView := newLogTextView(output)

	details := gtk.NewExpander("Details")
	details.SetExpanded(false)
	details.SetResizeToplevel(false)
	details.SetChild(newLogScrolledWindow(textView))
	details.NotifyProperty("expanded", func() {
		if details.Expanded() {
			scrollLogToEnd(textView)
		}
	})

	content := gtk.NewBox(gtk.OrientationVertical, 12)
	content.SetMarginTop(6)
	content.SetMarginBottom(6)
	content.SetMarginStart(6)
	content.SetMarginEnd(6)
	content.Append(progressBar)
	content.Append(details)

	dialog.SetExtraChild(content)
	return &importDialog{
		dialog:      dialog,
		textView:    textView,
		progressBar: progressBar,
		details:     details,
	}
}

func (dialog *importDialog) setOutput(output string) {
	setLogText(dialog.textView, output)
	next := max(dialog.progress, parseCompileProgress(output))
	if next != dialog.progress {
		dialog.progress = next
		setImportProgress(dialog.progressBar, dialog.progress)
	}
}

func (dialog *importDialog) finish(err error, output string) {
	dialog.setOutput(output)
	if err == nil && dialog.progress < 1 {
		dialog.progress = 1
		setImportProgress(dialog.progressBar, dialog.progress)
	}
	if err == nil {
		dialog.dialog.SetHeading("Import successful")
	} else {
		dialog.dialog.SetHeading("Import failed")
	}
	dialog.dialog.SetResponseLabel("close", "Close")
	if err != nil {
		dialog.details.SetExpanded(true)
	} else if !dialog.details.Expanded() {
		dialog.dialog.Close()
	}
}

func setImportProgress(progressBar *gtk.ProgressBar, fraction float64) {
	fraction = min(max(fraction, 0), 1)
	progressBar.SetFraction(fraction)
}

func parseCompileProgress(output string) float64 {
	progress := 0.0
	for _, match := range compileProgressPattern.FindAllStringSubmatch(output, -1) {
		current, currentErr := strconv.Atoi(match[1])
		total, totalErr := strconv.Atoi(match[2])
		if currentErr != nil || totalErr != nil || total <= 0 {
			continue
		}

		fraction := float64(current-1) / float64(total)
		if fraction > progress {
			progress = fraction
		}
	}
	return min(progress, 1)
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

	textView := newLogTextView(output)

	dialog.SetExtraChild(newLogScrolledWindow(textView))
	return dialog, textView
}

func newLogScrolledWindow(textView *gtk.TextView) *gtk.ScrolledWindow {
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetSizeRequest(640, 320)
	scrolled.SetPolicy(gtk.PolicyAutomatic, gtk.PolicyAutomatic)
	scrolled.SetChild(textView)
	return scrolled
}

func newLogTextView(output string) *gtk.TextView {
	textView := gtk.NewTextView()
	textView.SetEditable(false)
	textView.SetMonospace(true)
	textView.SetWrapMode(gtk.WrapWordChar)
	textView.SetTopMargin(12)
	textView.SetBottomMargin(12)
	textView.SetLeftMargin(12)
	textView.SetRightMargin(12)
	setLogText(textView, output)
	return textView
}

func setLogText(textView *gtk.TextView, output string) {
	output = normalizeLogText(output)
	output = strings.TrimRight(output, "\n")
	if output == "" {
		output = "(no output)"
	}
	buffer := textView.Buffer()
	buffer.SetText(output)
	scrollLogToEnd(textView)
}

func scrollLogToEnd(textView *gtk.TextView) {
	glib.IdleAdd(func() {
		scrollLogAdjustmentToEnd(textView)
		glib.IdleAdd(func() {
			scrollLogAdjustmentToEnd(textView)
		})
	})
}

func scrollLogAdjustmentToEnd(textView *gtk.TextView) {
	adjustment := textView.VAdjustment()
	if adjustment == nil {
		return
	}

	value := adjustment.Upper() - adjustment.PageSize()
	if value < adjustment.Lower() {
		value = adjustment.Lower()
	}
	adjustment.SetValue(value)
}

func normalizeLogText(output string) string {
	input := []rune(output)
	normalized := make([]rune, 0, len(output))
	for index := 0; index < len(input); index++ {
		char := input[index]
		if char == '\r' {
			for len(normalized) > 0 && normalized[len(normalized)-1] != '\n' {
				normalized = normalized[:len(normalized)-1]
			}
			continue
		}
		if char == '\x1b' {
			index = skipANSIEscape(input, index)
			continue
		}
		normalized = append(normalized, char)
	}
	return string(normalized)
}

func skipANSIEscape(input []rune, index int) int {
	if index+1 >= len(input) || input[index+1] != '[' {
		return index
	}

	for index += 2; index < len(input); index++ {
		if input[index] >= 0x40 && input[index] <= 0x7e {
			return index
		}
	}
	return len(input) - 1
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

func loadWallpapers(currentSettings settings) []wallpaper {
	if userDir := userOwuiDir(); userDir != "" {
		_ = os.MkdirAll(filepath.Join(userDir, "local"), 0o755)
	}

	wallpapers := loadWallpaperEngineWallpapers(currentSettings)
	wallpapers = append(wallpapers, loadNativeWallpapers(currentWallpaperEngineImportDirs(wallpapers))...)

	sort.Slice(wallpapers, func(left, right int) bool {
		return strings.ToLower(wallpapers[left].title) < strings.ToLower(wallpapers[right].title)
	})

	return wallpapers
}

func currentWallpaperEngineImportDirs(wallpapers []wallpaper) map[string]bool {
	dirs := map[string]bool{}
	for _, wallpaper := range wallpapers {
		if wallpaper.kind == wallpaperEngineScene && wallpaper.importDir != "" {
			dirs[filepath.Clean(wallpaper.importDir)] = true
		}
	}
	return dirs
}

func loadNativeWallpapers(skipDirs map[string]bool) []wallpaper {
	wallpapers := []wallpaper{}
	for _, root := range owuiDirs() {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil || !entry.IsDir() {
				return nil
			}
			if path != root && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			if skipDirs[filepath.Clean(path)] {
				return filepath.SkipDir
			}

			launchPath := wallpaperLaunchTarget(path)
			if launchPath == "" {
				return nil
			}

			wallpapers = append(wallpapers, loadWallpaper(path, launchPath, filepath.Base(path)))
			return filepath.SkipDir
		})
		if err != nil {
			continue
		}
	}
	return wallpapers
}

func loadWallpaperEngineWallpapers(currentSettings settings) []wallpaper {
	steamLibrary := steamLibraryPath(currentSettings)
	workshopDir := filepath.Join(steamLibrary, "steamapps", "workshop", "content", "431960")
	assetsDir := wallpaperEngineAssetsDir(steamLibrary)
	entries, err := os.ReadDir(workshopDir)
	if err != nil {
		return nil
	}

	wallpapers := make([]wallpaper, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		wallpaperPath := filepath.Join(workshopDir, entry.Name())
		if wallpaper, ok := loadWallpaperEngineWallpaper(wallpaperPath, assetsDir); ok {
			wallpapers = append(wallpapers, wallpaper)
		}
	}
	return wallpapers
}

func loadWallpaperEngineWallpaper(path string, assetsDir string) (wallpaper, bool) {
	project, err := loadWallpaperEngineProject(filepath.Join(path, "project.json"))
	if err != nil {
		return wallpaper{}, false
	}

	workshopID := filepath.Base(path)
	title := project.Title
	if strings.TrimSpace(title) == "" {
		title = workshopID
	}

	previewPath, _ := inputAssetPath(path, project.Preview)

	switch strings.ToLower(project.Type) {
	case "scene":
		return wallpaper{
			title:       title,
			description: project.Description,
			path:        path,
			previewPath: previewPath,
			kind:        wallpaperEngineScene,
			importDir:   wallpaperEngineImportDir(workshopID),
			assetsDir:   assetsDir,
		}, true
	case "video":
		videoPath, err := inputAssetPath(path, project.File)
		if err != nil || !isVideoFile(videoPath) {
			return wallpaper{}, false
		}
		return wallpaper{
			title:       title,
			description: project.Description,
			path:        path,
			launchPath:  videoPath,
			previewPath: previewPath,
		}, true
	default:
		return wallpaper{}, false
	}
}

func loadWallpaperEngineProject(path string) (wallpaperEngineProject, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return wallpaperEngineProject{}, err
	}

	project := wallpaperEngineProject{}
	err = json.Unmarshal(data, &project)
	return project, err
}

func steamLibraryPath(currentSettings settings) string {
	if path := strings.TrimSpace(currentSettings.SteamLibraryPath); path != "" {
		return expandHomePath(path)
	}
	return defaultSteamLibraryPath()
}

func wallpaperEngineAssetsDir(steamLibrary string) string {
	return filepath.Join(steamLibrary, "steamapps", "common", "wallpaper_engine", "assets")
}

func defaultSteamLibraryPath() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "share", "Steam")
	}
	return filepath.Join(".", "Steam")
}

func expandHomePath(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func wallpaperEngineImportDir(workshopID string) string {
	if userDir := userOwuiDir(); userDir != "" {
		return filepath.Join(userDir, "wpe", workshopID)
	}
	return filepath.Join(".", "owui", "wpe", workshopID)
}

func inputAssetPath(root string, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("empty asset path")
	}
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("asset path %q is absolute", name)
	}

	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("asset path %q escapes wallpaper directory", name)
	}

	path := filepath.Join(root, clean)
	if !regularFileExists(path) {
		return "", fmt.Errorf("asset path %q is not a regular file", name)
	}
	return path, nil
}

func owuiDirs() []string {
	dirs := []string{}
	if userDir := userOwuiDir(); userDir != "" {
		dirs = append(dirs, userDir)
	}
	return append(dirs, "/usr/share/owui", "/usr/local/share/owui")
}

func userOwuiDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "share", "owui")
	}
	return ""
}

func wallpaperLaunchTarget(path string) string {
	scenePath := filepath.Join(path, "scene.wasm")
	if regularFileExists(scenePath) {
		return scenePath
	}

	videoPath := filepath.Join(path, "video.mp4")
	if regularFileExists(videoPath) {
		return videoPath
	}
	if project, err := loadWallpaperEngineProject(filepath.Join(path, "project.json")); err == nil && strings.EqualFold(project.Type, "video") {
		if videoPath, err := inputAssetPath(path, project.File); err == nil && isVideoFile(videoPath) {
			return videoPath
		}
	}
	return ""
}

func isSceneFile(path string) bool {
	return filepath.Base(path) == "scene.wasm"
}

func isVideoFile(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".mp4")
}

func (wallpaper wallpaper) runnableLaunchPath() string {
	if wallpaper.kind == wallpaperEngineScene {
		return wallpaper.wallpaperEngineImportedLaunchPath()
	}
	return wallpaper.launchPath
}

func (wallpaper wallpaper) optionsPath() string {
	if wallpaper.kind == wallpaperEngineScene && wallpaper.importDir != "" {
		return wallpaper.importDir
	}
	if wallpaper.path != "" {
		return wallpaper.path
	}
	if wallpaper.launchPath != "" {
		return filepath.Dir(wallpaper.launchPath)
	}
	return ""
}

func (wallpaper wallpaper) wallpaperEngineImportedLaunchPath() string {
	if wallpaper.importDir == "" {
		return ""
	}
	return wallpaperLaunchTarget(wallpaper.importDir)
}

func (wallpaper wallpaper) importedWallpaperEngineScene() bool {
	return wallpaper.kind == wallpaperEngineScene && wallpaper.runnableLaunchPath() != ""
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
