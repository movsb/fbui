package main

import (
	"testing"

	"github.com/movsb/fbiw"
	"golang.org/x/image/font/gofont/goregular"
)

func TestROMPickerFitsContent(t *testing.T) {
	app := fbiw.NewApp(fbiw.WithRenderer(savedItemsTestRenderer{}), fbiw.WithSystemFontData(goregular.TTF))
	defer app.Close()
	doc := app.NewDesktop(_embed, "store_rom_picker.html")
	picker := &storeROMPicker{doc: doc}
	doc.Bind(picker)
	name := "game.zip"
	var views []*storeROMPickerItem
	picker.list.SetItems(2,
		func() (fbiw.Box, *storeROMPickerItem) {
			view := doc.Instantiate[storeROMPickerItem]("rom-item")
			views = append(views, view)
			return view.root, view
		},
		func(view *storeROMPickerItem, index int) {
			view.name.SetText(name)
			view.detail.SetText("MAME 0.259")
		},
	)
	measure := func() int {
		doc.Root().Calc(640, 480, fbiw.Constraints{ParentContentWidth: 640, ParentContentHeight: 480, PrefersMaxWidth: true, PrefersMaxHeight: true})
		return picker.list.Base().GetLayoutBox().Width
	}
	shortWidth := measure()
	if shortWidth <= 0 || shortWidth >= 500 {
		t.Fatalf("short ROM names should produce a compact picker, got width %d", shortWidth)
	}
	name = "a much longer game filename.zip"
	for _, view := range views {
		view.name.SetText(name)
	}
	longWidth := measure()
	if longWidth <= shortWidth || longWidth >= 640 {
		t.Fatalf("picker should grow with content: short=%d, long=%d", shortWidth, longWidth)
	}
}
