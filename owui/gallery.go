package main

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
	"github.com/sahilm/fuzzy"
)

var previewLoadSlots = make(chan struct{}, 4)

type wallpaperTitles []wallpaper

func (wallpapers wallpaperTitles) String(index int) string { return wallpapers[index].title }
func (wallpapers wallpaperTitles) Len() int                { return len(wallpapers) }

func buildLibraryPane(state *appState, detail detailWidgets, searchEntry *gtk.SearchEntry) (*gtk.Box, func()) {
	pane := gtk.NewBox(gtk.OrientationVertical, 0)
	pane.SetHExpand(true)
	pane.SetVExpand(true)

	wallpaperIndices := map[string]int{}
	searchQuery := ""
	searchRanks := map[string]int{}
	model := gtk.NewStringList(nil)
	filter := gtk.NewCustomFilter(func(object *glib.Object) bool {
		if searchQuery == "" {
			return true
		}
		_, found := searchRanks[stringObjectValue(object)]
		return found
	})
	filtered := gtk.NewFilterListModel(model, &filter.Filter)
	sorter := gtk.NewCustomSorter(glib.NewObjectComparer(func(left, right *gtk.StringObject) int {
		leftPath := left.String()
		rightPath := right.String()
		if searchQuery != "" {
			return searchRanks[leftPath] - searchRanks[rightPath]
		}
		return wallpaperIndices[leftPath] - wallpaperIndices[rightPath]
	}))
	sorted := gtk.NewSortListModel(filtered, &sorter.Sorter)
	selection := gtk.NewSingleSelection(sorted)
	selection.SetAutoselect(false)
	wallpaperIndex := func(object *glib.Object) int {
		index, found := wallpaperIndices[stringObjectValue(object)]
		if !found {
			return -1
		}
		return index
	}

	factory := gtk.NewSignalListItemFactory()
	previews := map[uintptr]*previewWidget{}
	factory.ConnectBind(func(object *glib.Object) {
		item := object.Cast().(*gtk.ListItem)
		index := wallpaperIndex(item.Item())
		if index >= 0 && index < len(state.wallpapers) {
			wallpaper := state.wallpapers[index]
			tile, preview := buildWallpaperTile(wallpaper)
			previews[object.Native()] = preview
			item.SetChild(tile)
			setPreviewContent(preview, wallpaper.previewPath)
		}
	})
	factory.ConnectUnbind(func(object *glib.Object) {
		key := object.Native()
		cancelPreviewLoad(previews[key])
		delete(previews, key)
		object.Cast().(*gtk.ListItem).SetChild(nil)
	})

	grid := gtk.NewGridView(selection, &factory.ListItemFactory)
	grid.AddCSSClass("wallpaper-grid")
	grid.SetMaxColumns(64)
	grid.SetMarginTop(9)
	grid.SetMarginBottom(9)
	grid.SetMarginStart(9)
	grid.SetMarginEnd(9)
	grid.AddTickCallback(func(_ gtk.Widgetter, _ gdk.FrameClocker) bool {
		grid.GrabFocus()
		return false
	})
	state.focusGallery = func() {
		grid.GrabFocus()
	}

	refreshing := false
	selectedPath := func() string {
		if state.selectedIndex >= 0 && state.selectedIndex < len(state.wallpapers) {
			return state.wallpapers[state.selectedIndex].path
		}
		return ""
	}
	replaceModel := func() {
		items := make([]string, len(state.wallpapers))
		wallpaperIndices = make(map[string]int, len(state.wallpapers))
		for index, wallpaper := range state.wallpapers {
			items[index] = wallpaper.path
			wallpaperIndices[wallpaper.path] = index
		}
		model.Splice(0, model.NItems(), items)
	}
	populate := func(query string, selectedPath string) {
		refreshing = true
		defer func() {
			refreshing = false
		}()

		selectedIndex := -1
		if index, found := wallpaperIndices[selectedPath]; found {
			selectedIndex = index
		}
		searchQuery = query
		searchRanks = map[string]int{}
		if query != "" {
			matches := fuzzy.FindFrom(query, wallpaperTitles(state.wallpapers))
			for rank, match := range matches {
				searchRanks[state.wallpapers[match.Index].path] = rank
			}
		}
		filter.Changed(gtk.FilterChangeDifferent)
		sorter.Changed(gtk.SorterChangeDifferent)

		selectedPosition := uint(gtk.INVALID_LIST_POSITION)
		if query == "" {
			for position := uint(0); position < sorted.NItems(); position++ {
				if stringObjectValue(sorted.Item(position)) == selectedPath {
					selectedPosition = uint(position)
					break
				}
			}
		}

		if sorted.NItems() > 0 {
			if selectedPosition == gtk.INVALID_LIST_POSITION {
				selectedPosition = 0
			}
			state.selectedIndex = wallpaperIndex(sorted.Item(selectedPosition))
			selection.SetSelected(selectedPosition)
		} else {
			state.selectedIndex = selectedIndex
			selection.SetSelected(gtk.INVALID_LIST_POSITION)
		}
		updateDetail(detail, state)
	}
	replaceModel()
	populate("", "")
	refreshWallpapers := func() {
		path := selectedPath()
		state.wallpapers = loadWallpapers(*state.settings)
		replaceModel()
		populate(searchEntry.Text(), path)
	}
	searchEntry.ConnectChanged(func() {
		populate(searchEntry.Text(), selectedPath())
	})
	searchEntry.ConnectActivate(func() {
		searchEntry.Emit("stop-search")
		runSelectedWallpaper(detail, state)
	})
	searchEntry.ConnectStopSearch(func() {
		glib.IdleAdd(func() {
			position := selection.Selected()
			if position < sorted.NItems() {
				grid.ScrollTo(position, gtk.ListScrollFocus|gtk.ListScrollSelect, nil)
			}
			grid.GrabFocus()
		})
	})
	searchEntry.SetKeyCaptureWidget(pane)

	selection.NotifyProperty("selected", func() {
		if refreshing {
			return
		}
		index := wallpaperIndex(selection.SelectedItem())
		if index < 0 || index >= len(state.wallpapers) {
			return
		}
		state.selectedIndex = index
		updateDetail(detail, state)
	})
	grid.ConnectActivate(func(position uint) {
		index := wallpaperIndex(sorted.Item(position))
		if index < 0 || index >= len(state.wallpapers) {
			return
		}
		state.selectedIndex = index
		runSelectedWallpaper(detail, state)
	})

	searchKeys := gtk.NewEventControllerKey()
	searchKeys.ConnectKeyPressed(func(keyval uint, _ uint, _ gdk.ModifierType) bool {
		if keyval != gdk.KEY_Down && keyval != gdk.KEY_Up {
			return false
		}
		position := selection.Selected()
		if position >= sorted.NItems() {
			return true
		}
		grid.ScrollTo(position, gtk.ListScrollFocus|gtk.ListScrollSelect, nil)
		return true
	})
	searchEntry.AddController(searchKeys)

	scrolled := gtk.NewScrolledWindow()
	scrolled.AddCSSClass("wallpaper-grid")
	scrolled.SetHExpand(true)
	scrolled.SetVExpand(true)
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scrolled.SetChild(grid)
	pane.Append(scrolled)

	return pane, refreshWallpapers
}

