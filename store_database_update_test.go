package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func makeSnapshot(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "snapshot.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE options (name TEXT, value TEXT)`,
		`INSERT INTO options VALUES ('db_ver', '14')`,
	}
	for _, table := range []string{"platforms", "series", "games", "names", "releases", "assets", "entries", "blobs", "rom_sets"} {
		statements = append(statements, fmt.Sprintf(`CREATE TABLE %s (id INTEGER)`, table))
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
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestDownloadLibrarySnapshotReplacesAndReportsProgress(t *testing.T) {
	snapshot := makeSnapshot(t)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet && r.URL.Path == "/v3/backups/version" {
			return jsonResponse(`{"database_version":14}`), nil
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v3/backups:download" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		return &http.Response{
			StatusCode:    http.StatusOK,
			Status:        "200 OK",
			ContentLength: int64(len(snapshot)),
			Body:          io.NopCloser(bytes.NewReader(snapshot)),
			Header:        make(http.Header),
		}, nil
	})}

	target := filepath.Join(t.TempDir(), "gm.db")
	if err := os.WriteFile(target, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	var received, total int64
	library, err := downloadLibrarySnapshot(context.Background(), client, "http://gm.test/", target, func(r, n int64) {
		received, total = r, n
	})
	if err != nil {
		t.Fatal(err)
	}
	defer library.Close()
	if received != int64(len(snapshot)) || total != int64(len(snapshot)) {
		t.Fatalf("progress=%d/%d want=%d", received, total, len(snapshot))
	}
	if _, err := os.Stat(target + ".part"); !os.IsNotExist(err) {
		t.Fatalf("part file remains: %v", err)
	}
}

func TestDownloadLibrarySnapshotFailurePreservesOldDatabase(t *testing.T) {
	downloadCalled := false
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method == http.MethodGet {
			return jsonResponse(`{"database_version":14}`), nil
		}
		downloadCalled = true
		return &http.Response{
			StatusCode:    http.StatusOK,
			Status:        "200 OK",
			ContentLength: 100,
			Body:          io.NopCloser(bytes.NewReader([]byte("short"))),
			Header:        make(http.Header),
		}, nil
	})}

	target := filepath.Join(t.TempDir(), "gm.db")
	if err := os.WriteFile(target, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := downloadLibrarySnapshot(context.Background(), client, "http://gm.test", target, nil); err == nil {
		t.Fatal("expected truncated download error")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "old" {
		t.Fatalf("old database changed: %q err=%v", data, err)
	}
	if _, err := os.Stat(target + ".part"); !os.IsNotExist(err) {
		t.Fatalf("part file remains: %v", err)
	}
	if !downloadCalled {
		t.Fatal("download request was not made")
	}
}

func TestDownloadLibrarySnapshotRejectsIncompatibleVersionBeforeDownload(t *testing.T) {
	downloadCalled := false
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method == http.MethodPost {
			downloadCalled = true
		}
		return jsonResponse(`{"database_version":15}`), nil
	})}
	if _, err := downloadLibrarySnapshot(context.Background(), client, "http://gm.test", filepath.Join(t.TempDir(), "gm.db"), nil); err == nil {
		t.Fatal("expected incompatible version error")
	}
	if downloadCalled {
		t.Fatal("snapshot was downloaded despite incompatible version")
	}
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode:    http.StatusOK,
		Status:        "200 OK",
		ContentLength: int64(len(body)),
		Body:          io.NopCloser(bytes.NewBufferString(body)),
		Header:        make(http.Header),
	}
}
