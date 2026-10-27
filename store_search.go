package main

import (
	"context"
	"encoding/gob"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/movsb/fbui/pkg/game_library"
	"github.com/movsb/fbui/pkg/search_window"
	"github.com/movsb/fbui/pkg/searchable"
)

// Bump when matching, display-name selection or search ranking rules change.
const storeSearchVersion = 1

// Background preparation and submitted searches share a single build at a time.
// The lock retains no index data after a request finishes.
var storeSearchMu sync.Mutex

func prepareStoreSearchIndex(databasePath string) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := loadStoreSearchIndex(context.Background(), nil, databasePath); err != nil {
			log.Printf("后台准备搜索索引失败: %v", err)
		}
	}()
	return done
}

type storeSearchEntry struct {
	Game         *game_library.Game
	Name, Detail string
	Groups       [][]searchable.Prepared
}

type storeSearchIndex struct {
	Version        int
	Size, Modified int64
	Entries        []storeSearchEntry
	Postings       map[rune][]int
	Fallback       []int
}

func buildStoreSearchIndex(items []search_window.Item) *storeSearchIndex {
	index := &storeSearchIndex{Version: storeSearchVersion, Postings: map[rune][]int{}}
	for id, item := range items {
		entry := storeSearchEntry{Game: item.Value.(*game_library.Game), Name: item.Name, Detail: item.Detail}
		keys := map[rune]bool{}
		fallback := false
		for _, group := range item.SearchGroups {
			prepared := make([]searchable.Prepared, 0, len(group))
			for _, name := range group {
				p := searchable.Prepare(name)
				prepared = append(prepared, p)
				for _, key := range p.Keys() {
					keys[key] = true
				}
				fallback = fallback || p.Fallback != ""
			}
			entry.Groups = append(entry.Groups, prepared)
		}
		index.Entries = append(index.Entries, entry)
		for key := range keys {
			index.Postings[key] = append(index.Postings[key], id)
		}
		if fallback {
			index.Fallback = append(index.Fallback, id)
		}
	}
	return index
}

func (index *storeSearchIndex) search(text string) []search_window.Item {
	query := searchable.NewQuery(text)
	candidates := make([]bool, len(index.Entries))
	if text == "" {
		for i := range candidates {
			candidates[i] = true
		}
	} else {
		for _, keys := range query.Keys() {
			// Start with the shortest posting list, then intersect the others.
			slices.SortFunc(keys, func(a, b rune) int { return len(index.Postings[a]) - len(index.Postings[b]) })
			if len(keys) == 0 {
				continue
			}
			for _, id := range index.Postings[keys[0]] {
				found := true
				for _, key := range keys[1:] {
					if _, ok := slices.BinarySearch(index.Postings[key], id); !ok {
						found = false
						break
					}
				}
				if found {
					candidates[id] = true
				}
			}
		}
		for _, id := range index.Fallback {
			candidates[id] = true
		}
	}
	type match struct{ id, rank int }
	var matches []match
	for id, candidate := range candidates {
		if !candidate {
			continue
		}
	groups:
		for rank, group := range index.Entries[id].Groups {
			for _, p := range group {
				if query.Match(p) {
					matches = append(matches, match{id, rank})
					break groups
				}
			}
		}
	}
	slices.SortFunc(matches, func(a, b match) int {
		if a.rank != b.rank {
			return a.rank - b.rank
		}
		if d := len(index.Entries[a.id].Name) - len(index.Entries[b.id].Name); d != 0 {
			return d
		}
		return a.id - b.id
	})
	items := make([]search_window.Item, 0, len(matches))
	for _, m := range matches {
		e := index.Entries[m.id]
		items = append(items, search_window.Item{Name: e.Name, Detail: e.Detail, Value: e.Game})
	}
	return items
}

func readStoreSearchIndex(path string, info os.FileInfo) (*storeSearchIndex, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	decoder := gob.NewDecoder(f)
	// Read the small header before decoding the potentially large body.
	var header struct {
		Version        int
		Size, Modified int64
	}
	if err := decoder.Decode(&header); err != nil {
		return nil, err
	}
	if header.Version != storeSearchVersion || header.Size != info.Size() || header.Modified != info.ModTime().UnixNano() {
		return nil, fmt.Errorf("stale search index")
	}
	var index storeSearchIndex
	if err := decoder.Decode(&index); err != nil {
		return nil, err
	}
	// Guard against truncated or invalid posting lists before using their IDs.
	for _, ids := range index.Postings {
		if !slices.IsSorted(ids) {
			return nil, fmt.Errorf("invalid search postings")
		}
		for _, id := range ids {
			if id < 0 || id >= len(index.Entries) {
				return nil, fmt.Errorf("invalid search entry")
			}
		}
	}
	for _, id := range index.Fallback {
		if id < 0 || id >= len(index.Entries) {
			return nil, fmt.Errorf("invalid fallback entry")
		}
	}
	return &index, nil
}

func writeStoreSearchIndex(path string, index *storeSearchIndex) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".search-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	encoder := gob.NewEncoder(f)
	err = encoder.Encode(struct {
		Version        int
		Size, Modified int64
	}{index.Version, index.Size, index.Modified})
	if err == nil {
		err = encoder.Encode(index)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// loadStoreSearchIndex never retains the index globally. A background caller
// opens its own read-only connection so replacing the navigator's library cannot
// close a database connection in use by the builder.
func loadStoreSearchIndex(ctx context.Context, library *game_library.Library, databasePath string) (*storeSearchIndex, error) {
	storeSearchMu.Lock()
	defer storeSearchMu.Unlock()
	start := time.Now()
	info, err := os.Stat(databasePath)
	if err != nil {
		return nil, err
	}
	path := databasePath + ".search-v1"
	if index, err := readStoreSearchIndex(path, info); err == nil {
		return index, nil
	}
	if library == nil {
		library, err = game_library.OpenLibrary(databasePath)
		if err != nil {
			return nil, err
		}
		defer library.Close()
	}
	games, err := library.ListGames(ctx, 0, 0)
	if err != nil {
		return nil, err
	}
	releases, err := library.ListReleases(ctx, 0)
	if err != nil {
		return nil, err
	}
	index := buildStoreSearchIndex(buildStoreSearchItems(games, releases))
	// A snapshot downloaded while this build was running must not receive an
	// index generated from the previous database.
	current, err := os.Stat(databasePath)
	if err != nil {
		return nil, err
	}
	if !os.SameFile(info, current) || info.Size() != current.Size() || !info.ModTime().Equal(current.ModTime()) {
		return nil, fmt.Errorf("database changed while building search index")
	}
	index.Size, index.Modified = info.Size(), info.ModTime().UnixNano()
	if err := writeStoreSearchIndex(path, index); err != nil {
		// A read-only/full card must not prevent searching.
		log.Printf("保存搜索索引失败，使用本次构建结果: %v", err)
	}
	log.Printf("构建搜索索引: %d 游戏，耗时 %s", len(index.Entries), time.Since(start))
	return index, nil
}

// No index is retained by StoreNavigator. Only matched games escape this call.
func searchStore(ctx context.Context, library *game_library.Library, databasePath, text string) ([]search_window.Item, error) {
	start := time.Now()
	index, err := loadStoreSearchIndex(ctx, library, databasePath)
	if err != nil {
		return nil, err
	}
	items := index.search(text)
	log.Printf("仓库搜索: %d 结果，总耗时 %s", len(items), time.Since(start))
	return items, nil
}
