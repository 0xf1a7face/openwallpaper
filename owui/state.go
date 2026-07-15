package main

import (
	"sync"
	"sync/atomic"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type detailWidgets struct {
	title                        *gtk.Label
	description                  *gtk.Label
	descriptionExpander          *gtk.Expander
	preview                      *previewWidget
	optionsBox                   *gtk.Box
	otherBox                     *gtk.Box
	speedSpin                    *gtk.SpinButton
	selectedRunButton            *gtk.Button
	selectedRunMenu              *gtk.MenuButton
	selectedAdvancedImportButton *gtk.Button
	bottomAdvancedImportButton   *gtk.Button
	deleteWallpaperButton        *gtk.Button
}

type previewWidget struct {
	overlay    *gtk.Overlay
	measure    *gtk.Box
	picture    *gtk.Picture
	path       string
	loadCancel chan struct{}
}

type appState struct {
	settings               *settings
	wallpapers             []wallpaper
	selectedIndex          int
	selectedDisplay        string
	displayMappingsChanged []func()
	processMu              sync.Mutex
	processes              map[int]*wallpaperProcess
	dialogParent           gtk.Widgetter
	exiting                atomic.Bool
	updatingDetail         bool
	working                atomic.Bool
	refreshWallpapers      func()
	focusGallery           func()
}

func (state *appState) notifyDisplayMappingsChanged() {
	for _, callback := range state.displayMappingsChanged {
		callback()
	}
}

func (state *appState) onDisplayMappingsChanged(callback func()) {
	state.displayMappingsChanged = append(state.displayMappingsChanged, callback)
	callback()
}
