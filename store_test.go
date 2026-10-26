package main

import (
	"testing"

	"github.com/movsb/fbiw"

	"github.com/movsb/fbui/pkg/game_library"
	"github.com/movsb/fbui/pkg/search_window"
	"github.com/movsb/fbui/pkg/searchable"
)

func TestStoreSearchDisplaysGamesForGameAndReleaseMatches(t *testing.T) {
	game := &game_library.Game{
		ID:            1,
		Names:         []game_library.Name{{Language: game_library.LanguageChinese, Name: "索菲亚复苏", Deprecated: true}, {Language: game_library.LanguageChinese, Name: "魂斗罗"}},
		PlatformNames: []game_library.Name{{Language: game_library.LanguageChinese, Name: "红白机"}},
	}
	items := buildStoreSearchItems(
		[]*game_library.Game{game},
		[]*game_library.Release{
			{ID: 10, GameID: game.ID, Names: []game_library.Name{{Language: game_library.LanguageChinese, Name: "日版"}}},
			{ID: 11, GameID: game.ID, Names: []game_library.Name{{Language: game_library.LanguageEnglish, Name: "USA"}}},
		},
	)
	if len(items) != 1 || items[0].Value != game || items[0].Name != "魂斗罗" || items[0].Detail != "红白机" {
		t.Fatalf("search items=%#v", items)
	}
	for _, query := range []string{"hdl", "rb", "usa", "sfyfs"} {
		if !searchable.Match(query, items[0].SearchTerms...) {
			t.Errorf("query %q did not match game through game/release names", query)
		}
	}
}

func TestBuildReleaseItemsOrdersByType(t *testing.T) {
	releases := []*game_library.Release{
		{ID: 5, ReleaseType: game_library.ReleaseTypeUnspecified, Names: []game_library.Name{{Name: "未分类"}}},
		{ID: 4, ReleaseType: game_library.ReleaseTypeOther, Names: []game_library.Name{{Name: "其他"}}},
		{ID: 3, ReleaseType: game_library.ReleaseTypeModified, Names: []game_library.Name{{Name: "修改版"}}},
		{ID: 2, ReleaseType: game_library.ReleaseTypeTranslation, Names: []game_library.Name{{Name: "汉化版"}}},
		{ID: 1, ReleaseType: game_library.ReleaseTypeOriginal, Names: []game_library.Name{{Name: "原版"}}},
	}
	items := buildReleaseItems(releases)
	want := []string{"原版", "汉化版", "修改版", "其他", "未分类"}
	for i := range want {
		if items[i].name != want[i] {
			t.Fatalf("item %d=%q, want %q", i, items[i].name, want[i])
		}
	}
}

func TestStoreSearchPrioritizesOriginalReleaseMatches(t *testing.T) {
	originalGame := &game_library.Game{ID: 1, Names: []game_library.Name{{Name: "较长的原版游戏名"}}}
	translationGame := &game_library.Game{ID: 2, Names: []game_library.Name{{Name: "短名"}}}
	items := buildStoreSearchItems(
		[]*game_library.Game{translationGame, originalGame},
		[]*game_library.Release{
			{GameID: translationGame.ID, ReleaseType: game_library.ReleaseTypeTranslation, Names: []game_library.Name{{Name: "共同版本名"}}},
			{GameID: originalGame.ID, ReleaseType: game_library.ReleaseTypeOriginal, Names: []game_library.Name{{Name: "共同版本名"}}},
		},
	)
	matched := search_window.MatchItems(items, "共同版本名")
	if len(matched) != 2 || matched[0].Value != originalGame {
		t.Fatalf("matched=%#v; original release match should be first", matched)
	}
}

