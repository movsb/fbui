package game_library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/movsb/taorm"
	_ "github.com/ncruces/go-sqlite3/driver"
)

var (
	ErrAssetNotFound          = errors.New("asset not found")
	ErrAssetNotROM            = errors.New("asset is not a ROM")
	ErrAssetNotRelease        = errors.New("asset does not belong to a release")
	ErrUnsupportedMAMEVersion = errors.New("unsupported MAME ROM set version")
)

const supportedMAMEVersion = "0.259"

type unsupportedMAMEVersionError struct {
	required  string
	supported string
}

func (e unsupportedMAMEVersionError) Error() string {
	return fmt.Sprintf("此 ROM Set 需要 MAME %s，当前设备最高支持 MAME %s。", e.required, e.supported)
}

func (unsupportedMAMEVersionError) Unwrap() error { return ErrUnsupportedMAMEVersion }

type LaunchableAsset struct {
	Asset      *Asset
	PlatformID int32
}

const supportedDatabaseVersion = 14

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
	for _, table := range []string{"platforms", "series", "games", "names", "releases", "assets", "entries", "blobs", "rom_sets"} {
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
	if err := l.tdb.Select(`id,kind,kind_id,type,name,format,size,blob_id`).From(Asset{}).
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

func (l *Library) GetLaunchableAsset(ctx context.Context, assetID int32) (*LaunchableAsset, error) {
	var assets []*Asset
	if err := l.tdb.Select(`id,kind,kind_id,type,name,format,size,blob_id`).From(Asset{}).
		Where(`id=?`, assetID).Find(&assets); err != nil {
		return nil, err
	}
	if len(assets) == 0 {
		return nil, ErrAssetNotFound
	}
	asset := assets[0]
	if asset.Type != AssetTypeROM {
		return nil, ErrAssetNotROM
	}
	if asset.Kind != KindRelease {
		return nil, ErrAssetNotRelease
	}
	var releases []*Release
	if err := l.tdb.Select(`id,game_id`).From(Release{}).Where(`id=?`, asset.KindID).Find(&releases); err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, ErrAssetNotRelease
	}
	var games []*Game
	if err := l.tdb.Select(`id,platform_id`).From(Game{}).Where(`id=?`, releases[0].GameID).Find(&games); err != nil {
		return nil, err
	}
	if len(games) == 0 {
		return nil, ErrAssetNotRelease
	}
	if err := l.validateMAMEVersion(ctx, assetID); err != nil {
		return nil, err
	}

	items, err := l.ListAssets(ctx, asset.KindID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.ID == assetID {
			return &LaunchableAsset{Asset: item, PlatformID: games[0].PlatformID}, nil
		}
	}
	return nil, ErrAssetNotFound
}

func (l *Library) validateMAMEVersion(ctx context.Context, assetID int32) error {
	rows, err := l.db.QueryContext(ctx, `SELECT DISTINCT version FROM rom_sets WHERE asset_id=? AND emulator='mame'`, assetID)
	if err != nil {
		return fmt.Errorf("query MAME ROM set version: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return fmt.Errorf("read MAME ROM set version: %w", err)
		}
		comparison, err := compareDottedVersions(version, supportedMAMEVersion)
		if err != nil {
			return fmt.Errorf("invalid MAME ROM set version %q: %w", version, err)
		}
		if comparison > 0 {
			return unsupportedMAMEVersionError{required: version, supported: supportedMAMEVersion}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("query MAME ROM set version: %w", err)
	}
	return nil
}

func compareDottedVersions(left, right string) (int, error) {
	parse := func(version string) ([]int64, error) {
		parts := strings.Split(version, ".")
		values := make([]int64, len(parts))
		for i, part := range parts {
			if part == "" {
				return nil, fmt.Errorf("empty version component")
			}
			value, err := strconv.ParseInt(part, 10, 64)
			if err != nil || value < 0 {
				return nil, fmt.Errorf("invalid version component %q", part)
			}
			values[i] = value
		}
		return values, nil
	}

	leftParts, err := parse(left)
	if err != nil {
		return 0, err
	}
	rightParts, err := parse(right)
	if err != nil {
		return 0, err
	}
	for i := range max(len(leftParts), len(rightParts)) {
		var leftPart, rightPart int64
		if i < len(leftParts) {
			leftPart = leftParts[i]
		}
		if i < len(rightParts) {
			rightPart = rightParts[i]
		}
		if leftPart < rightPart {
			return -1, nil
		}
		if leftPart > rightPart {
			return 1, nil
		}
	}
	return 0, nil
}
