package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	ndl "github.com/mechakotik/ndl/lib"
)

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
	name := object.Name
	if name == "" {
		name = "(unnamed)"
	}
	return fmt.Sprintf("Object %d: %s (%s)", object.Index, name, object.Type)
}

func effectLabel(effect wallpaperEngineEffect) string {
	name := effect.Name
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
		if value != "" {
			set[value] = true
		}
	}
	return set
}

func wallpaperEngineEffectID(effect wallpaperEngineEffect) string {
	if effect.ID != "" {
		return effect.ID
	}
	return strconv.Itoa(effect.Index)
}

func importWallpaperEngineScene(parent gtk.Widgetter, focusGallery func(), wallpaper wallpaper, options wallpaperEngineImportOptions, done func(error)) {
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
		if focusGallery != nil {
			glib.IdleAdd(focusGallery)
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