func TestSelectReleaseImage(t *testing.T) {
	blob := &game_library.Blob{SHA256: "image"}
	gameTitle := &game_library.Asset{Type: game_library.AssetTypeTitle, Format: game_library.FormatRegular, Blob: blob}
	releaseTitle := &game_library.Asset{Type: game_library.AssetTypeTitle, Format: game_library.FormatRegular, Blob: blob}
	if got := selectReleaseImage([]*game_library.Asset{gameTitle}, []*game_library.Asset{releaseTitle}); got != gameTitle {
		t.Fatalf("game title must win: %#v", got)
	}
	for _, invalid := range []*game_library.Asset{
		{Type: game_library.AssetTypeTitle, Format: game_library.FormatZIP, Blob: blob},
		{Type: game_library.AssetTypeTitle, Format: game_library.FormatRegular},
		{Type: game_library.AssetTypeROM, Format: game_library.FormatRegular, Blob: blob},
		{Type: game_library.AssetType(7), Format: game_library.FormatRegular, Blob: blob},
	} {
		if got := selectReleaseImage([]*game_library.Asset{invalid}, []*game_library.Asset{releaseTitle}); got != releaseTitle {
			t.Fatalf("invalid game image should fall back: %#v", got)
		}
	}
}

func TestFirstReleaseImageSkipsReleasesWithoutImages(t *testing.T) {
	releases := []*game_library.Release{{ID: 10}, {ID: 20}, {ID: 30}}
	image := &game_library.Asset{
		ID:     2,
		Type:   game_library.AssetTypeTitle,
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
	asset.Entries = []*game_library.Entry{
		{Name: "present.bin", Blob: &game_library.Blob{SHA256: "present"}},
		{Name: "missing.bin", Blob: &game_library.Blob{}},
	}
	if got, want := assetDetail(&game_library.Game{PlatformID: 1}, asset), "1.0 KB  ·  ROM  ·  缺少文件"; got != want {
		t.Fatalf("incomplete assetDetail=%q, want %q", got, want)
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
		{Language: game_library.LanguageChinese, Name: "历史中文", Deprecated: true},
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

func TestIconImagesUseOnlyDisplayableTitleScreens(t *testing.T) {
	blob := &game_library.Blob{SHA256: "title"}
	cover := &game_library.Asset{Type: game_library.AssetTypeROM, Format: game_library.FormatRegular, Blob: blob}
	missingBlob := &game_library.Asset{Type: game_library.AssetTypeTitle, Format: game_library.FormatRegular}
	archive := &game_library.Asset{Type: game_library.AssetTypeTitle, Format: game_library.FormatZIP, Blob: blob}
	title := &game_library.Asset{Type: game_library.AssetTypeTitle, Format: game_library.FormatRegular, Blob: blob}
	if got := selectTitleImage([]*game_library.Asset{cover, missingBlob, archive}); got != nil {
		t.Fatalf("platform without displayable title must keep an empty image: got %#v", got)
	}
	if got := selectTitleImage([]*game_library.Asset{cover, missingBlob, archive, title}); got != title {
		t.Fatalf("platform title = %#v, want %#v", got, title)
	}
}

func TestStoreIconGridNavigation(t *testing.T) {
	n := &StoreNavigator{
		list: &fbiw.List{}, icons: &fbiw.List{}, series: &fbiw.List{},
		releases: &fbiw.List{}, assets: &fbiw.List{},
	}
	for _, tc := range []struct {
		page storePage
		want *fbiw.List
	}{
		{storePage{level: storePlatforms}, n.icons},
		{storePage{level: storeSeries}, n.icons},
		{storePage{level: storeGames}, n.list},
		{storePage{level: storeGames, groupByPlatform: true}, n.series},
		{storePage{level: storeReleases}, n.releases},
		{storePage{level: storeAssets}, n.assets},
	} {
		n.stack = []storePage{tc.page}
		if got := n.activeList(); got != tc.want {
			t.Fatalf("level %v: active list = %p, want %p", tc.page.level, got, tc.want)
		}
	}
}

func TestStoreIndependentHomePages(t *testing.T) {
	for _, level := range []storeLevel{storePlatforms, storeSeries} {
		n := &StoreNavigator{stack: []storePage{{level: level}}}
		if !n.isRootPage() {
			t.Fatalf("level %v should be a home page", level)
		}
		n.stack = append(n.stack, storePage{level: storeGames})
		if n.isRootPage() {
			t.Fatalf("games below level %v should not be a home page", level)
		}
	}
}
