package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/movsb/fbui/pkg/game_library"
	"github.com/movsb/fbui/pkg/search_window"
)

func searchFixture() []search_window.Item {
	games := []*game_library.Game{
		{ID: 1, Names: []game_library.Name{{Name: "拳皇 '97"}, {Name: "The King of Fighters"}}},
		{ID: 2, Names: []game_library.Name{{Name: "第二次机器人大战"}}},
		{ID: 3, Names: []game_library.Name{{Name: "Final Fantasy VII"}}},
		{ID: 4, Names: []game_library.Name{{Name: "重装机兵"}}},
		{ID: 5, Names: []game_library.Name{{Name: "短名"}}},
	}
	releases := []*game_library.Release{
		{GameID: 1, ReleaseType: game_library.ReleaseTypeOriginal, Names: []game_library.Name{{Name: "共同版本名"}}},
		{GameID: 5, ReleaseType: game_library.ReleaseTypeTranslation, Names: []game_library.Name{{Name: "共同版本名"}}},
	}
	return buildStoreSearchItems(games, releases)
}

func TestStoreSearchRanksShortestMatchedName(t *testing.T) {
	var items []search_window.Item
	for i, groups := range [][][]string{
		{{"骡子大作战"}},
		{{"联合大作战"}, {"中东战争"}},
		{{"战斗装置"}},
		{{"ABzdzz"}, nil, {"zdzz"}},
	} {
		items = append(items, search_window.Item{Name: groups[0][0], SearchGroups: groups, Value: &game_library.Game{ID: int32(i + 1)}})
	}
	index := buildStoreSearchIndex(items)
	path := filepath.Join(t.TempDir(), "index")
	if err := writeStoreSearchIndex(path, index); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Supply the database metadata stored in this synthetic index.
	index.Size, index.Modified = info.Size(), info.ModTime().UnixNano()
	if err := writeStoreSearchIndex(path, index); err != nil {
		t.Fatal(err)
	}
	loaded, err := readStoreSearchIndex(path, info)
	if err != nil {
		t.Fatal(err)
	}
	for _, matched := range [][]search_window.Item{search_window.MatchItems(items, "zdzz"), loaded.search("zdzz")} {
		var ids []int32
		for _, item := range matched {
			ids = append(ids, item.Value.(*game_library.Game).ID)
		}
		if want := []int32{3, 2, 4, 1}; !reflect.DeepEqual(ids, want) {
			t.Fatalf("IDs=%v; want %v", ids, want)
		}
	}
}

