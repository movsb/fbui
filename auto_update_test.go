package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExecutableUpdater(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fbui")
	if err := os.WriteFile(path, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	execErr := errors.New("exec failed")
	updater := executableUpdater{path: path, modTime: info.ModTime(), replace: func() error {
		calls++
		return execErr
	}}
	if err := updater.check(); err != nil || calls != 0 {
		t.Fatalf("unchanged binary: calls=%d, err=%v", calls, err)
	}
	// Atomic deployment replaces the file at the same executable path.
	newPath := filepath.Join(filepath.Dir(path), "fbui.new")
	if err := os.WriteFile(newPath, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	updated := info.ModTime().Add(time.Second)
	if err := os.Chtimes(newPath, updated, updated); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(newPath, path); err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		if err := updater.check(); !errors.Is(err, execErr) || calls != attempt {
			t.Fatalf("replacement attempt %d: calls=%d, err=%v", attempt, calls, err)
		}
	}
	updater.path = filepath.Join(t.TempDir(), "missing")
	if err := updater.check(); !errors.Is(err, os.ErrNotExist) || calls != 2 {
		t.Fatalf("missing binary: calls=%d, err=%v", calls, err)
	}
}
