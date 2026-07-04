package main

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func sectionLabel(text string) *gtk.Label {
	label := gtk.NewLabel(text)
	label.AddCSSClass("heading")
	label.SetHAlign(gtk.AlignStart)
	label.SetMarginTop(6)
	return label
}

func boxedList() *gtk.ListBox {
	list := gtk.NewListBox()
	list.AddCSSClass("boxed-list")
	list.SetSelectionMode(gtk.SelectionNone)
	return list
}

func labeledWidgetRow(labelText string, widgets ...gtk.Widgetter) *gtk.ListBoxRow {
	label := gtk.NewLabel(labelText)
	label.SetXAlign(0)
	label.SetHExpand(true)
	label.SetWrap(true)

	return plainWidgetRow(rowContent(append([]gtk.Widgetter{label}, widgets...)...))
}

func entryRow(labelText string, entry *gtk.Entry) *gtk.ListBoxRow {
	entry.SetHExpand(true)
	return labeledWidgetRow(labelText, entry)
}

func menuSectionButtonRow(iconName string, text string, onActivate func()) *gtk.ListBoxRow {
	row := menuSectionRow(iconName, text)
	row.SetActivatable(true)
	row.ConnectActivate(onActivate)

	click := gtk.NewGestureClick()
	click.ConnectReleased(func(nPress int, x, y float64) {
		if nPress == 1 {
			onActivate()
		}
	})
	row.AddController(click)
	return row
}

func menuSectionRow(iconName string, text string) *gtk.ListBoxRow {
	label := gtk.NewLabel(text)
	label.SetXAlign(0)
	label.SetHExpand(true)
	label.SetWrap(true)

	row := plainWidgetRow(rowContent(adwaitaIcon(iconName), label, adwaitaIcon("go-next")))
	row.SetSelectable(false)
	row.SetActivatable(false)
	return row
}

func plainWidgetRow(widget gtk.Widgetter) *gtk.ListBoxRow {
	row := gtk.NewListBoxRow()
	row.SetChild(widget)
	return row
}

func setRowMargins(widget *gtk.Box) {
	widget.SetMarginTop(12)
	widget.SetMarginBottom(12)
	widget.SetMarginStart(12)
	widget.SetMarginEnd(12)
}

func rowContent(widgets ...gtk.Widgetter) *gtk.Box {
	rowBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	setRowMargins(rowBox)
	for _, widget := range widgets {
		rowBox.Append(widget)
	}
	return rowBox
}
