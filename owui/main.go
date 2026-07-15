package main

import (
	"os"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const (
	appID                    = "org.openwallpaper.ui"
	appTitle                 = "OpenWallpaper UI"
	optionsMaxWidth          = 720
	selectedPaneMinWidth     = 280
	selectedPaneDefaultWidth = 360
	galleryPreviewWidth      = 160
	galleryPreviewHeight     = galleryPreviewWidth * 9 / 16
	galleryTileWidth         = galleryPreviewWidth + 20
	galleryTileHeight        = galleryPreviewHeight + 42
)

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "--autorun" || arg == "-a" {
			autorun()
			return
		}
	}

	app := adw.NewApplication(appID, gio.ApplicationFlagsNone)
	app.ConnectActivate(func() { buildUI(app) })

	if code := app.Run(os.Args); code > 0 {
		os.Exit(code)
	}
}

func buildUI(app *adw.Application) {
	registerBundledIcons()
	installCSS()

	loadedSettings := loadSettings()
	displays := loadDisplays()
	selectedDisplay := ""
	if len(displays) > 0 {
		selectedDisplay = displays[0]
	}

	state := &appState{
		settings:        &loadedSettings,
		wallpapers:      loadWallpapers(loadedSettings),
		selectedDisplay: selectedDisplay,
		processes:       map[int]*wallpaperProcess{},
	}
	app.ConnectShutdown(func() {
		state.disownWallpaperProcesses()
	})

	root := gtk.NewBox(gtk.OrientationVertical, 0)
	root.SetVExpand(true)

	pageStack := adw.NewViewStack()
	pageStack.SetHhomogeneous(false)
	pageStack.SetVhomogeneous(false)

	header, controls := buildTopBar(app, state, pageStack)
	root.Append(header)
	searchKeys := gtk.NewEventControllerKey()
	searchKeys.SetPropagationPhase(gtk.PhaseCapture)
	searchKeys.ConnectKeyPressed(func(keyval uint, _ uint, _ gdk.ModifierType) bool {
		if keyval != gdk.KEY_Escape || controls.titleStack.VisibleChildName() != "search" {
			return false
		}
		controls.searchEntry.Emit("stop-search")
		return true
	})
	root.AddController(searchKeys)

	selectedPane, detail := buildSelectedPane(state, displays)
	selectedPaneVisible := true
	controls.selectedPaneToggle.ConnectClicked(func() {
		selectedPaneVisible = !selectedPaneVisible
		selectedPane.SetVisible(selectedPaneVisible)
	})

	libraryPane, refreshWallpapers := buildLibraryPane(state, detail, controls.searchEntry)
	state.refreshWallpapers = refreshWallpapers

	mainPaned := gtk.NewPaned(gtk.OrientationHorizontal)
	mainPaned.SetVExpand(true)
	mainPaned.SetStartChild(libraryPane)
	mainPaned.SetEndChild(selectedPane)
	mainPaned.SetResizeStartChild(true)
	mainPaned.SetResizeEndChild(false)
	mainPaned.SetShrinkStartChild(false)
	mainPaned.SetShrinkEndChild(true)
	mainPaned.SetWideHandle(true)

	clampSelectedPane := func() {
		width := mainPaned.AllocatedWidth()
		if width <= selectedPaneMinWidth {
			return
		}

		maxPosition := width - selectedPaneMinWidth
		if mainPaned.Position() > maxPosition {
			mainPaned.SetPosition(maxPosition)
		}
	}
	mainPaned.NotifyProperty("position", clampSelectedPane)
	mainPaned.NotifyProperty("width-request", clampSelectedPane)

	defaultSplitSet := false
	mainPaned.AddTickCallback(func(_ gtk.Widgetter, _ gdk.FrameClocker) bool {
		if defaultSplitSet {
			return false
		}
		width := mainPaned.AllocatedWidth()
		if width <= 0 {
			return true
		}
		mainPaned.SetPosition(max(width-selectedPaneDefaultWidth, 0))
		clampSelectedPane()
		defaultSplitSet = true
		return false
	})

	optionsPage := buildOptionsPage(state)
	currentWallpaperSource := steamLibraryPath(*state.settings)

	pageStack.AddTitledWithIcon(mainPaned, "gallery", "Gallery", bundledIconName("wallpaper"))
	pageStack.AddTitledWithIcon(optionsPage, "options", "Options", bundledIconName("settings"))
	pageStack.SetVisibleChildName("gallery")
	pageStack.NotifyProperty("visible-child-name", func() {
		galleryVisible := pageStack.VisibleChildName() == "gallery"
		controls.setVisible(galleryVisible)
		if !galleryVisible {
			return
		}

		nextWallpaperSource := steamLibraryPath(*state.settings)
		if nextWallpaperSource != currentWallpaperSource {
			currentWallpaperSource = nextWallpaperSource
			refreshWallpapers()
		}
	})

	root.Append(pageStack)

	window := adw.NewApplicationWindow(&app.Application)
	window.SetTitle(appTitle)
	window.SetDefaultSize(1080, 360)
	window.SetSizeRequest(360, 360)
	window.SetContent(root)
	state.dialogParent = window
	window.Present()
}

func installCSS() {
	css := gtk.NewCSSProvider()
	css.LoadFromString(`
		.wallpaper-tile {
			padding: 6px;
			border-radius: 6px;
		}

		.wallpaper-grid,
		.wallpaper-grid viewport {
			background: @view_bg_color;
		}

		gridview.wallpaper-grid > child {
			margin: 9px;
			border-radius: 6px;
		}

		.preview-box,
		.large-preview-box {
			background: alpha(currentColor, 0.08);
			border-radius: 6px;
		}

		.bundled-icon {
			color: @window_fg_color;
		}

		button.selected-run-main:dir(ltr) {
			border-top-right-radius: 0;
			border-bottom-right-radius: 0;
			padding-left: 80px;
			padding-right: 32px;
		}

		button.selected-run-main:dir(rtl) {
			border-top-left-radius: 0;
			border-bottom-left-radius: 0;
			padding-left: 32px;
			padding-right: 80px;
		}

		menubutton.selected-run-menu:dir(ltr) {
			border-top-left-radius: 0;
			border-bottom-left-radius: 0;
			border-top-right-radius: 9999px;
			border-bottom-right-radius: 9999px;
			margin-left: -1px;
		}

		menubutton.selected-run-menu:dir(rtl) {
			border-top-right-radius: 0;
			border-bottom-right-radius: 0;
			border-top-left-radius: 9999px;
			border-bottom-left-radius: 9999px;
			margin-right: -1px;
		}

		menubutton.selected-run-menu > button {
			border-radius: inherit;
			min-width: 28px;
			padding-left: 10px;
			padding-right: 10px;
		}
	`)

	if display := gdk.DisplayGetDefault(); display != nil {
		gtk.StyleContextAddProviderForDisplay(display, css, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
	}
}
