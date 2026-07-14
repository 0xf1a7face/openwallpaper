package main

import (
	"strconv"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

func buildSelectedPane(state *appState, displays []string) (*gtk.Box, detailWidgets) {
	pane := gtk.NewBox(gtk.OrientationVertical, 0)
	pane.SetSizeRequest(selectedPaneMinWidth, -1)
	pane.SetHExpand(false)
	pane.SetVExpand(true)

	body := gtk.NewBox(gtk.OrientationVertical, 18)
	body.SetHExpand(true)
	body.SetVAlign(gtk.AlignStart)
	body.SetVExpand(false)
	body.SetMarginTop(18)
	body.SetMarginBottom(18)
	body.SetMarginStart(18)
	body.SetMarginEnd(18)

	scrolled := gtk.NewScrolledWindow()
	scrolled.SetHExpand(true)
	scrolled.SetVExpand(true)
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scrolled.SetPropagateNaturalHeight(false)
	scrolled.SetPropagateNaturalWidth(false)
	scrolled.SetChild(body)
	pane.Append(scrolled)

	largePreview := buildPreviewBox(160, 90)
	largePreview.overlay.SetSizeRequest(-1, 90)
	largePreview.measure.SetSizeRequest(-1, 90)
	largePreview.overlay.SetHExpand(true)
	largePreview.overlay.SetHAlign(gtk.AlignFill)
	largePreview.overlay.SetVAlign(gtk.AlignStart)

	lastPreviewWidth := 0
	largePreview.overlay.AddTickCallback(func(_ gtk.Widgetter, _ gdk.FrameClocker) bool {
		width := largePreview.overlay.AllocatedWidth()
		if width > 0 && width != lastPreviewWidth {
			lastPreviewWidth = width
			height := max(width*9/16, 90)
			largePreview.overlay.SetSizeRequest(-1, height)
			largePreview.measure.SetSizeRequest(-1, height)
		}
		return true
	})

	body.Append(largePreview.overlay)

	title := gtk.NewLabel("")
	title.AddCSSClass("title-2")
	title.SetWrap(true)
	title.SetWrapMode(pango.WrapWordChar)
	title.SetEllipsize(pango.EllipsizeEnd)
	title.SetLines(3)
	title.SetJustify(gtk.JustifyCenter)
	title.SetXAlign(0.5)
	body.Append(title)

	runControls := buildSelectedRunControls(state, displays)
	body.Append(runControls.box)

	description := gtk.NewLabel("")
	description.SetWrap(true)
	description.SetWrapMode(pango.WrapWordChar)
	description.SetEllipsize(pango.EllipsizeEnd)
	description.SetLines(4)
	description.SetJustify(gtk.JustifyLeft)
	description.SetXAlign(0)
	description.SetHAlign(gtk.AlignFill)

	descriptionExpander := gtk.NewExpander("Description")
	descriptionExpander.SetExpanded(false)
	descriptionExpander.SetResizeToplevel(false)
	descriptionExpander.SetChild(description)
	descriptionExpander.SetVisible(false)
	body.Append(descriptionExpander)

	optionsBox := gtk.NewBox(gtk.OrientationVertical, 6)
	optionsBox.SetHExpand(true)
	optionsBox.Append(sectionLabel("General options"))

	optionsList := boxedList()

	speedSpin := gtk.NewSpinButtonWithRange(0.01, 1000, 0.25)
	speedSpin.SetDigits(2)
	speedSpin.SetNumeric(true)
	speedSpin.SetUpdatePolicy(gtk.UpdateIfValid)
	speedSpin.SetValue(defaultWallpaperOptions().Speed)
	optionsList.Append(labeledWidgetRow("Speed", speedSpin))

	overrideOptionsRow := menuSectionButtonRow("settings", "Override global options", func() {
		showOverrideOptionsDialog(state)
	})
	optionsList.Append(overrideOptionsRow)

	optionsBox.Append(optionsList)
	body.Append(optionsBox)

	otherBox := gtk.NewBox(gtk.OrientationVertical, 6)
	otherBox.SetHExpand(true)
	otherBox.SetVisible(false)
	otherBox.Append(sectionLabel("Other"))

	bottomAdvancedImportButton := newAdvancedImportButton()
	otherBox.Append(bottomAdvancedImportButton)

	deleteWallpaperButton := gtk.NewButtonWithLabel("Delete wallpaper")
	deleteWallpaperButton.AddCSSClass("destructive-action")
	deleteWallpaperButton.AddCSSClass("pill")
	deleteWallpaperButton.SetHExpand(true)
	deleteWallpaperButton.SetVisible(false)
	otherBox.Append(deleteWallpaperButton)
	body.Append(otherBox)

	detail := detailWidgets{
		title:                        title,
		description:                  description,
		descriptionExpander:          descriptionExpander,
		preview:                      largePreview,
		optionsBox:                   optionsBox,
		otherBox:                     otherBox,
		speedSpin:                    speedSpin,
		selectedRunButton:            runControls.runButton,
		selectedRunMenu:              runControls.runMenu,
		selectedAdvancedImportButton: runControls.advancedImportButton,
		bottomAdvancedImportButton:   bottomAdvancedImportButton,
		deleteWallpaperButton:        deleteWallpaperButton,
	}
	updateDetail(detail, state)
	detail.selectedRunButton.ConnectClicked(func() {
		runSelectedWallpaper(detail, state)
	})
	detail.selectedAdvancedImportButton.ConnectClicked(func() {
		importSelectedWallpaperWithOptions(detail, state)
	})
	detail.bottomAdvancedImportButton.ConnectClicked(func() {
		importSelectedWallpaperWithOptions(detail, state)
	})
	detail.deleteWallpaperButton.ConnectClicked(func() {
		deleteSelectedWallpaper(detail, state)
	})
	detail.speedSpin.ConnectValueChanged(func() {
		if !state.updatingDetail {
			saveSelectedWallpaperOptions(detail, state)
		}
	})

	return pane, detail
}

type selectedRunControls struct {
	box                  *gtk.Box
	runButton            *gtk.Button
	runMenu              *gtk.MenuButton
	advancedImportButton *gtk.Button
}

func buildSelectedRunControls(state *appState, displays []string) selectedRunControls {
	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetHAlign(gtk.AlignFill)
	box.SetHExpand(true)

	runBox := gtk.NewBox(gtk.OrientationHorizontal, 0)
	runBox.AddCSSClass("linked")
	runBox.SetHAlign(gtk.AlignFill)
	runBox.SetHExpand(true)

	runButton := gtk.NewButtonWithLabel(selectedRunButtonTitle(state))
	runButton.AddCSSClass("suggested-action")
	runButton.AddCSSClass("pill")
	runButton.SetHExpand(true)
	runButton.SetTooltipText("Run wallpaper")

	menuButton := gtk.NewMenuButton()
	menuButton.AddCSSClass("suggested-action")
	menuButton.AddCSSClass("pill")
	menuButton.AddCSSClass("selected-run-menu")
	menuButton.SetSizeRequest(48, -1)
	menuButton.SetTooltipText("Select display")

	if len(displays) > 0 {
		popover := gtk.NewPopover()
		list := gtk.NewBox(gtk.OrientationVertical, 0)
		list.SetMarginTop(6)
		list.SetMarginBottom(6)
		list.SetMarginStart(6)
		list.SetMarginEnd(6)

		for _, display := range displays {
			display := display
			item := gtk.NewButtonWithLabel(display)
			item.AddCSSClass("flat")
			item.SetHAlign(gtk.AlignFill)
			item.ConnectClicked(func() {
				state.selectedDisplay = display
				runButton.SetLabel(selectedRunButtonTitle(state))
				popover.Popdown()
			})
			list.Append(item)
		}

		popover.SetChild(list)
		menuButton.SetPopover(popover)
	}

	runBox.Append(runButton)
	runBox.Append(menuButton)
	box.Append(runBox)

	advancedImportButton := newAdvancedImportButton()
	box.Append(advancedImportButton)

	return selectedRunControls{
		box:                  box,
		runButton:            runButton,
		runMenu:              menuButton,
		advancedImportButton: advancedImportButton,
	}
}

func newAdvancedImportButton() *gtk.Button {
	button := gtk.NewButtonWithLabel("Advanced import")
	button.AddCSSClass("pill")
	button.SetHExpand(true)
	button.SetTooltipText("Import with object selection")
	button.SetVisible(false)
	return button
}

func updateDetail(detail detailWidgets, state *appState) {
	state.updatingDetail = true
	defer func() {
		state.updatingDetail = false
	}()

	if state.selectedIndex >= 0 && state.selectedIndex < len(state.wallpapers) {
		wallpaper := state.wallpapers[state.selectedIndex]
		isWallpaperEngineScene := wallpaper.kind == wallpaperEngineScene
		importedWallpaperEngineScene := isWallpaperEngineScene && wallpaper.importedWallpaperEngineScene()
		unimportedWallpaperEngineScene := isWallpaperEngineScene && !importedWallpaperEngineScene
		hasOptions := !isWallpaperEngineScene || importedWallpaperEngineScene
		options := defaultWallpaperOptions()
		if hasOptions {
			options = wallpaperOptionsForPath(*state.settings, wallpaper.optionsPath())
		}

		detail.title.SetText(wallpaper.title)
		detail.description.SetText(wallpaper.description)
		detail.descriptionExpander.SetVisible(wallpaper.description != "")
		detail.descriptionExpander.SetExpanded(false)
		setPreviewContent(detail.preview, wallpaper.previewPath)
		detail.speedSpin.SetValue(options.Speed)
		detail.optionsBox.SetVisible(hasOptions)
		updateRunButtonTitle(detail, state)
		detail.selectedRunMenu.SetVisible(!unimportedWallpaperEngineScene)
		setRunButtonSplit(detail, !unimportedWallpaperEngineScene)
		showBottomAdvancedImport := importedWallpaperEngineScene
		showDeleteWallpaper := wallpaper.canDelete()
		detail.selectedAdvancedImportButton.SetVisible(unimportedWallpaperEngineScene)
		detail.bottomAdvancedImportButton.SetVisible(showBottomAdvancedImport)
		detail.deleteWallpaperButton.SetVisible(showDeleteWallpaper)
		detail.otherBox.SetVisible(showBottomAdvancedImport || showDeleteWallpaper)
		setSelectedActionButtonsSensitive(detail, !state.working.Load())
		return
	}

	detail.title.SetText("Wallpaper")
	detail.description.SetText("")
	detail.descriptionExpander.SetVisible(false)
	detail.descriptionExpander.SetExpanded(false)
	setPreviewContent(detail.preview, "")
	detail.speedSpin.SetValue(defaultWallpaperOptions().Speed)
	detail.optionsBox.SetVisible(false)
	detail.selectedRunMenu.SetVisible(true)
	setRunButtonSplit(detail, true)
	detail.selectedAdvancedImportButton.SetVisible(false)
	detail.bottomAdvancedImportButton.SetVisible(false)
	detail.deleteWallpaperButton.SetVisible(false)
	detail.otherBox.SetVisible(false)
	setSelectedActionButtonsSensitive(detail, false)
}

func runSelectedWallpaper(detail detailWidgets, state *appState) {
	if state.working.Load() {
		return
	}
	if state.selectedIndex < 0 || state.selectedIndex >= len(state.wallpapers) {
		return
	}

	selectedWallpaper := state.wallpapers[state.selectedIndex]
	if selectedWallpaper.kind == wallpaperEngineScene && !selectedWallpaper.importedWallpaperEngineScene() {
		importSelectedWallpaper(detail, state)
		return
	}

	launchPath := selectedWallpaper.runnableLaunchPath()
	if launchPath == "" {
		return
	}
	settingsPath := selectedWallpaper.optionsPath()

	state.working.Store(true)
	setSelectedActionButtonsSensitive(detail, false)

	display := state.selectedDisplay
	if state.settings.AutorunWallpapers == nil {
		state.settings.AutorunWallpapers = map[string]string{}
	}
	state.settings.AutorunWallpapers[display] = settingsPath
	saveSettings(*state.settings)
	state.notifyDisplayMappingsChanged()

	saveSelectedWallpaperOptions(detail, state)
	args := wallpaperdArgs(*state.settings, launchPath, settingsPath, display)
	go func() {
		runWallpaperWithArgs(state, settingsPath, display, args)
		glib.IdleAdd(func() {
			state.working.Store(false)
			setSelectedActionButtonsSensitive(detail, state.selectedIndex >= 0 && state.selectedIndex < len(state.wallpapers))
		})
	}()
}

func importSelectedWallpaper(detail detailWidgets, state *appState) {
	selectedWallpaper, ok := beginSelectedWallpaperImport(detail, state)
	if !ok {
		return
	}

	settingsPath := selectedWallpaper.optionsPath()
	importOptions, logText, err := wallpaperEngineImportOptionsFromSavedSettings(selectedWallpaper, wallpaperOptionsForPath(*state.settings, settingsPath))
	if err != nil {
		dialog, _ := rendererLogDialog("Import failed", logText)
		dialog.Present(state.dialogParent)
		finishSelectedWallpaperImport(detail, state)
		return
	}

	importWallpaperEngineScene(state.dialogParent, selectedWallpaper, importOptions, func(err error) {
		finishSelectedWallpaperImport(detail, state)
	})
}

func importSelectedWallpaperWithOptions(detail detailWidgets, state *appState) {
	selectedWallpaper, ok := beginSelectedWallpaperImport(detail, state)
	if !ok {
		return
	}

	settingsPath := selectedWallpaper.optionsPath()
	showWallpaperEngineImportOptions(state.dialogParent, selectedWallpaper, wallpaperOptionsForPath(*state.settings, settingsPath), func(options *wallpaperEngineImportOptions) {
		if options == nil {
			finishSelectedWallpaperImport(detail, state)
			return
		}
		saveWallpaperEngineImportOptions(state, selectedWallpaper, *options)
		importWallpaperEngineScene(state.dialogParent, selectedWallpaper, *options, func(err error) {
			finishSelectedWallpaperImport(detail, state)
		})
	})
}

func beginSelectedWallpaperImport(detail detailWidgets, state *appState) (wallpaper, bool) {
	if state.working.Load() {
		return wallpaper{}, false
	}
	if state.selectedIndex < 0 || state.selectedIndex >= len(state.wallpapers) || state.dialogParent == nil {
		return wallpaper{}, false
	}

	selectedWallpaper := state.wallpapers[state.selectedIndex]
	if selectedWallpaper.kind != wallpaperEngineScene {
		return wallpaper{}, false
	}

	state.working.Store(true)
	setSelectedActionButtonsSensitive(detail, false)
	return selectedWallpaper, true
}

func finishSelectedWallpaperImport(detail detailWidgets, state *appState) {
	state.working.Store(false)
	updateDetail(detail, state)
}

func saveWallpaperEngineImportOptions(state *appState, wallpaper wallpaper, importOptions wallpaperEngineImportOptions) {
	settingsPath := wallpaper.optionsPath()
	options := wallpaperOptionsForPath(*state.settings, settingsPath)
	options.HiddenObjectIDs = importOptions.hiddenObjectIDs
	options.HiddenEffects = importOptions.hiddenEffects
	state.settings.setWallpaperOptions(settingsPath, options)
	saveSettings(*state.settings)
}

func saveSelectedWallpaperOptions(detail detailWidgets, state *appState) {
	if state.selectedIndex < 0 || state.selectedIndex >= len(state.wallpapers) {
		return
	}

	wallpaper := state.wallpapers[state.selectedIndex]
	options := wallpaperOptionsForPath(*state.settings, wallpaper.optionsPath())
	options.Speed = detail.speedSpin.Value()

	state.settings.setWallpaperOptions(wallpaper.optionsPath(), options)
	saveSettings(*state.settings)
}

func deleteSelectedWallpaper(detail detailWidgets, state *appState) {
	if state.working.Load() {
		return
	}
	if state.selectedIndex < 0 || state.selectedIndex >= len(state.wallpapers) {
		return
	}

	wallpaper := state.wallpapers[state.selectedIndex]
	deletePath, ok, err := deleteWallpaperFiles(wallpaper)
	if !ok {
		return
	}
	if err != nil {
		showDeleteErrorDialog(state, err)
		return
	}

	removeWallpaperFromSettings(state, deletePath)
	if state.refreshWallpapers != nil {
		state.refreshWallpapers()
	} else {
		updateDetail(detail, state)
	}
}

func removeWallpaperFromSettings(state *appState, path string) {
	delete(state.settings.WallpaperOptions, path)
	for display, wallpaperPath := range state.settings.AutorunWallpapers {
		if wallpaperPath != path {
			continue
		}
		delete(state.settings.AutorunWallpapers, display)
		stopWallpaper(state, display)
	}
	saveSettings(*state.settings)
	state.notifyDisplayMappingsChanged()
}

func showDeleteErrorDialog(state *appState, err error) {
	if state.dialogParent == nil {
		return
	}
	dialog := adw.NewAlertDialog("Delete failed", err.Error())
	dialog.AddResponse("close", "Close")
	dialog.SetDefaultResponse("close")
	dialog.SetCloseResponse("close")
	dialog.Present(state.dialogParent)
}

func showOverrideOptionsDialog(state *appState) {
	if state.selectedIndex < 0 || state.selectedIndex >= len(state.wallpapers) || state.dialogParent == nil {
		return
	}

	wallpaper := state.wallpapers[state.selectedIndex]
	optionsPath := wallpaper.optionsPath()
	launchPath := wallpaper.runnableLaunchPath()
	options := wallpaperOptionsForPath(*state.settings, optionsPath)

	dialog := adw.NewAlertDialog("Override global options", "")
	dialog.AddResponse("close", "Close")
	dialog.SetDefaultResponse("close")
	dialog.SetCloseResponse("close")

	list := boxedList()
	list.SetSizeRequest(460, -1)

	updating := false
	save := func() {
		if updating {
			return
		}
		state.settings.setWallpaperOptions(optionsPath, options)
		saveSettings(*state.settings)
	}
	refreshControls := func() {}
	saveAndRefresh := func(update func()) {
		if updating {
			return
		}
		update()
		save()
		refreshControls()
	}

	if isSceneFile(launchPath) {
		scene := func() sceneWallpaperOptions {
			return sceneOptionsWithOverrides(globalSceneOptions(*state.settings), options)
		}

		vSyncSwitch := gtk.NewSwitch()
		vSyncSwitch.SetVAlign(gtk.AlignCenter)
		vSyncReset := overrideResetButton()
		list.Append(overrideWidgetRow("V-Sync", vSyncReset, vSyncSwitch))

		fpsSpin := gtk.NewSpinButtonWithRange(1, 720, 1)
		fpsReset := overrideResetButton()
		list.Append(overrideWidgetRow("FPS limit", fpsReset, fpsSpin))

		dgpuSwitch := gtk.NewSwitch()
		dgpuSwitch.SetVAlign(gtk.AlignCenter)
		dgpuReset := overrideResetButton()
		list.Append(overrideWidgetRow("Prefer discrete GPU", dgpuReset, dgpuSwitch))

		audioSwitch := gtk.NewSwitch()
		audioSwitch.SetVAlign(gtk.AlignCenter)
		audioReset := overrideResetButton()
		list.Append(overrideWidgetRow("Enable audio visualization", audioReset, audioSwitch))

		backendDropdown := gtk.NewDropDownFromStrings([]string{"Default", "PipeWire", "PulseAudio", "PortAudio"})
		backendReset := overrideResetButton()
		list.Append(overrideWidgetRow("Audio backend", backendReset, backendDropdown))

		audioSource := gtk.NewEntry()
		audioSource.SetPlaceholderText("Default")
		audioSource.SetHExpand(true)
		sourceReset := overrideResetButton()
		list.Append(overrideWidgetRow("Audio source", sourceReset, audioSource))

		refreshControls = func() {
			current := scene()
			updating = true
			vSyncSwitch.SetActive(current.VSync)
			vSyncReset.SetSensitive(options.VSyncOverridden)
			fpsSpin.SetValue(float64(current.FPSLimit))
			fpsReset.SetSensitive(options.FPSLimitOverridden)
			dgpuSwitch.SetActive(current.PreferDiscreteGPU)
			dgpuReset.SetSensitive(options.PreferDiscreteGPUOverridden)
			audioSwitch.SetActive(current.AudioVisualization)
			audioReset.SetSensitive(options.AudioVisualizationOverridden)
			backendDropdown.SetSelected(min(current.AudioBackend, 3))
			backendReset.SetSensitive(options.AudioBackendOverridden)
			audioSource.SetText(current.AudioSource)
			sourceReset.SetSensitive(options.AudioSourceOverridden)
			fpsSpin.SetSensitive(!current.VSync)
			audioEnabled := current.AudioVisualization
			backendDropdown.SetSensitive(audioEnabled)
			audioSource.SetSensitive(audioEnabled)
			updating = false
		}

		vSyncSwitch.ConnectStateSet(func(value bool) bool {
			saveAndRefresh(func() {
				options.VSync = value
				options.VSyncOverridden = true
			})
			return false
		})
		vSyncReset.ConnectClicked(func() {
			saveAndRefresh(func() { options.VSyncOverridden = false })
		})

		fpsSpin.ConnectValueChanged(func() {
			saveAndRefresh(func() {
				options.FPSLimit = fpsSpin.ValueAsInt()
				options.FPSLimitOverridden = true
			})
		})
		fpsReset.ConnectClicked(func() {
			saveAndRefresh(func() { options.FPSLimitOverridden = false })
		})

		dgpuSwitch.ConnectStateSet(func(value bool) bool {
			saveAndRefresh(func() {
				options.PreferDiscreteGPU = value
				options.PreferDiscreteGPUOverridden = true
			})
			return false
		})
		dgpuReset.ConnectClicked(func() {
			saveAndRefresh(func() { options.PreferDiscreteGPUOverridden = false })
		})

		audioSwitch.ConnectStateSet(func(value bool) bool {
			saveAndRefresh(func() {
				options.AudioVisualization = value
				options.AudioVisualizationOverridden = true
			})
			return false
		})
		audioReset.ConnectClicked(func() {
			saveAndRefresh(func() { options.AudioVisualizationOverridden = false })
		})

		backendDropdown.NotifyProperty("selected", func() {
			saveAndRefresh(func() {
				options.AudioBackend = backendDropdown.Selected()
				options.AudioBackendOverridden = true
			})
		})
		backendReset.ConnectClicked(func() {
			saveAndRefresh(func() { options.AudioBackendOverridden = false })
		})

		audioSource.NotifyProperty("text", func() {
			saveAndRefresh(func() {
				options.AudioSource = audioSource.Text()
				options.AudioSourceOverridden = true
			})
		})
		sourceReset.ConnectClicked(func() {
			saveAndRefresh(func() { options.AudioSourceOverridden = false })
		})
	} else if isVideoFile(launchPath) {
		video := func() videoWallpaperOptions {
			return videoOptionsWithOverrides(globalVideoOptions(*state.settings), options)
		}

		scaleModeDropdown := gtk.NewDropDownFromStrings([]string{"Aspect crop", "Aspect fit", "Stretch"})
		scaleModeReset := overrideResetButton()
		list.Append(overrideWidgetRow("Scale mode", scaleModeReset, scaleModeDropdown))

		filterOptions := overrideFilterOptions(video().Filter)
		filterDropdown := gtk.NewDropDownFromStrings(filterOptions)
		filterReset := overrideResetButton()
		list.Append(overrideWidgetRow("Filter", filterReset, filterDropdown))

		refreshControls = func() {
			current := video()
			updating = true
			scaleModeDropdown.SetSelected(scaleModeIndex(current.ScaleMode))
			scaleModeReset.SetSensitive(options.ScaleModeOverridden)
			filterDropdown.SetSelected(filterOptionIndex(filterOptions, current.Filter))
			filterReset.SetSensitive(options.FilterOverridden)
			updating = false
		}

		scaleModeDropdown.NotifyProperty("selected", func() {
			saveAndRefresh(func() {
				options.ScaleMode = scaleModeValue(scaleModeDropdown.Selected())
				options.ScaleModeOverridden = true
			})
		})
		scaleModeReset.ConnectClicked(func() {
			saveAndRefresh(func() { options.ScaleModeOverridden = false })
		})

		filterDropdown.NotifyProperty("selected", func() {
			saveAndRefresh(func() {
				selected := filterDropdown.Selected()
				if selected == 0 || int(selected) >= len(filterOptions) {
					options.Filter = ""
				} else {
					options.Filter = filterOptions[selected]
				}
				options.FilterOverridden = true
			})
		})
		filterReset.ConnectClicked(func() {
			saveAndRefresh(func() { options.FilterOverridden = false })
		})
	}

	refreshControls()

	content := gtk.NewBox(gtk.OrientationVertical, 0)
	content.SetMarginTop(6)
	content.SetMarginBottom(6)
	content.SetMarginStart(6)
	content.SetMarginEnd(6)
	content.Append(list)
	dialog.SetExtraChild(content)
	dialog.Present(state.dialogParent)
}

func overrideResetButton() *gtk.Button {
	button := adwaitaIconButton("reset", "Use default")
	button.AddCSSClass("flat")
	return button
}

func overrideWidgetRow(labelText string, resetButton *gtk.Button, widget gtk.Widgetter) *gtk.ListBoxRow {
	return labeledWidgetRow(labelText, widget, resetButton)
}

func overrideFilterOptions(current string) []string {
	filters := loadMPVScaleFilters()
	if current != "" && indexOfString(filters, current) < 0 {
		filters = append([]string{current}, filters...)
	}
	return append([]string{"Default"}, filters...)
}

func filterOptionIndex(options []string, filter string) uint {
	if filter == "" {
		return 0
	}
	index := indexOfString(options, filter)
	if index < 0 {
		return 0
	}
	return uint(index)
}

func formatSpeed(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func setSelectedActionButtonsSensitive(detail detailWidgets, sensitive bool) {
	detail.selectedRunButton.SetSensitive(sensitive)
	detail.selectedRunMenu.SetSensitive(sensitive)
	detail.selectedAdvancedImportButton.SetSensitive(sensitive)
	detail.bottomAdvancedImportButton.SetSensitive(sensitive)
	detail.deleteWallpaperButton.SetSensitive(sensitive)
}

func setRunButtonSplit(detail detailWidgets, split bool) {
	if split {
		detail.selectedRunButton.AddCSSClass("selected-run-main")
	} else {
		detail.selectedRunButton.RemoveCSSClass("selected-run-main")
	}
}

func updateRunButtonTitle(detail detailWidgets, state *appState) {
	detail.selectedRunButton.SetLabel(selectedRunButtonTitle(state))
}

func selectedRunButtonTitle(state *appState) string {
	prefix := "Run"
	if state.selectedIndex >= 0 && state.selectedIndex < len(state.wallpapers) {
		wallpaper := state.wallpapers[state.selectedIndex]
		if wallpaper.kind == wallpaperEngineScene && !wallpaper.importedWallpaperEngineScene() {
			return "Import"
		}
	}

	if state.selectedDisplay == "" {
		return prefix
	}
	return prefix + " on " + state.selectedDisplay
}
