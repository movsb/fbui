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

const supportedDatabaseVersion = 11

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
	ids := make([]int32, 0, len(items))
	for _, item := range items {
		byID[item.ID] = item
		ids = append(ids, item.ID)
	}
	if err := l.attachNames(KindPlatform, ids, func(name Name) {
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
	ids := make([]int32, 0, len(items))
	for _, item := range items {
		byID[item.ID] = item
		ids = append(ids, item.ID)
	}
	if err := l.attachNames(KindSeries, ids, func(name Name) {
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
	ids := make([]int32, 0, len(items))
	for _, item := range items {
		byID[item.ID] = item
		ids = append(ids, item.ID)
	}
	if err := l.attachNames(KindGame, ids, func(name Name) {
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
	ids := make([]int32, 0, len(items))
	for _, item := range items {
		byID[item.ID] = item
		ids = append(ids, item.ID)
	}
	if err := l.attachNames(KindRelease, ids, func(name Name) {
		if item := byID[name.KindID]; item != nil {
			item.Names = append(item.Names, name)
		}
	}); err != nil {
		return nil, err
	}
	return items, nil
}

const maxQueryIDs = 900

func (l *Library) attachNames(kind Kind, ownerIDs []int32, attach func(Name)) error {
	for begin := 0; begin < len(ownerIDs); begin += maxQueryIDs {
		end := min(begin+maxQueryIDs, len(ownerIDs))
		var names []Name
		if err := l.tdb.Where(`kind=? AND kind_id IN (?)`, kind, ownerIDs[begin:end]).OrderBy(`id`).Find(&names); err != nil {
			return err
		}
		for _, name := range names {
			attach(name)
		}
	}
	return nil
}

func (l *Library) ListAssets(ctx context.Context, releaseID int32) ([]*Asset, error) {
	var items []*Asset
	if err := l.tdb.Select(`id,type,name,format,size,blob_id`).From(Asset{}).
		Where(`kind=? AND kind_id=?`, KindRelease, releaseID).OrderBy(`id`).Find(&items); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return items, nil
	}

	assetIDs := make([]int32, 0, len(items))
	assetsByID := make(map[int32]*Asset, len(items))
	for _, asset := range items {
		assetIDs = append(assetIDs, asset.ID)
		assetsByID[asset.ID] = asset
	}
	var entries []*Entry
	for begin := 0; begin < len(assetIDs); begin += maxQueryIDs {
		end := min(begin+maxQueryIDs, len(assetIDs))
		var batch []*Entry
		if err := l.tdb.Select(`id,asset_id,name,size,blob_id`).From(Entry{}).
			Where(`asset_id IN (?)`, assetIDs[begin:end]).OrderBy(`id`).Find(&batch); err != nil {
			return nil, err
		}
		entries = append(entries, batch...)
	}

	blobIDSet := map[int32]bool{}
	for _, asset := range items {
		if asset.BlobID != 0 {
			blobIDSet[asset.BlobID] = true
		}
	}
	for _, entry := range entries {
		if entry.BlobID != 0 {
			blobIDSet[entry.BlobID] = true
		}
	}
	blobIDs := make([]int32, 0, len(blobIDSet))
	for id := range blobIDSet {
		blobIDs = append(blobIDs, id)
	}
	var blobs []*Blob
	for begin := 0; begin < len(blobIDs); begin += maxQueryIDs {
		end := min(begin+maxQueryIDs, len(blobIDs))
		var batch []*Blob
		if err := l.tdb.Select(`id,size,sha256`).From(Blob{}).
			Where(`id IN (?)`, blobIDs[begin:end]).Find(&batch); err != nil {
			return nil, err
		}
		blobs = append(blobs, batch...)
	}
	blobsByID := make(map[int32]*Blob, len(blobs))
	for _, blob := range blobs {
		blobsByID[blob.ID] = blob
	}
	for _, asset := range items {
		asset.Blob = blobsByID[asset.BlobID]
	}
	for _, entry := range entries {
		entry.Blob = blobsByID[entry.BlobID]
		if asset := assetsByID[entry.AssetID]; asset != nil {
			asset.Entries = append(asset.Entries, entry)
		}
	}
	return items, nil
}