func stringObjectValue(object *glib.Object) string {
	if object == nil {
		return ""
	}
	return object.Cast().(*gtk.StringObject).String()
}

func buildWallpaperTile(wallpaper wallpaper) (*gtk.Box, *previewWidget) {
	tile := gtk.NewBox(gtk.OrientationVertical, 6)
	tile.SetSizeRequest(galleryTileWidth, galleryTileHeight)
	tile.SetHAlign(gtk.AlignCenter)
	tile.SetHExpand(false)
	tile.SetVAlign(gtk.AlignStart)
	tile.SetVExpand(false)
	tile.AddCSSClass("wallpaper-tile")

	preview := buildPreviewBox(galleryPreviewWidth, galleryPreviewHeight)
	preview.overlay.SetHAlign(gtk.AlignCenter)
	preview.overlay.SetHExpand(false)
	preview.overlay.SetVAlign(gtk.AlignCenter)
	preview.overlay.SetVExpand(false)
	tile.Append(preview.overlay)

	label := gtk.NewLabel(wallpaper.title)
	label.SetSizeRequest(galleryPreviewWidth, -1)
	label.SetWidthChars(18)
	label.SetMaxWidthChars(18)
	label.SetEllipsize(pango.EllipsizeEnd)
	label.SetSingleLineMode(true)
	label.SetLines(1)
	label.SetHAlign(gtk.AlignCenter)
	label.SetXAlign(0.5)
	label.SetHExpand(false)
	tile.Append(label)

	return tile, preview
}

