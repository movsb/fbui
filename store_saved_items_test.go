package main

import (
	"github.com/movsb/fbui/pkg/config"
	"os"
	"path/filepath"
	"testing"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbui/pkg/game_library"
)

func TestStoreROMItemLabel(t *testing.T) {
	game := &game_library.Game{ID: 1, PlatformID: 2, Names: []game_library.Name{{Name: "游戏", Language: game_library.LanguageChinese}}}
	release := &game_library.Release{ID: 2, Names: []game_library.Name{{Name: "日版", Language: game_library.LanguageChinese}}}
	asset := &game_library.Asset{ID: 3, Name: "game.zip", Type: game_library.AssetTypeROM}
	item := storeROMItem(game, release, asset)
	if item.Label != "游戏 / 日版 / game.zip" || item.ROMPath != "" || item.SubLabel != "FC" {
		t.Fatalf("unexpected ROM item: %#v", item)
	}
	game.Names, release.Names, asset.Name = nil, nil, ""
	if got := storeROMItem(game, release, asset).Label; got != "游戏 #1 / 发行版 #2 / ROM #3" {
		t.Fatalf("unnamed ROM label = %q", got)
	}
}

func TestStoreFavoriteTargetIsOnlySelectedROM(t *testing.T) {
	app := fbiw.NewApp(fbiw.WithRenderer(savedItemsTestRenderer{}))
	defer app.Close()
	doc := app.NewDesktop(_embed, "window.html")
	n := &StoreNavigator{assets: doc.QuerySelector[*fbiw.List]("#store-asset-list")}
	n.assets.SetItems(2,
		func() (fbiw.Box, *storeAssetView) {
			view := doc.Instantiate[storeAssetView]("store-asset-item")
			return view.root, view
		},
		func(*storeAssetView, int) {},
	)
	n.stack = []storePage{{
		level: storeAssets,
		game:  &game_library.Game{ID: 1}, release: &game_library.Release{ID: 2},
		items: []storeItem{
			{value: &game_library.Asset{ID: 3, Type: game_library.AssetTypeROM}},
			{value: &game_library.Asset{ID: 4, Type: game_library.AssetTypeTitle}},
		},
	}}
	n.assets.SetIndex(0, 0, 0)
	if item := n.selectedROM(); item == nil || item.ID != 3 {
		t.Fatalf("selected ROM should be the favorite target: %#v", item)
	}
	n.assets.SetIndex(1, 0, 0)
	if item := n.selectedROM(); item != nil {
		t.Fatalf("image should not be a favorite target: %#v", item)
	}
	n.stack[0].level = storeReleases
	if item := n.selectedROM(); item != nil {
		t.Fatalf("release should not be a favorite target: %#v", item)
	}
}

func TestPreparedStoreROMItemHasNativeLaunchPaths(t *testing.T) {
	root := t.TempDir()
	oldRoot := config.SDCARDRoot
	config.SDCARDRoot = root
	t.Cleanup(func() { config.SDCARDRoot = oldRoot })
	emuDir := filepath.Join(root, "Emus", "FC")
	if err := os.MkdirAll(emuDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(emuDir, "config.json"), []byte(`{"label":"FC","launch":"launch.sh"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(emuDir, "launch.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "game.zip")
	if err := os.WriteFile(path, []byte("rom"), 0o644); err != nil {
		t.Fatal(err)
	}
	rom := &game_library.LaunchableAsset{
		PlatformID: 2,
		Game:       &game_library.Game{ID: 1, PlatformID: 2},
		Release:    &game_library.Release{ID: 2},
		Asset:      &game_library.Asset{ID: 3, Name: "game.zip", Type: game_library.AssetTypeROM},
	}
	item, err := preparedStoreROMItem(rom, path)
	if err != nil || item.Launch != script || item.ROMPath != path {
		t.Fatalf("unexpected native launch item: %#v, err=%v", item, err)
	}
	rom.PlatformID = 999
	if _, err := preparedStoreROMItem(rom, path); err == nil {
		t.Fatal("unsupported platform must not create an unusable native record")
	}
}
