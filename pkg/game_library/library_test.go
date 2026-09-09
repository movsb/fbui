package game_library

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
)

func createTestLibrary(t *testing.T, version int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gm.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	schema := []string{
		`CREATE TABLE options (name TEXT, value TEXT)`,
		`CREATE TABLE platforms (id INTEGER PRIMARY KEY, description TEXT)`,
		`CREATE TABLE series (id INTEGER PRIMARY KEY, description TEXT)`,
		`CREATE TABLE games (id INTEGER PRIMARY KEY, platform_id INTEGER, series_id INTEGER, description TEXT)`,
		`CREATE TABLE names (id INTEGER PRIMARY KEY, kind INTEGER, kind_id INTEGER, language INTEGER, name TEXT)`,
		`CREATE TABLE releases (id INTEGER PRIMARY KEY, game_id INTEGER, description TEXT, release_date INTEGER)`,
		`CREATE TABLE assets (id INTEGER PRIMARY KEY, kind INTEGER, kind_id INTEGER, type INTEGER, name TEXT, description TEXT, debug TEXT, format INTEGER, size INTEGER, blob_id INTEGER)`,
		`CREATE TABLE entries (id INTEGER PRIMARY KEY, asset_id INTEGER, name TEXT, size INTEGER, blob_id INTEGER)`,
		`CREATE TABLE blobs (id INTEGER PRIMARY KEY, size INTEGER, crc32 TEXT, md5 TEXT, sha256 TEXT)`,
		fmt.Sprintf(`INSERT INTO options VALUES ('db_ver','%d')`, version),
		`INSERT INTO platforms VALUES (1,'')`,
		`INSERT INTO series VALUES (2,'')`,
		`INSERT INTO games VALUES (3,1,2,'')`,
		`INSERT INTO releases VALUES (4,3,'',0)`,
		`INSERT INTO names VALUES (1,1,1,1,'NES'),(2,1,1,2,'红白机'),(3,7,2,2,'马力欧'),(4,2,3,1,'Mario'),(5,3,4,2,'日版')`,
		`INSERT INTO blobs VALUES (5,4,'','','aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'),(6,3,'','','bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb')`,
		`INSERT INTO assets VALUES (7,3,4,1,'game.rom','','',1,4,5),(8,3,4,1,'set','','',2,3,0)`,
		`INSERT INTO entries VALUES (9,8,'game.bin',3,6)`,
	}
	for _, statement := range schema {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLibraryQueriesCatalogAndBlobs(t *testing.T) {
	path := createTestLibrary(t, supportedDatabaseVersion)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	library, err := OpenLibrary(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	platforms, err := library.ListPlatforms(ctx)
	if err != nil || len(platforms) != 1 || len(platforms[0].Names) != 2 {
		t.Fatalf("platforms=%#v err=%v", platforms, err)
	}
	series, err := library.ListSeries(ctx)
	if err != nil || len(series) != 1 || series[0].ID != 2 {
		t.Fatalf("series=%#v err=%v", series, err)
	}
	games, err := library.ListGames(ctx, 1, 0)
	if err != nil || len(games) != 1 || games[0].SeriesID != 2 {
		t.Fatalf("games=%#v err=%v", games, err)
	}
	if filtered, err := library.ListGames(ctx, 99, 0); err != nil || len(filtered) != 0 {
		t.Fatalf("filtered=%#v err=%v", filtered, err)
	}
	releases, err := library.ListReleases(ctx, 3)
	if err != nil || len(releases) != 1 || releases[0].ID != 4 {
		t.Fatalf("releases=%#v err=%v", releases, err)
	}
	assets, err := library.ListAssets(ctx, 4)
	if err != nil || len(assets) != 2 {
		t.Fatalf("assets=%#v err=%v", assets, err)
	}
	if assets[0].Blob == nil || assets[0].Blob.Size != 4 || len(assets[1].Entries) != 1 || assets[1].Entries[0].Blob.Size != 3 {
		t.Fatalf("blob metadata was not attached: %#v", assets)
	}
	if err := library.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if before.ModTime() != after.ModTime() || before.Size() != after.Size() {
		t.Fatal("read-only library changed the database file")
	}
}

func TestOpenLibraryRejectsMissingAndWrongVersion(t *testing.T) {
	if _, err := OpenLibrary(filepath.Join(t.TempDir(), "missing.db")); err == nil {
		t.Fatal("expected missing database error")
	}
	path := createTestLibrary(t, supportedDatabaseVersion-1)
	if _, err := OpenLibrary(path); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenLibraryRejectsMissingTable(t *testing.T) {
	path := createTestLibrary(t, supportedDatabaseVersion)
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE entries`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenLibrary(path); err == nil || !strings.Contains(err.Error(), "missing table entries") {
		t.Fatalf("unexpected error: %v", err)
	}
}
