package file_manager

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyPathCopiesDirectoryAndSymlink(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, `source`)
	if err := os.Mkdir(source, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, `file.txt`), []byte(`hello`), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(`file.txt`, filepath.Join(source, `link`)); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(root, `destination`)
	if err := copyPath(source, destination); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(destination, `file.txt`))
	if err != nil || string(data) != `hello` {
		t.Fatalf(`copied file = %q, %v`, data, err)
	}
	target, err := os.Readlink(filepath.Join(destination, `link`))
	if err != nil || target != `file.txt` {
		t.Fatalf(`copied symlink = %q, %v`, target, err)
	}
}

func TestCopyPathRejectsExistingDestinationAndDescendant(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, `source`)
	if err := os.Mkdir(source, 0755); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(root, `existing`)
	if err := os.WriteFile(existing, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := copyPath(source, existing); err == nil {
		t.Fatal(`copyPath accepted an existing destination`)
	}
	if err := copyPath(source, filepath.Join(source, `child`)); err == nil {
		t.Fatal(`copyPath accepted a destination inside the source`)
	}
}

func TestMovePath(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, `source.txt`)
	destination := filepath.Join(root, `destination.txt`)
	if err := os.WriteFile(source, []byte(`hello`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := movePath(source, destination); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf(`source still exists: %v`, err)
	}
	if data, err := os.ReadFile(destination); err != nil || string(data) != `hello` {
		t.Fatalf(`moved file = %q, %v`, data, err)
	}
}