func buildPreviewBox(width int, height int) *previewWidget {
	overlay := gtk.NewOverlay()
	overlay.SetSizeRequest(width, height)
	overlay.SetOverflow(gtk.OverflowHidden)
	overlay.AddCSSClass("preview-box")

	measure := gtk.NewBox(gtk.OrientationVertical, 0)
	measure.SetSizeRequest(width, height)
	measure.SetHAlign(gtk.AlignFill)
	measure.SetVAlign(gtk.AlignFill)
	measure.SetHExpand(true)
	measure.SetVExpand(true)
	overlay.SetChild(measure)

	preview := &previewWidget{
		overlay: overlay,
		measure: measure,
	}
	return preview
}

func setPreviewContent(preview *previewWidget, previewPath string) {
	if preview.path == previewPath {
		return
	}
	preview.path = previewPath
	cancelPreviewLoad(preview)
	if preview.picture != nil {
		preview.overlay.RemoveOverlay(preview.picture)
		preview.picture = nil
	}
	if previewPath == "" {
		return
	}

	preview.loadCancel = make(chan struct{})
	go loadPreview(preview, previewPath, preview.loadCancel)
}

func cancelPreviewLoad(preview *previewWidget) {
	if preview != nil && preview.loadCancel != nil {
		close(preview.loadCancel)
		preview.loadCancel = nil
	}
}

func loadPreview(preview *previewWidget, path string, cancelled <-chan struct{}) {
	select {
	case previewLoadSlots <- struct{}{}:
	case <-cancelled:
		return
	}
	pixbuf, err := loadPreviewPixbuf(path)
	<-previewLoadSlots
	if err != nil || pixbuf == nil || previewLoadCancelled(cancelled) {
		return
	}

	glib.IdleAdd(func() {
		if !previewLoadCancelled(cancelled) {
			showPreview(preview, pixbuf)
		}
	})
}

func previewLoadCancelled(cancelled <-chan struct{}) bool {
	select {
	case <-cancelled:
		return true
	default:
		return false
	}
}

func showPreview(preview *previewWidget, pixbuf *gdkpixbuf.Pixbuf) {
	picture := gtk.NewPictureForPaintable(gdk.NewTextureForPixbuf(pixbuf))
	picture.SetContentFit(gtk.ContentFitCover)
	picture.SetCanShrink(true)
	picture.SetHAlign(gtk.AlignFill)
	picture.SetVAlign(gtk.AlignFill)
	picture.SetHExpand(true)
	picture.SetVExpand(true)

	preview.overlay.AddOverlay(picture)
	preview.overlay.SetMeasureOverlay(picture, false)
	preview.overlay.SetClipOverlay(picture, true)
	preview.picture = picture
}

func loadPreviewPixbuf(path string) (*gdkpixbuf.Pixbuf, error) {
	animation, err := gdkpixbuf.NewPixbufAnimationFromFile(path)
	if err == nil {
		return animation.StaticImage(), nil
	}
	return gdkpixbuf.NewPixbufFromFile(path)
}
