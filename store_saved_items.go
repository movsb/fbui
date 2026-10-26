package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbui/pkg/config"
	"github.com/movsb/fbui/pkg/game_library"
	"github.com/movsb/fbui/pkg/mainui"
	"github.com/movsb/fbui/pkg/menu_popup"
	"github.com/movsb/fbui/pkg/progress_popup"
)

// Build the combined label before preparing native MainUI launch paths.
func storeROMItem(game *game_library.Game, release *game_library.Release, asset *game_library.Asset) mainui.Item {
	gameName, releaseName, romName := displayNames(game.Names), displayNames(release.Names), strings.TrimSpace(asset.Name)
	if gameName == "未命名" {
		gameName = fmt.Sprintf("游戏 #%d", game.ID)
	}
	if releaseName == "未命名" {
		releaseName = fmt.Sprintf("发行版 #%d", release.ID)
	}
	if romName == "" {
		romName = fmt.Sprintf("ROM #%d", asset.ID)
	}
	platform := platformEmulators[game.PlatformID]
	if platform == "" {
		platform = "仓库"
	}
	return mainui.Item{
		Label:    strings.Join([]string{gameName, releaseName, romName}, " / "),
		SubLabel: platform,
		Type:     mainui.ROMType,
	}
}

func (n *StoreNavigator) selectedROM() *game_library.Asset {
	if len(n.stack) == 0 {
		return nil
	}
	page := &n.stack[len(n.stack)-1]
	if page.level != storeAssets || page.game == nil || page.release == nil {
		return nil
	}
	index := n.activeList().DataIndex()
	if index < 0 || index >= len(page.items) {
		return nil
	}
	asset, ok := page.items[index].value.(*game_library.Asset)
	if !ok || asset.Type != game_library.AssetTypeROM {
		return nil
	}
	return asset
}

func (n *StoreNavigator) showROMMenu() {
	asset := n.selectedROM()
	if asset == nil {
		return
	}
	path, err := n.store.MaterializedPath(asset)
	page := &n.stack[len(n.stack)-1]
	var item mainui.Item
	if err == nil {
		item, err = preparedStoreROMItem(&game_library.LaunchableAsset{
			Game: page.game, Release: page.release, Asset: asset, PlatformID: page.game.PlatformID,
		}, path)
	}
	if err != nil {
		n.window.app.ShowAlertDialog(n.window.doc, fbiw.AlertDialogOptions{Title: "读取收藏失败", Description: err.Error()})
		return
	}
	favorite, err := n.window.lists.IsFavorite(item)
	if err != nil {
		n.window.app.ShowAlertDialog(n.window.doc, fbiw.AlertDialogOptions{Title: "读取收藏失败", Description: err.Error()})
		return
	}
	name := "加入收藏"
	if favorite {
		name = "取消收藏"
	}
	menu_popup.NewMenuPopup(n.window.app, n.window.doc, []menu_popup.MenuItem{{
		Name: name,
		Click: func() {
			if !favorite {
				if err := n.favoriteROMByID(asset.ID); err != nil {
					n.window.app.ShowAlertDialog(n.window.doc, fbiw.AlertDialogOptions{Title: "收藏失败", Description: err.Error()})
				}
				return
			}
			if _, err := n.window.lists.ToggleFavorite(item); err != nil {
				n.window.app.ShowAlertDialog(n.window.doc, fbiw.AlertDialogOptions{Title: "更新收藏失败", Description: err.Error()})
			}
		},
	}}, nil, nil)
}

func (n *StoreNavigator) materializeROM(rom *game_library.LaunchableAsset, showProgress, confirmDownload bool, ready func(string)) {
	var popup *progress_popup.Popup
	if showProgress {
		popup = progress_popup.New(n.window.app, n.window.doc, "正在准备 ROM")
	}
	go func() {
		lastUpdate := time.Time{}
		materialize := n.store.Materialize
		if confirmDownload {
			materialize = n.store.MaterializeLocal
		}
		path, err := materialize(context.Background(), rom.Asset, func(message string, progress float32) {
			if time.Since(lastUpdate) < 250*time.Millisecond && progress != 100 {
				return
			}
			lastUpdate = time.Now()
			if popup != nil {
				n.window.doc.Async(func() { popup.SetIndeterminate(fmt.Sprintf("%.0f%%  %s", progress, message)) })
			} else {
				log.Printf("打开仓库 ROM %d：%s %.0f%%", rom.Asset.ID, message, progress)
			}
		})
		n.window.doc.Async(func() {
			if popup != nil {
				popup.Close()
			}
			if errors.Is(err, game_library.ErrDownloadRequired) {
				n.remoteBusy.Store(false)
				n.window.app.ShowAlertDialog(n.window.doc, fbiw.AlertDialogOptions{
					Title:       "下载并收藏？",
					Description: "此 ROM 尚未下载或本地文件需要修复，是否下载并加入收藏？\n" + storeROMItem(rom.Game, rom.Release, rom.Asset).Label,
					ActionText:  "下载并收藏", CancelText: "取消",
					OnAction: func() {
						if !n.remoteBusy.CompareAndSwap(false, true) {
							n.window.showLaunchError(errStoreOpenBusy)
							return
						}
						n.materializeROM(rom, showProgress, false, ready)
					},
				})
				return
			}
			if err != nil {
				n.remoteBusy.Store(false)
				n.showRemoteOpenError(rom.Asset.ID, err)
				return
			}
			ready(path)
		})
	}()
}

func storeEmulator(platformID int32) (*config.LaunchConfig, error) {
	wanted := platformEmulators[platformID]
	if wanted != "" {
		for _, candidate := range config.LoadDir(filepath.Join(config.SDCARDRoot, "Emus")) {
			if strings.EqualFold(filepath.Base(candidate.Dir), wanted) {
				return candidate, nil
			}
		}
	}
	return nil, fmt.Errorf("没有平台 %d 的模拟器映射", platformID)
}

func preparedStoreROMItem(rom *game_library.LaunchableAsset, path string) (mainui.Item, error) {
	item := storeROMItem(rom.Game, rom.Release, rom.Asset)
	emulator, err := storeEmulator(rom.PlatformID)
	if err != nil {
		return item, err
	}
	item.Launch, err = filepath.Abs(emulator.LauncherScriptPath())
	if err != nil {
		return item, err
	}
	item.ROMPath, err = filepath.Abs(path)
	return item, err
}

func (n *StoreNavigator) favoriteROMByID(id int32) error {
	if n.metadataErr != nil {
		return n.metadataErr
	}
	rom, err := n.metadata.GetLaunchableAsset(context.Background(), id)
	if err != nil {
		return err
	}
	if _, err := storeEmulator(rom.PlatformID); err != nil {
		return err
	}
	if !n.remoteBusy.CompareAndSwap(false, true) {
		return errStoreOpenBusy
	}
	n.materializeROM(rom, true, true, func(path string) {
		defer n.remoteBusy.Store(false)
		item, err := preparedStoreROMItem(rom, path)
		if err == nil {
			_, err = n.window.lists.ToggleFavorite(item)
		}
		if err != nil {
			n.window.app.ShowAlertDialog(n.window.doc, fbiw.AlertDialogOptions{Title: "收藏失败", Description: err.Error()})
		}
	})
	return nil
}
