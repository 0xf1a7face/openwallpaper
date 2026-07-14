package main

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

func buildLibraryPane(state *appState, detail detailWidgets) (*gtk.Box, func()) {
	pane := gtk.NewBox(gtk.OrientationVertical, 0)
	pane.SetHExpand(true)
	pane.SetVExpand(true)

	model := gtk.NewStringList(nil)
	selection := gtk.NewSingleSelection(model)

	factory := gtk.NewSignalListItemFactory()
	factory.ConnectBind(func(object *glib.Object) {
		item := object.Cast().(*gtk.ListItem)
		position := item.Position()
		if position < uint(len(state.wallpapers)) {
			item.SetChild(buildWallpaperTile(state.wallpapers[position]))
		}
	})
	factory.ConnectUnbind(func(object *glib.Object) {
		object.Cast().(*gtk.ListItem).SetChild(nil)
	})

	grid := gtk.NewGridView(selection, &factory.ListItemFactory)
	grid.AddCSSClass("wallpaper-grid")
	grid.SetMaxColumns(64)
	grid.SetMarginTop(9)
	grid.SetMarginBottom(9)
	grid.SetMarginStart(9)
	grid.SetMarginEnd(9)

	refreshing := false
	populate := func(selectedPath string) {
		refreshing = true
		defer func() {
			refreshing = false
		}()

		state.selectedIndex = -1
		items := make([]string, len(state.wallpapers))
		selectedIndex := 0
		for index, wallpaper := range state.wallpapers {
			items[index] = wallpaper.path
			if selectedPath != "" && wallpaper.path == selectedPath {
				selectedIndex = index
			}
		}
		model.Splice(0, model.NItems(), items)

		if len(state.wallpapers) > 0 {
			state.selectedIndex = selectedIndex
			selection.SetSelected(uint(selectedIndex))
		}
		updateDetail(detail, state)
	}
	populate("")
	refreshWallpapers := func() {
		selectedPath := ""
		if state.selectedIndex >= 0 && state.selectedIndex < len(state.wallpapers) {
			selectedPath = state.wallpapers[state.selectedIndex].path
		}
		state.wallpapers = loadWallpapers(*state.settings)
		populate(selectedPath)
	}

	selection.NotifyProperty("selected", func() {
		if refreshing {
			return
		}
		position := selection.Selected()
		if position >= uint(len(state.wallpapers)) {
			return
		}
		state.selectedIndex = int(position)
		updateDetail(detail, state)
	})
	grid.ConnectActivate(func(position uint) {
		if position >= uint(len(state.wallpapers)) {
			return
		}
		state.selectedIndex = int(position)
		runSelectedWallpaper(detail, state)
	})

	scrolled := gtk.NewScrolledWindow()
	scrolled.AddCSSClass("wallpaper-grid")
	scrolled.SetHExpand(true)
	scrolled.SetVExpand(true)
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scrolled.SetChild(grid)
	pane.Append(scrolled)

	return pane, refreshWallpapers
}

func buildWallpaperTile(wallpaper wallpaper) *gtk.Box {
	tile := gtk.NewBox(gtk.OrientationVertical, 6)
	tile.SetSizeRequest(galleryTileWidth, galleryTileHeight)
	tile.SetHAlign(gtk.AlignCenter)
	tile.SetHExpand(false)
	tile.SetVAlign(gtk.AlignStart)
	tile.SetVExpand(false)
	tile.AddCSSClass("wallpaper-tile")

	preview := buildPreviewBox(wallpaper.previewPath, galleryPreviewWidth, galleryPreviewHeight)
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

	return tile
}

func buildPreviewBox(previewPath string, width int, height int) *previewWidget {
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

	placeholder := gtk.NewLabel("No preview")
	placeholder.SetHAlign(gtk.AlignCenter)
	placeholder.SetVAlign(gtk.AlignCenter)
	overlay.AddOverlay(placeholder)
	overlay.SetMeasureOverlay(placeholder, false)

	preview := &previewWidget{
		overlay:     overlay,
		measure:     measure,
		placeholder: placeholder,
	}
	setPreviewContent(preview, previewPath)

	return preview
}

func setPreviewContent(preview *previewWidget, previewPath string) {
	if preview.picture != nil {
		preview.overlay.RemoveOverlay(preview.picture)
		preview.picture = nil
	}

	if previewPath != "" {
		if pixbuf, err := loadPreviewPixbuf(previewPath); err == nil && pixbuf != nil {
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
	}

	preview.placeholder.SetVisible(preview.picture == nil)
}

func loadPreviewPixbuf(path string) (*gdkpixbuf.Pixbuf, error) {
	animation, err := gdkpixbuf.NewPixbufAnimationFromFile(path)
	if err == nil {
		return animation.StaticImage(), nil
	}
	return gdkpixbuf.NewPixbufFromFile(path)
}
