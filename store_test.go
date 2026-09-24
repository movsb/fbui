package main

import (
	"testing"

	"github.com/movsb/fbui/pkg/game_library"
)

func TestSelectReleaseImage(t *testing.T) {
	blob := &game_library.Blob{SHA256: "image"}
	asset := func(id int32, typ game_library.AssetType) *game_library.Asset {
		return &game_library.Asset{ID: id, Type: typ, Format: game_library.FormatRegular, Blob: blob}
	}

	gameCover := asset(1, game_library.AssetTypeCover)
	releaseScreenshot := asset(2, game_library.AssetTypeScreenshot)
	if got := selectReleaseImage([]*game_library.Asset{gameCover}, []*game_library.Asset{releaseScreenshot}); got != gameCover {
		t.Fatalf("game asset must win across owners: got %#v", got)
	}

	gameLogo := asset(3, game_library.AssetTypeLogo)
	gameScreenshot := asset(4, game_library.AssetTypeScreenshot)
	if got := selectReleaseImage([]*game_library.Asset{gameCover, gameLogo, gameScreenshot}, nil); got != gameScreenshot {
		t.Fatalf("screenshot must win within an owner: got %#v", got)
	}

	zipScreenshot := asset(5, game_library.AssetTypeScreenshot)
	zipScreenshot.Format = game_library.FormatZIP
	if got := selectReleaseImage([]*game_library.Asset{zipScreenshot}, []*game_library.Asset{releaseScreenshot}); got != releaseScreenshot {
		t.Fatalf("non-displayable game asset should fall back to release asset: got %#v", got)
	}
}

func TestFirstReleaseImageSkipsReleasesWithoutImages(t *testing.T) {
	releases := []*game_library.Release{{ID: 10}, {ID: 20}, {ID: 30}}
	image := &game_library.Asset{
		ID:     2,
		Type:   game_library.AssetTypeScreenshot,
		Format: game_library.FormatRegular,
		Blob:   &game_library.Blob{SHA256: "image"},
	}
	var queried []int32
	got, err := firstReleaseImage(releases, func(releaseID int32) ([]*game_library.Asset, error) {
		queried = append(queried, releaseID)
		if releaseID == 20 {
			return []*game_library.Asset{image}, nil
		}
		return []*game_library.Asset{{Type: game_library.AssetTypeROM}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != image {
		t.Fatalf("got %#v, want the image from the second release", got)
	}
	if len(queried) != 2 || queried[0] != 10 || queried[1] != 20 {
		t.Fatalf("queried releases %v, want [10 20]", queried)
	}
}

func TestSameImageAssetUsesBlobSHA256(t *testing.T) {
	left := &game_library.Asset{ID: 1, Blob: &game_library.Blob{SHA256: "ABCDEF"}}
	right := &game_library.Asset{ID: 2, Blob: &game_library.Blob{SHA256: "abcdef"}}
	if !sameImageAsset(left, right) {
		t.Fatal("assets backed by the same blob should be treated as the same image")
	}
	right.Blob.SHA256 = "different"
	if sameImageAsset(left, right) {
		t.Fatal("assets backed by different blobs should not be treated as the same image")
	}
}

func TestAssetDetailIncludesArcadeROMSets(t *testing.T) {
	asset := &game_library.Asset{
		Type: game_library.AssetTypeROM,
		Size: 1024,
		ROMSets: []*game_library.ROMSet{
			{Emulator: "mame", Version: "0.259"},
			{Emulator: "fbneo", Version: "1.0"},
			{Emulator: "mame", Version: "0.259"},
		},
	}
	if got, want := assetDetail(&game_library.Game{PlatformID: 1}, asset), "1.0 KB  ·  ROM  ·  FBNEO/1.0  ·  MAME/0.259"; got != want {
		t.Fatalf("assetDetail=%q, want %q", got, want)
	}
	if got, want := assetDetail(&game_library.Game{PlatformID: 2}, asset), "1.0 KB  ·  ROM"; got != want {
		t.Fatalf("non-arcade assetDetail=%q, want %q", got, want)
	}
}

func TestBuildGameItemsGroupsSeriesByPlatform(t *testing.T) {
	games := []*game_library.Game{
		{ID: 3, Names: []game_library.Name{{Language: game_library.LanguageChinese, Name: "塞尔达"}}, PlatformNames: []game_library.Name{{Language: game_library.LanguageChinese, Name: "红白机"}}},
		{ID: 2, Names: []game_library.Name{{Language: game_library.LanguageChinese, Name: "马力欧B"}}, PlatformNames: []game_library.Name{{Language: game_library.LanguageChinese, Name: "街机"}}},
		{ID: 1, Names: []game_library.Name{{Language: game_library.LanguageChinese, Name: "马力欧A"}}, PlatformNames: []game_library.Name{{Language: game_library.LanguageChinese, Name: "街机"}}},
	}
	items := buildGameItems(games, true)
	wantNames := []string{"塞尔达", "马力欧A", "马力欧B"}
	wantPlatforms := []string{"红白机", "街机", "街机"}
	for index := range wantNames {
		if items[index].name != wantNames[index] || items[index].platform != wantPlatforms[index] {
			t.Fatalf("item %d=%q/%q, want %q/%q", index, items[index].platform, items[index].name, wantPlatforms[index], wantNames[index])
		}
	}
}

func TestBuildGameItemsOmitsPlatformOutsideSeries(t *testing.T) {
	game := &game_library.Game{ID: 1, Names: []game_library.Name{{Language: game_library.LanguageEnglish, Name: "Game"}}, PlatformNames: []game_library.Name{{Language: game_library.LanguageEnglish, Name: "Platform"}}}
	items := buildGameItems([]*game_library.Game{game}, false)
	if items[0].name != "Game" {
		t.Fatalf("unexpected item name: %q", items[0].name)
	}
}

func TestDisplayNamesPrefersManualChinese(t *testing.T) {
	names := []game_library.Name{
		{Language: game_library.LanguageChinese, Name: "导入中文", Source: "trimui"},
		{Language: game_library.LanguageEnglish, Name: "English"},
		{Language: game_library.LanguageChinese, Name: "手动中文", Source: "  "},
	}
	if got := displayNames(names); got != "手动中文" {
		t.Fatalf("displayNames=%q, want 手动中文", got)
	}
}

func TestDisplayNamesFallsBackByLanguage(t *testing.T) {
	tests := []struct {
		names []game_library.Name
		want  string
	}{
		{names: []game_library.Name{{Language: game_library.LanguageEnglish, Name: "English"}, {Language: game_library.LanguageChinese, Name: "导入中文", Source: "trimui"}}, want: "导入中文"},
		{names: []game_library.Name{{Language: game_library.LanguageJapanese, Name: "日本語"}, {Language: game_library.LanguageEnglish, Name: "English"}}, want: "English"},
		{names: nil, want: "未命名"},
	}
	for _, test := range tests {
		if got := displayNames(test.names); got != test.want {
			t.Errorf("displayNames=%q, want %q", got, test.want)
		}
	}
}
