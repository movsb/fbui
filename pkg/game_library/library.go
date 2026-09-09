package game_library

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/movsb/taorm"
	_ "github.com/ncruces/go-sqlite3/driver"
)

const supportedDatabaseVersion = 9

type Library struct {
	db  *sql.DB
	tdb *taorm.DB
}

func OpenLibrary(path string) (*Library, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("open game library: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("game library is not a regular file: %s", path)
	}
	dsn := "file:" + filepath.ToSlash(path) + "?mode=ro"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open game library: %w", err)
	}
	db.SetMaxOpenConns(1)
	library := &Library{db: db, tdb: taorm.NewDB(db)}
	if err := library.validate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return library, nil
}

func (l *Library) Close() error { return l.db.Close() }

func (l *Library) validate(ctx context.Context) error {
	var version int
	if err := l.db.QueryRowContext(ctx, `SELECT value FROM options WHERE name='db_ver'`).Scan(&version); err != nil {
		return fmt.Errorf("invalid game library database: %w", err)
	}
	if version != supportedDatabaseVersion {
		return fmt.Errorf("unsupported game library database version: got %d, want %d", version, supportedDatabaseVersion)
	}
	for _, table := range []string{"platforms", "series", "games", "names", "releases", "assets", "entries", "blobs"} {
		var found string
		err := l.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&found)
		if err != nil {
			return fmt.Errorf("invalid game library database: missing table %s", table)
		}
	}
	return nil
}

func (l *Library) ListPlatforms(ctx context.Context) ([]*Platform, error) {
	var items []*Platform
	if err := l.tdb.Select(`id`).From(Platform{}).OrderBy(`id`).Find(&items); err != nil {
		return nil, err
	}
	byID := map[int32]*Platform{}
	for _, item := range items {
		byID[item.ID] = item
	}
	if err := l.attachNames(KindPlatform, func(name Name) {
		if item := byID[name.KindID]; item != nil {
			item.Names = append(item.Names, name)
		}
	}); err != nil {
		return nil, err
	}
	return items, nil
}

func (l *Library) ListSeries(ctx context.Context) ([]*Series, error) {
	var items []*Series
	if err := l.tdb.Select(`id`).From(Series{}).OrderBy(`id`).Find(&items); err != nil {
		return nil, err
	}
	byID := map[int32]*Series{}
	for _, item := range items {
		byID[item.ID] = item
	}
	if err := l.attachNames(KindSeries, func(name Name) {
		if item := byID[name.KindID]; item != nil {
			item.Names = append(item.Names, name)
		}
	}); err != nil {
		return nil, err
	}
	return items, nil
}

func (l *Library) ListGames(ctx context.Context, platformID, seriesID int32) ([]*Game, error) {
	var items []*Game
	if err := l.tdb.Select(`id,platform_id,series_id`).From(Game{}).
		WhereIf(platformID != 0, `platform_id=?`, platformID).
		WhereIf(seriesID != 0, `series_id=?`, seriesID).
		OrderBy(`id`).Find(&items); err != nil {
		return nil, err
	}
	byID := map[int32]*Game{}
	for _, item := range items {
		byID[item.ID] = item
	}
	if err := l.attachNames(KindGame, func(name Name) {
		if item := byID[name.KindID]; item != nil {
			item.Names = append(item.Names, name)
		}
	}); err != nil {
		return nil, err
	}
	return items, nil
}

func (l *Library) ListReleases(ctx context.Context, gameID int32) ([]*Release, error) {
	var items []*Release
	if err := l.tdb.Select(`id,game_id`).From(Release{}).Where(`game_id=?`, gameID).OrderBy(`id`).Find(&items); err != nil {
		return nil, err
	}
	byID := map[int32]*Release{}
	for _, item := range items {
		byID[item.ID] = item
	}
	if err := l.attachNames(KindRelease, func(name Name) {
		if item := byID[name.KindID]; item != nil {
			item.Names = append(item.Names, name)
		}
	}); err != nil {
		return nil, err
	}
	return items, nil
}

func (l *Library) attachNames(kind Kind, attach func(Name)) error {
	var names []Name
	if err := l.tdb.Where(`kind=?`, kind).OrderBy(`id`).Find(&names); err != nil {
		return err
	}
	for _, name := range names {
		attach(name)
	}
	return nil
}

func (l *Library) ListAssets(ctx context.Context, releaseID int32) ([]*Asset, error) {
	rows, err := l.db.QueryContext(ctx, `SELECT a.id,a.type,a.name,a.format,a.size,b.id,b.size,b.sha256 FROM assets a LEFT JOIN blobs b ON b.id=a.blob_id WHERE a.kind=? AND a.kind_id=? ORDER BY a.id`, KindRelease, releaseID)
	if err != nil {
		return nil, err
	}
	items := []*Asset{}
	byID := map[int32]*Asset{}
	for rows.Next() {
		var item Asset
		var blobID sql.NullInt32
		var blobSize sql.NullInt64
		var blobSHA sql.NullString
		if err := rows.Scan(&item.ID, &item.Type, &item.Name, &item.Format, &item.Size, &blobID, &blobSize, &blobSHA); err != nil {
			rows.Close()
			return nil, err
		}
		if blobID.Valid {
			item.Blob = &Blob{ID: blobID.Int32, Size: blobSize.Int64, SHA256: blobSHA.String}
		}
		byID[item.ID] = &item
		items = append(items, &item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return items, nil
	}
	entryRows, err := l.db.QueryContext(ctx, `SELECT e.id,e.asset_id,e.name,b.id,b.size,b.sha256 FROM entries e JOIN assets a ON a.id=e.asset_id JOIN blobs b ON b.id=e.blob_id WHERE a.kind=? AND a.kind_id=? ORDER BY e.id`, KindRelease, releaseID)
	if err != nil {
		return nil, err
	}
	defer entryRows.Close()
	for entryRows.Next() {
		entry := &Entry{Blob: &Blob{}}
		if err := entryRows.Scan(&entry.ID, &entry.AssetID, &entry.Name, &entry.Blob.ID, &entry.Blob.Size, &entry.Blob.SHA256); err != nil {
			return nil, err
		}
		if asset := byID[entry.AssetID]; asset != nil {
			asset.Entries = append(asset.Entries, entry)
		}
	}
	return items, entryRows.Err()
}
