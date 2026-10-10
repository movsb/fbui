package main

import (
	"context"
	"embed"
	"sort"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbiw/input/sticks"
	"github.com/movsb/fbui/pkg/game_library"
)

//go:embed store_search_detail.html
var storeSearchDetailAssets embed.FS

type storeSearchDetailLevel uint8

const (
	storeSearchDetailReleases storeSearchDetailLevel = iota
	storeSearchDetailAssetsLevel
)

type storeSearchDetail struct {
	navigator *StoreNavigator
	doc       *fbiw.Document

	list    *fbiw.List `css:"#list"`
	message fbiw.Box   `css:"#message"`
	msgText *fbiw.Text `css:"#message text"`

	game           *game_library.Game
	release        *game_library.Release
	level          storeSearchDetailLevel
	startedAtAsset bool
	items          []storeItem
	busy           bool
}

type storeSearchDetailItemView struct {
	root   fbiw.Box
	name   *fbiw.Text `css:".name"`
	detail *fbiw.Text `css:".detail"`
}

func newStoreSearchDetail(n *StoreNavigator, searchDoc *fbiw.Document, game *game_library.Game, release *game_library.Release) {
	w := &storeSearchDetail{
		navigator:      n,
		doc:            n.window.app.NewPopup(storeSearchDetailAssets, "store_search_detail.html", searchDoc),
		game:           game,
		release:        release,
		startedAtAsset: release != nil,
	}
	w.doc.Bind(w)
	w.doc.Listen(fbiw.InputDownEvent, w.handleEvents)
	w.setHints()
	if release == nil {
		w.loadReleases()
	} else {
		w.loadAssets(release)
	}
}

func (w *storeSearchDetail) setHints() {
	overlayWindow.SetButtonHints(
		ButtonHint{Button: `A`, Text: `打开`},
		ButtonHint{Button: `B`, Text: `返回`},
	)
}

func (w *storeSearchDetail) showMessage(message string) {
	w.msgText.SetText(message)
	w.message.SetProp("display", "true")
	w.list.SetProp("display", "false")
}

func (w *storeSearchDetail) render() {
	w.message.SetProp("display", "false")
	w.list.SetProp("display", "true")
	w.list.SetItems(len(w.items),
		func() (fbiw.Box, *storeSearchDetailItemView) {
			view := w.doc.Instantiate[storeSearchDetailItemView]("item")
			return view.root, view
		},
		func(view *storeSearchDetailItemView, index int) {
			view.name.SetText(w.items[index].name)
			view.detail.SetText(w.items[index].detail)
		},
	)
	if len(w.items) == 0 {
		w.showMessage("没有内容")
		return
	}
	w.list.SetIndex(0, 0, 0)
	w.list.Activate()
	w.setHints()
}

func (w *storeSearchDetail) loadReleases() {
	w.busy = true
	w.level = storeSearchDetailReleases
	w.showMessage("加载发行版...")
	go func() {
		releases, err := w.navigator.metadata.ListReleases(context.Background(), w.game.ID)
		w.doc.Async(func() {
			w.busy = false
			if err != nil {
				w.showMessage("加载发行版失败：" + err.Error())
				w.setHints()
				return
			}
			w.items = make([]storeItem, 0, len(releases))
			for _, release := range releases {
				w.items = append(w.items, storeItem{name: displayNames(release.Names), value: release})
			}
			w.render()
		})
	}()
}

func (w *storeSearchDetail) loadAssets(release *game_library.Release) {
	w.busy = true
	w.level = storeSearchDetailAssetsLevel
	w.release = release
	w.showMessage("加载资源...")
	go func() {
		assets, err := w.navigator.metadata.ListAssets(context.Background(), release.ID)
		w.doc.Async(func() {
			w.busy = false
			if err != nil {
				w.showMessage("加载资源失败：" + err.Error())
				w.setHints()
				return
			}
			sort.SliceStable(assets, func(i, j int) bool {
				return assets[i].Type == game_library.AssetTypeROM && assets[j].Type != game_library.AssetTypeROM
			})
			w.items = make([]storeItem, 0, len(assets))
			for _, asset := range assets {
				w.items = append(w.items, storeItem{name: asset.Name, detail: assetDetail(w.game, asset), value: asset})
			}
			w.render()
		})
	}()
}

func (w *storeSearchDetail) close() {
	w.doc.Close()
	overlayWindow.SetButtonHints(
		ButtonHint{Button: `A`, Text: `打开`},
		ButtonHint{Button: `B`, Text: `返回`},
	)
}

func (w *storeSearchDetail) handleEvents(event *fbiw.Event) {
	if w.busy {
		event.StopPropagation()
		return
	}
	switch event.Input.Name {
	case sticks.B:
		if w.level == storeSearchDetailAssetsLevel && !w.startedAtAsset {
			w.loadReleases()
		} else {
			w.close()
		}
		event.StopPropagation()
	case sticks.A:
		index := w.list.DataIndex()
		if index < 0 || index >= len(w.items) {
			return
		}
		if w.level == storeSearchDetailReleases {
			w.loadAssets(w.items[index].value.(*game_library.Release))
		} else {
			asset := w.items[index].value.(*game_library.Asset)
			newStoreAssetView(w.navigator, w.doc, w.game, asset, w.setHints)
		}
		event.StopPropagation()
	}
}
