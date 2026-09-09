package game_library

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type fakeSource struct {
	contents map[string][]byte
	requests map[string]int
}

type failingReader struct{ read bool }

func (r *failingReader) Read(buffer []byte) (int, error) {
	if !r.read {
		r.read = true
		return copy(buffer, "part"), nil
	}
	return 0, io.ErrUnexpectedEOF
}

func (*failingReader) Close() error { return nil }

type failingSource struct{}

func (failingSource) Open(context.Context, string) (io.ReadCloser, error) {
	return &failingReader{}, nil
}

func (s *fakeSource) Open(_ context.Context, sha string) (io.ReadCloser, error) {
	s.requests[sha]++
	content, ok := s.contents[sha]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(content)), nil
}

func testBlob(id int32, content string) (*Blob, string) {
	data := []byte(content)
	sha := fmt.Sprintf("%x", sha256.Sum256(data))
	return &Blob{ID: id, SHA256: sha, Size: int64(len(data))}, sha
}

func TestEnsureBlobVerifiesAndReusesCAS(t *testing.T) {
	blob, sha := testBlob(1, "verified content")
	source := &fakeSource{contents: map[string][]byte{blob.SHA256: []byte("verified content")}, requests: map[string]int{}}
	store := New(t.TempDir(), source)
	path, err := store.ensureBlob(context.Background(), blob, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if path != store.blobPath(sha) {
		t.Fatalf("unexpected path: %s", path)
	}
	if _, err := store.ensureBlob(context.Background(), blob, nil, nil); err != nil {
		t.Fatal(err)
	}
	if source.requests[blob.SHA256] != 1 {
		t.Fatalf("blob downloaded %d times", source.requests[blob.SHA256])
	}
	if err := os.WriteFile(path, []byte("corrupt"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ensureBlob(context.Background(), blob, nil, nil); err != nil {
		t.Fatal(err)
	}
	if source.requests[blob.SHA256] != 2 {
		t.Fatal("corrupt cached blob was not downloaded again")
	}
}

func TestEnsureBlobRemovesFailedDownload(t *testing.T) {
	blob, sha := testBlob(1, "expected")
	source := &fakeSource{contents: map[string][]byte{sha: []byte("wrong")}, requests: map[string]int{}}
	store := New(t.TempDir(), source)
	if _, err := store.ensureBlob(context.Background(), blob, nil, nil); err == nil {
		t.Fatal("expected verification error")
	}
	if _, err := os.Stat(store.blobPath(sha)); !os.IsNotExist(err) {
		t.Fatalf("invalid final blob remains: %v", err)
	}
	if _, err := os.Stat(store.blobPath(sha) + ".part"); !os.IsNotExist(err) {
		t.Fatalf("partial blob remains: %v", err)
	}
}

func TestEnsureBlobRemovesInterruptedDownload(t *testing.T) {
	blob, sha := testBlob(1, "expected")
	store := New(t.TempDir(), failingSource{})
	if _, err := store.ensureBlob(context.Background(), blob, nil, nil); err == nil {
		t.Fatal("expected transfer error")
	}
	if _, err := os.Stat(store.blobPath(sha) + ".part"); !os.IsNotExist(err) {
		t.Fatalf("partial blob remains: %v", err)
	}
}

func TestMaterializeRegularAndZIP(t *testing.T) {
	first, _ := testBlob(1, "first")
	second, _ := testBlob(2, "second")
	source := &fakeSource{contents: map[string][]byte{first.SHA256: []byte("first"), second.SHA256: []byte("second")}, requests: map[string]int{}}
	store := New(t.TempDir(), source)
	progressMessages := map[string]bool{}
	progress := func(message string, _ float32) { progressMessages[message] = true }
	regular, err := store.Materialize(context.Background(), &Asset{ID: 1, Name: "game.rom", Format: FormatRegular, Blob: first}, progress)
	if err != nil {
		t.Fatal(err)
	}
	if content, _ := os.ReadFile(regular); string(content) != "first" {
		t.Fatalf("unexpected regular content: %q", content)
	}
	asset := &Asset{ID: 2, Name: "bundle", Format: FormatZIP, Entries: []*Entry{
		{Name: "folder/a.rom", Blob: first}, {Name: "b.rom", Blob: second},
	}}
	archivePath, err := store.Materialize(context.Background(), asset, progress)
	if err != nil {
		t.Fatal(err)
	}
	if !progressMessages["下载文件"] || !progressMessages["重新组装"] || !progressMessages["校验文件"] {
		t.Fatalf("missing zip phase progress: %#v", progressMessages)
	}
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, entry := range archive.File {
		file, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
		got[entry.Name] = string(content)
	}
	if got["folder/a.rom"] != "first" || got["b.rom"] != "second" {
		t.Fatalf("unexpected zip contents: %#v", got)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Materialize(context.Background(), asset, progress); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("valid zip cache was rebuilt")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(archivePath), "sha.txt")); err != nil {
		t.Fatalf("zip sha.txt missing: %v", err)
	}
	if err := os.WriteFile(archivePath, []byte("corrupt"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Materialize(context.Background(), asset, progress); err != nil {
		t.Fatal(err)
	}
	if rebuilt, err := zip.OpenReader(archivePath); err != nil {
		t.Fatalf("corrupt zip was not rebuilt: %v", err)
	} else {
		rebuilt.Close()
	}
}

func TestMaterializeZIPNormalizesCachedNameCase(t *testing.T) {
	blob, _ := testBlob(1, "data")
	source := &fakeSource{contents: map[string][]byte{blob.SHA256: []byte("data")}, requests: map[string]int{}}
	store := New(t.TempDir(), source)
	progress := func(string, float32) {}
	upper := &Asset{ID: 3, Name: "005.ZIP", Format: FormatZIP, Entries: []*Entry{{Name: "rom.bin", Blob: blob}}}

	upperPath, err := store.Materialize(context.Background(), upper, progress)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(upperPath)
	if err != nil {
		t.Fatal(err)
	}
	requests := source.requests[blob.SHA256]

	lower := &Asset{ID: 3, Name: "005.zip", Format: FormatZIP, Entries: upper.Entries}
	lowerPath, err := store.Materialize(context.Background(), lower, progress)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(lowerPath)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(lowerPath) != "005.zip" {
		t.Fatalf("materialized name was not normalized: %s", lowerPath)
	}
	directoryEntries, err := os.ReadDir(filepath.Dir(lowerPath))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range directoryEntries {
		if entry.Name() == "005.ZIP" {
			t.Fatal("old uppercase cache name still exists")
		}
	}
	if !os.SameFile(before, after) {
		t.Fatal("valid ZIP cache was rebuilt during case normalization")
	}
	if source.requests[blob.SHA256] != requests {
		t.Fatal("blob was fetched again during case normalization")
	}
}

func TestMaterializeRejectsUnsafeAndDuplicateZIPEntries(t *testing.T) {
	blob, _ := testBlob(1, "data")
	source := &fakeSource{contents: map[string][]byte{blob.SHA256: []byte("data")}, requests: map[string]int{}}
	store := New(t.TempDir(), source)
	for _, entries := range [][]*Entry{
		{{Name: "../escape", Blob: blob}},
		{{Name: "same", Blob: blob}, {Name: "same", Blob: blob}},
	} {
		if _, err := store.Materialize(context.Background(), &Asset{ID: 3, Name: "bad.zip", Format: FormatZIP, Entries: entries}, func(string, float32) {}); err == nil {
			t.Fatal("expected unsafe zip entries to fail")
		}
	}
}
