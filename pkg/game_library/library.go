package game_library

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/ncruces/go-sqlite3/driver"
)

const supportedDatabaseVersion = 8

type Library struct {
	db *sql.DB
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
	library := &Library{db: db}
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
	rows, err := l.db.QueryContext(ctx, `SELECT p.id,n.language,n.name FROM platforms p LEFT JOIN names n ON n.kind=? AND n.kind_id=p.id ORDER BY p.id,n.id`, KindPlatform)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []*Platform{}
	byID := map[int32]*Platform{}
	for rows.Next() {
		var id int32
		var language sql.NullInt32
		var name sql.NullString
		if err := rows.Scan(&id, &language, &name); err != nil {
			return nil, err
		}
		item := byID[id]
		if item == nil {
			item = &Platform{ID: id}
			byID[id] = item
			items = append(items, item)
		}
		if name.Valid {
			item.Names = append(item.Names, Name{Language: Language(language.Int32), Name: name.String})
		}
	}
	return items, rows.Err()
}

func (l *Library) ListSeries(ctx context.Context) ([]*Series, error) {
	rows, err := l.db.QueryContext(ctx, `SELECT s.id,n.language,n.name FROM series s LEFT JOIN names n ON n.kind=? AND n.kind_id=s.id ORDER BY s.id,n.id`, KindSeries)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []*Series{}
	byID := map[int32]*Series{}
	for rows.Next() {
		var id int32
		var language sql.NullInt32
		var name sql.NullString
		if err := rows.Scan(&id, &language, &name); err != nil {
			return nil, err
		}
		item := byID[id]
		if item == nil {
			item = &Series{ID: id}
			byID[id] = item
			items = append(items, item)
		}
		if name.Valid {
			item.Names = append(item.Names, Name{Language: Language(language.Int32), Name: name.String})
		}
	}
	return items, rows.Err()
}

func (l *Library) ListGames(ctx context.Context, platformID, seriesID int32) ([]*Game, error) {
	query := `SELECT g.id,g.platform_id,g.series_id,n.language,n.name FROM games g LEFT JOIN names n ON n.kind=? AND n.kind_id=g.id WHERE (?=0 OR g.platform_id=?) AND (?=0 OR g.series_id=?) ORDER BY g.id,n.id`
	rows, err := l.db.QueryContext(ctx, query, KindGame, platformID, platformID, seriesID, seriesID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []*Game{}
	byID := map[int32]*Game{}
	for rows.Next() {
		var id, platform, series int32
		var language sql.NullInt32
		var name sql.NullString
		if err := rows.Scan(&id, &platform, &series, &language, &name); err != nil {
			return nil, err
		}
		item := byID[id]
		if item == nil {
			item = &Game{ID: id, PlatformID: platform, SeriesID: series}
			byID[id] = item
			items = append(items, item)
		}
		if name.Valid {
			item.Names = append(item.Names, Name{Language: Language(language.Int32), Name: name.String})
		}
	}
	return items, rows.Err()
}

func (l *Library) ListReleases(ctx context.Context, gameID int32) ([]*Release, error) {
	rows, err := l.db.QueryContext(ctx, `SELECT r.id,r.game_id,n.language,n.name FROM releases r LEFT JOIN names n ON n.kind=? AND n.kind_id=r.id WHERE r.game_id=? ORDER BY r.id,n.id`, KindRelease, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []*Release{}
	byID := map[int32]*Release{}
	for rows.Next() {
		var id, game int32
		var language sql.NullInt32
		var name sql.NullString
		if err := rows.Scan(&id, &game, &language, &name); err != nil {
			return nil, err
		}
		item := byID[id]
		if item == nil {
			item = &Release{ID: id, GameID: game}
			byID[id] = item
			items = append(items, item)
		}
		if name.Valid {
			item.Names = append(item.Names, Name{Language: Language(language.Int32), Name: name.String})
		}
	}
	return items, rows.Err()
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
