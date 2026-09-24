package main

import (
	"testing"

	"github.com/movsb/fbui/pkg/game_library"
)

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