func TestDiskStoreSearchMatchesOriginal(t *testing.T) {
	items := searchFixture()
	index := buildStoreSearchIndex(items)
	dir := t.TempDir()
	db := filepath.Join(dir, "gm.db")
	if err := os.WriteFile(db, []byte("snapshot"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(db)
	if err != nil {
		t.Fatal(err)
	}
	index.Size, index.Modified = info.Size(), info.ModTime().UnixNano()
	path := db + ".search-v1"
	if err := writeStoreSearchIndex(path, index); err != nil {
		t.Fatal(err)
	}
	loaded, err := readStoreSearchIndex(path, info)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"", "qh97", "qh 97", "QH-97", "拳皇", "king", "d2c", "dec", "7", "vii", "zzjb", "czjb", "共同版本名", "gtbbm", "短名", "---", "不存在"} {
		got, want := loaded.search(query), search_window.MatchItems(items, query)
		ids := func(items []search_window.Item) []int32 {
			var out []int32
			for _, item := range items {
				out = append(out, item.Value.(*game_library.Game).ID)
			}
			return out
		}
		if !reflect.DeepEqual(ids(got), ids(want)) {
			t.Errorf("query=%q got=%v want=%v", query, ids(got), ids(want))
		}
		for i := range got {
			if !reflect.DeepEqual(got[i].Value, want[i].Value) {
				t.Errorf("game metadata differs for %q", query)
			}
		}
	}
	// Database timestamp and size changes both invalidate the cache.
	if err := os.Chtimes(db, time.Now().Add(time.Hour), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	changed, _ := os.Stat(db)
	if _, err := readStoreSearchIndex(path, changed); err == nil {
		t.Fatal("accepted stale timestamp")
	}
	index.Version++
	if err := writeStoreSearchIndex(path, index); err != nil {
		t.Fatal(err)
	}
	if _, err := readStoreSearchIndex(path, info); err == nil {
		t.Fatal("accepted stale index version")
	}
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readStoreSearchIndex(path, info); err == nil {
		t.Fatal("accepted corrupt cache")
	}
}

func BenchmarkStoreSearch(b *testing.B) {
	items := searchFixture()
	// Reuse names but assign separate game IDs, approximating a large library.
	var all []search_window.Item
	for i := 0; i < 4000; i++ {
		all = append(all, items...)
	}
	index := buildStoreSearchIndex(all)
	b.Run("original", func(b *testing.B) {
		for b.Loop() {
			search_window.MatchItems(all, "zzjb")
		}
	})
	b.Run("prepared-index", func(b *testing.B) {
		for b.Loop() {
			index.search("zzjb")
		}
	})
}

// Set FBUI_SEARCH_BENCH_DB to benchmark a real read-only snapshot. The generated
// cache stays in the test's temporary directory, never beside that snapshot.
func BenchmarkStoreSearchDatabase(b *testing.B) {
	database := os.Getenv("FBUI_SEARCH_BENCH_DB")
	if database == "" {
		b.Skip("set FBUI_SEARCH_BENCH_DB")
	}
	library, err := game_library.OpenLibrary(database)
	if err != nil {
		b.Fatal(err)
	}
	defer library.Close()
	games, err := library.ListGames(context.Background(), 0, 0)
	if err != nil {
		b.Fatal(err)
	}
	releases, err := library.ListReleases(context.Background(), 0)
	if err != nil {
		b.Fatal(err)
	}
	items := buildStoreSearchItems(games, releases)
	start := time.Now()
	index := buildStoreSearchIndex(items)
	info, err := os.Stat(database)
	if err != nil {
		b.Fatal(err)
	}
	index.Size, index.Modified = info.Size(), info.ModTime().UnixNano()
	cacheDir := b.TempDir()
	if parent := os.Getenv("FBUI_SEARCH_BENCH_DIR"); parent != "" {
		cacheDir, err = os.MkdirTemp(parent, ".fbui-search-bench-*")
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() { os.RemoveAll(cacheDir) })
	}
	path := filepath.Join(cacheDir, "index")
	if err := writeStoreSearchIndex(path, index); err != nil {
		b.Fatal(err)
	}
	cacheInfo, _ := os.Stat(path)
	b.Logf("games=%d build+write=%s cache=%.2f MiB", len(items), time.Since(start), float64(cacheInfo.Size())/(1024*1024))
	ids := func(items []search_window.Item) []int32 {
		var out []int32
		for _, item := range items {
			out = append(out, item.Value.(*game_library.Game).ID)
		}
		return out
	}
	for _, query := range []string{"qh97", "zzjb", "mario", "魂斗罗", "7", "共同版本名", "---", ""} {
		if !reflect.DeepEqual(ids(index.search(query)), ids(search_window.MatchItems(items, query))) {
			b.Fatalf("different results: %q", query)
		}
	}
	for _, query := range []string{"zzjb", "qh97", "魂斗罗", "mario"} {
		b.Run(query, func(b *testing.B) {
			b.Logf("query=%q results=%d", query, len(index.search(query)))
			b.Run("original-with-database", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					games, err := library.ListGames(context.Background(), 0, 0)
					if err != nil {
						b.Fatal(err)
					}
					releases, err := library.ListReleases(context.Background(), 0)
					if err != nil {
						b.Fatal(err)
					}
					search_window.MatchItems(buildStoreSearchItems(games, releases), query)
				}
			})
			b.Run("disk-load-and-search", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					loaded, err := readStoreSearchIndex(path, info)
					if err != nil {
						b.Fatal(err)
					}
					loaded.search(query)
				}
			})
		})
	}
}

func TestBackgroundSearchIndexPreparation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gm.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE options (name TEXT, value TEXT)`,
		fmt.Sprintf(`INSERT INTO options VALUES ('db_ver', '%d')`, game_library.SupportedDatabaseVersion),
		`CREATE TABLE platforms (id INTEGER, description TEXT)`,
		`CREATE TABLE games (id INTEGER, platform_id INTEGER, series_id INTEGER, description TEXT)`,
		`CREATE TABLE releases (id INTEGER, game_id INTEGER, description TEXT, release_type INTEGER)`,
		`CREATE TABLE names (id INTEGER, kind INTEGER, kind_id INTEGER, language INTEGER, name TEXT, source TEXT, deprecated INTEGER)`,
		`INSERT INTO games VALUES (1, 0, 0, 'test')`,
		fmt.Sprintf(`INSERT INTO names VALUES (1, %d, 1, %d, '魂斗罗', '', 0)`, game_library.KindGame, game_library.LanguageChinese),
	}
	for _, table := range []string{"series", "assets", "entries", "blobs", "rom_sets", "rom_import_exclusions"} {
		statements = append(statements, fmt.Sprintf("CREATE TABLE %s (id INTEGER)", table))
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Startup preparation and early searches may overlap. They must share the
	// same persisted index without retaining or closing the navigator's DB.
	done := prepareStoreSearchIndex(path)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			items, err := searchStore(context.Background(), nil, path, "hdl")
			if err != nil {
				t.Error(err)
				return
			}
			if len(items) != 1 || items[0].Name != "魂斗罗" {
				t.Errorf("unexpected results: %#v", items)
			}
		})
	}
	wg.Wait()
	<-done
	before, err := os.Stat(path + ".search-v1")
	if err != nil {
		t.Fatal(err)
	}
	// Another startup must validate/reuse the disk file, not regenerate it.
	<-prepareStoreSearchIndex(path)
	after, err := os.Stat(path + ".search-v1")
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("valid index was rebuilt")
	}
	if err := os.WriteFile(path+".search-v1", []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	<-prepareStoreSearchIndex(path)
	info, _ := os.Stat(path)
	if _, err := readStoreSearchIndex(path+".search-v1", info); err != nil {
		t.Fatalf("background did not repair corrupt index: %v", err)
	}
}
