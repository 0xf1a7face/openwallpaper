package main

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

func buildLibraryPane(state *appState, detail detailWidgets) (*gtk.Box, func()) {
	pane := gtk.NewBox(gtk.OrientationVertical, 0)
	pane.SetHExpand(true)
	pane.SetVExpand(true)

	flowBox := gtk.NewFlowBox()
	flowBox.AddCSSClass("wallpaper-grid")
	flowBox.SetVAlign(gtk.AlignStart)
	flowBox.SetVExpand(false)
	flowBox.SetActivateOnSingleClick(false)
	flowBox.SetColumnSpacing(18)
	flowBox.SetRowSpacing(18)
	flowBox.SetMarginTop(18)
	flowBox.SetMarginBottom(18)
	flowBox.SetMarginStart(18)
	flowBox.SetMarginEnd(18)
	flowBox.SetMaxChildrenPerLine(64)
	flowBox.SetMinChildrenPerLine(1)
	flowBox.SetSelectionMode(gtk.SelectionSingle)

	refreshing := false
	populate := func(selectedPath string) {
		refreshing = true
		defer func() {
			refreshing = false
		}()

		flowBox.RemoveAll()
		for _, wallpaper := range state.wallpapers {
			flowBox.Insert(buildWallpaperTile(wallpaper), -1)
		}

		state.selectedIndex = -1
		selectedIndex := 0
		if selectedPath != "" {
			for index, wallpaper := range state.wallpapers {
				if wallpaper.path == selectedPath {
					selectedIndex = index
					break
				}
			}
		}

		if child := flowBox.ChildAtIndex(selectedIndex); child != nil {
			state.selectedIndex = selectedIndex
			flowBox.SelectChild(child)
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

	flowBox.ConnectSelectedChildrenChanged(func() {
		if refreshing {
			return
		}
		selected := flowBox.SelectedChildren()
		if len(selected) == 0 {
			return
		}
		index := selected[0].Index()
		if index >= 0 {
			state.selectedIndex = index
			updateDetail(detail, state)
		}
	})
	flowBox.ConnectChildActivated(func(child *gtk.FlowBoxChild) {
		index := child.Index()
		if index < 0 {
			return
		}
		state.selectedIndex = index
		flowBox.SelectChild(child)
		updateDetail(detail, state)
		runSelectedWallpaper(detail, state)
	})

	scrolled := gtk.NewScrolledWindow()
	scrolled.AddCSSClass("wallpaper-grid")
	scrolled.SetHExpand(true)
	scrolled.SetVExpand(true)
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scrolled.SetChild(flowBox)
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
