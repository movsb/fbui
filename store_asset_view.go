package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbiw/input/sticks"
	"github.com/movsb/fbui/pkg/game_library"
	"github.com/movsb/fbui/pkg/video_player"
)

//go:embed store_asset_view.html
var storeAssetViewAssets embed.FS

type storeAssetOpenWindow struct {
	navigator *StoreNavigator
	doc       *fbiw.Document
	game      *game_library.Game
	asset     *game_library.Asset
	onClose   func()

	status     fbiw.Box                  `css:"#status"`
	statusText *fbiw.Text                `css:"#status text"`
	image      *fbiw.Image               `css:"#image"`
	video      *video_player.VideoPlayer `css:"#video"`
	busy       bool
}

func newStoreAssetView(n *StoreNavigator, opener *fbiw.Document, game *game_library.Game, asset *game_library.Asset, onClose func()) {
	w := &storeAssetOpenWindow{
		navigator: n,
		doc:       n.window.app.NewPopup(storeAssetViewAssets, "store_asset_view.html", opener),
		game:      game,
		asset:     asset,
		onClose:   onClose,
		busy:      true,
	}
	w.doc.Bind(w)
	w.doc.Listen(fbiw.InputDownEvent, w.handleEvents)
	w.setStatus("准备资源...")
	overlayWindow.SetButtonHints(ButtonHint{Button: `B`, Text: `关闭`})
	w.open()
}

func (w *storeAssetOpenWindow) setStatus(message string) {
	w.statusText.SetText(message)
	w.status.SetProp("display", "true")
	w.image.SetProp("display", "false")
	w.video.SetProp("display", "false")
}

func (w *storeAssetOpenWindow) open() {
	asset := w.asset
	if asset.Type == game_library.AssetTypeROM {
		launchable, err := w.navigator.metadata.GetLaunchableAsset(context.Background(), asset.ID)
		if err != nil {
			w.busy = false
			title := "无法打开 ROM"
			if errors.Is(err, game_library.ErrUnsupportedMAMEVersion) {
				title = "ROM 不受支持"
			}
			w.navigator.window.app.ShowAlertDialog(w.doc, fbiw.AlertDialogOptions{Title: title, Description: err.Error()})
			return
		}
		asset = launchable.Asset
	}

	go func() {
		lastMessage := ""
		lastTime := time.Time{}
		path, err := w.navigator.store.Materialize(context.Background(), asset, func(message string, progress float32) {
			if message == lastMessage && time.Since(lastTime) < 250*time.Millisecond && progress != 100 {
				return
			}
			lastMessage = message
			lastTime = time.Now()
			w.doc.Async(func() { w.setStatus(fmt.Sprintf("%.0f%%\n%s", progress, message)) })
		})
		w.doc.Async(func() {
			w.busy = false
			if err != nil {
				w.setStatus("下载失败：\n" + err.Error())
				return
			}
			switch asset.Type {
			case game_library.AssetTypeROM:
				w.busy = true
				w.setStatus("游戏运行中...")
				w.navigator.runROMWithOpener(w.doc, w.game.PlatformID, path, func() {
					w.doc.Async(func() {
						w.busy = false
						w.setStatus("游戏已退出")
					})
				})
			case game_library.AssetTypeCover, game_library.AssetTypeScreenshot, game_library.AssetTypeLogo, game_library.AssetTypeTitle:
				w.status.SetProp("display", "false")
				w.image.SetOSPath(path)
				w.image.SetProp("display", "true")
			case game_library.AssetTypeVideo:
				w.status.SetProp("display", "false")
				w.video.SetPath(path)
				w.video.SetProp("display", "true")
			default:
				w.setStatus("下载成功：\n" + path)
			}
		})
	}()
}

func (w *storeAssetOpenWindow) handleEvents(event *fbiw.Event) {
	event.StopPropagation()
	if event.Input.Name != sticks.B || w.busy {
		return
	}
	w.video.Stop()
	w.doc.Close()
	if w.onClose != nil {
		w.onClose()
	}
}
