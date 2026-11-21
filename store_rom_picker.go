package main

import (
	"github.com/movsb/fbiw"
	"github.com/movsb/fbiw/input/sticks"
	"github.com/movsb/fbui/pkg/game_library"
)

type storeROMPicker struct {
	doc  *fbiw.Document
	list *fbiw.List `css:"#rom-list"`
}

type storeROMPickerItem struct {
	root   fbiw.Box
	name   *fbiw.Text `css:".name"`
	detail *fbiw.Text `css:".detail"`
}

func (view *storeROMPickerItem) ListSelectionChanged(selected bool) {
	view.name.SetMarqueeRunning(selected)
	view.detail.SetMarqueeRunning(selected)
}

func (n *StoreNavigator) showROMPicker(roms []*game_library.Asset) {
	picker := &storeROMPicker{doc: n.window.app.NewPopup(_embed, "store_rom_picker.html", n.window.doc)}
	picker.doc.Bind(picker)
	picker.list.SetItems(len(roms),
		func() (fbiw.Box, *storeROMPickerItem) {
			view := picker.doc.Instantiate[storeROMPickerItem]("rom-item")
			return view.root, view
		},
		func(view *storeROMPickerItem, index int) {
			view.name.SetText(roms[index].Name)
			view.detail.SetText(releaseROMDetail(roms[index]))
		},
	)
	picker.list.Activate()
	picker.doc.Listen(fbiw.InputDownEvent, func(event *fbiw.Event) {
		switch event.Input.Name {
		case sticks.B:
			picker.doc.Close()
			event.StopPropagation()
		case sticks.A:
			if index := picker.list.DataIndex(); index >= 0 && index < len(roms) {
				picker.doc.Close()
				n.openAsset(roms[index])
			}
			event.StopPropagation()
		}
	})
}
