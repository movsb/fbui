package game_library

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"testing"

	pb "github.com/movsb/gm/protocols/go/proto"
)

type fakeSource struct {
	contents map[int32][]byte
	requests map[int32]int
}

func (s *fakeSource) Open(_ context.Context, blobID int32) (io.ReadCloser, error) {
	s.requests[blobID]++
	content, ok := s.contents[blobID]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(content)), nil
}

func testBlob(id int32, content string) (*pb.Blob, string) {
	data := []byte(content)
	sha := fmt.Sprintf("%x", sha256.Sum256(data))
	return &pb.Blob{Id: id, Sha256: sha, Size: int32(len(data))}, sha
}

func TestEnsureBlobVerifiesAndReusesCAS(t *testing.T) {
	blob, sha := testBlob(1, "verified content")
	source := &fakeSource{contents: map[int32][]byte{blob.Id: []byte("verified content")}, requests: map[int32]int{}}
	store := New(t.TempDir(), source)
	path, err := store.ensureBlob(context.Background(), blob)
	if err != nil {
		t.Fatal(err)
	}
	if path != store.blobPath(sha) {
		t.Fatalf("unexpected path: %s", path)
	}
	if _, err := store.ensureBlob(context.Background(), blob); err != nil {
		t.Fatal(err)
	}
	if source.requests[blob.Id] != 1 {
		t.Fatalf("blob downloaded %d times", source.requests[blob.Id])
	}
	if err := os.WriteFile(path, []byte("corrupt"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ensureBlob(context.Background(), blob); err != nil {
		t.Fatal(err)
	}
	if source.requests[blob.Id] != 2 {
		t.Fatal("corrupt cached blob was not downloaded again")
	}
}

func TestMaterializeRegularAndZIP(t *testing.T) {
	first, _ := testBlob(1, "first")
	second, _ := testBlob(2, "second")
	source := &fakeSource{contents: map[int32][]byte{first.Id: []byte("first"), second.Id: []byte("second")}, requests: map[int32]int{}}
	store := New(t.TempDir(), source)
	regular, err := store.Materialize(context.Background(), &pb.Asset{Id: 1, Name: "game.rom", Format: pb.Format_FORMAT_REGULAR, Blob: first})
	if err != nil {
		t.Fatal(err)
	}
	if content, _ := os.ReadFile(regular); string(content) != "first" {
		t.Fatalf("unexpected regular content: %q", content)
	}
	archivePath, err := store.Materialize(context.Background(), &pb.Asset{Id: 2, Name: "bundle", Format: pb.Format_FORMAT_ZIP, Entries: []*pb.Entry{
		{Name: "folder/a.rom", Blob: first}, {Name: "b.rom", Blob: second},
	}})
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
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
}

func TestMaterializeRejectsUnsafeAndDuplicateZIPEntries(t *testing.T) {
	blob, _ := testBlob(1, "data")
	source := &fakeSource{contents: map[int32][]byte{blob.Id: []byte("data")}, requests: map[int32]int{}}
	store := New(t.TempDir(), source)
	for _, entries := range [][]*pb.Entry{
		{{Name: "../escape", Blob: blob}},
		{{Name: "same", Blob: blob}, {Name: "same", Blob: blob}},
	} {
		if _, err := store.Materialize(context.Background(), &pb.Asset{Id: 3, Name: "bad.zip", Format: pb.Format_FORMAT_ZIP, Entries: entries}); err == nil {
			t.Fatal("expected unsafe zip entries to fail")
		}
	}
}
