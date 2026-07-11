package main

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

type galleryControls struct {
	selectedPaneToggle *gtk.Button
	displayButton      *gtk.MenuButton
}

func (c galleryControls) setVisible(visible bool) {
	c.selectedPaneToggle.SetVisible(visible)
	c.displayButton.SetVisible(visible)
}

func buildTopBar(app *adw.Application, state *appState, pageStack *adw.ViewStack) (*adw.HeaderBar, galleryControls) {
	header := adw.NewHeaderBar()
	header.SetShowStartTitleButtons(false)
	header.SetShowEndTitleButtons(false)

	pageSwitcher := adw.NewViewSwitcher()
	pageSwitcher.SetStack(pageStack)
	pageSwitcher.SetPolicy(adw.ViewSwitcherPolicyWide)
	header.SetTitleWidget(pageSwitcher)

	selectedPaneToggle := adwaitaIconButton("split-view", "Show or hide selected wallpaper")
	header.PackStart(selectedPaneToggle)

	displayButton := gtk.NewMenuButton()
	displayButton.AddCSSClass("image-button")
	displayButton.SetChild(adwaitaIcon("display"))
	displayButton.SetTooltipText("Active wallpapers")
	displayButton.SetPopover(buildDisplayPopover(state))
	header.PackStart(displayButton)

	closeButton := adwaitaIconButton("close", "Close")
	closeButton.ConnectClicked(func() {
		app.Quit()
	})
	header.PackEnd(closeButton)

	return header, galleryControls{
		selectedPaneToggle: selectedPaneToggle,
		displayButton:      displayButton,
	}
}

type displayMapping struct {
	display string
	path    string
}

func buildDisplayPopover(state *appState) *gtk.Popover {
	popover := gtk.NewPopover()
	state.onDisplayMappingsChanged(func() {
		popover.SetChild(buildDisplayPopoverContent(state, popover))
	})
	return popover
}

func buildDisplayPopoverContent(state *appState, popover *gtk.Popover) *gtk.Box {
	content := gtk.NewBox(gtk.OrientationVertical, 8)
	content.SetSizeRequest(320, -1)
	content.SetMarginTop(10)
	content.SetMarginBottom(10)
	content.SetMarginStart(10)
	content.SetMarginEnd(10)

	mappings := sortedDisplayMappings(state.settings.AutorunWallpapers)
	if len(mappings) == 0 {
		empty := gtk.NewLabel("No active wallpapers")
		empty.AddCSSClass("dim-label")
		empty.SetMarginTop(12)
		empty.SetMarginBottom(12)
		content.Append(empty)
		return content
	}

	list := boxedList()

	for _, mapping := range mappings {
		mapping := mapping
		rowBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
		setRowMargins(rowBox)

		textBox := gtk.NewBox(gtk.OrientationVertical, 2)
		textBox.SetHExpand(true)

		displayLabel := gtk.NewLabel(mapping.display)
		displayLabel.SetXAlign(0)
		displayLabel.SetEllipsize(pango.EllipsizeEnd)
		displayLabel.SetMaxWidthChars(24)
		displayLabel.SetSingleLineMode(true)
		textBox.Append(displayLabel)

		wallpaperLabel := gtk.NewLabel(wallpaperTitleForPath(state, mapping.path))
		wallpaperLabel.AddCSSClass("dim-label")
		wallpaperLabel.SetXAlign(0)
		wallpaperLabel.SetEllipsize(pango.EllipsizeEnd)
		wallpaperLabel.SetMaxWidthChars(28)
		wallpaperLabel.SetSingleLineMode(true)
		textBox.Append(wallpaperLabel)

		logsButton := adwaitaIconButton("terminal", "Renderer logs")
		logsButton.AddCSSClass("flat")
		logsButton.SetSensitive(state.processForDisplayMapping(mapping.display, mapping.path) != nil)
		logsButton.ConnectClicked(func() {
			process := state.processForDisplayMapping(mapping.display, mapping.path)
			if process == nil || state.dialogParent == nil {
				return
			}
			popover.Popdown()
			showRendererLogsDialog(state.dialogParent, process)
		})

		closeButton := adwaitaIconButton("close", "Disable wallpaper")
		closeButton.AddCSSClass("flat")
		closeButton.ConnectClicked(func() {
			delete(state.settings.AutorunWallpapers, mapping.display)
			saveSettings(*state.settings)
			stopWallpaper(state, mapping.display)
			state.notifyDisplayMappingsChanged()
		})

		rowBox.Append(textBox)
		rowBox.Append(logsButton)
		rowBox.Append(closeButton)

		row := plainWidgetRow(rowBox)
		row.SetSelectable(false)
		row.SetActivatable(false)
		list.Append(row)
	}

	content.Append(list)
	return content
}

func sortedDisplayMappings(source map[string]string) []displayMapping {
	mappings := make([]displayMapping, 0, len(source))
	for display, path := range source {
		mappings = append(mappings, displayMapping{display: display, path: path})
	}
	sort.Slice(mappings, func(left, right int) bool {
		return strings.ToLower(mappings[left].display) < strings.ToLower(mappings[right].display)
	})
	return mappings
}

func wallpaperTitleForPath(state *appState, path string) string {
	for _, wallpaper := range state.wallpapers {
		if wallpaper.optionsPath() == path {
			return wallpaper.title
		}
	}

	return filepath.Base(path)
}
