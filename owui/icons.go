package main

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const bundledIconResourcePath = "/org/openwallpaper/ui/icons"

//go:embed res/icons.gresource
var bundledIconResourceData []byte

var bundledIconResource *gio.Resource

func adwaitaIconButton(iconName string, tooltip string) *gtk.Button {
	button := gtk.NewButton()
	button.AddCSSClass("image-button")
	button.SetChild(adwaitaIcon(iconName))
	button.SetTooltipText(tooltip)
	return button
}

func adwaitaIcon(iconName string) *gtk.Image {
	image := gtk.NewImageFromIconName(bundledIconName(iconName))
	image.AddCSSClass("bundled-icon")
	image.SetIconSize(gtk.IconSizeNormal)
	return image
}

func bundledIconName(iconName string) string {
	return "owui-" + iconName + "-symbolic"
}

func registerBundledIcons() {
	if bundledIconResource == nil {
		resource, err := gio.NewResourceFromData(glib.NewBytesWithGo(bundledIconResourceData))
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load bundled icons: %v\n", err)
			return
		}
		gio.ResourcesRegister(resource)
		bundledIconResource = resource
	}

	display := gdk.DisplayGetDefault()
	if display == nil {
		return
	}
	gtk.IconThemeGetForDisplay(display).AddResourcePath(bundledIconResourcePath)
}
