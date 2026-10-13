package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fswatcher/fswatcher"
)

func TestScreenshotUploaderUploadsOnlyNewStableImages(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "old.png"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: screenshotRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Error(err)
		} else {
			defer file.Close()
			content, _ := io.ReadAll(file)
			if header.Filename != "new.png" || string(content) != "new" {
				t.Errorf("uploaded %q=%q", header.Filename, content)
			}
		}
		if r.FormValue("kind") != "3" || r.FormValue("kind_id") != "42" || r.FormValue("type") != "9" {
			t.Errorf("unexpected owner/type: %s/%s/%s", r.FormValue("kind"), r.FormValue("kind_id"), r.FormValue("type"))
		}
		return &http.Response{StatusCode: http.StatusCreated, Status: "201 Created", Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header)}, nil
	})}

	events := make(chan fswatcher.Event, 2)
	errors := make(chan error)
	uploader := &screenshotUploader{
		dir: dir, baseURL: "http://gm.test", client: client, settleDelay: 10 * time.Millisecond,
		newEvents: func(string) (*screenshotEvents, error) {
			return &screenshotEvents{events: events, errors: errors, close: func() error { return nil }}, nil
		},
		screenshot: func() error {
			path := filepath.Join(dir, "new.png")
			if err := os.WriteFile(path, []byte("new"), 0644); err != nil {
				return err
			}
			events <- fswatcher.Event{Name: path, Op: fswatcher.Create | fswatcher.Write}
			return nil
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	path, err := uploader.capture(ctx, 42)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "new.png" {
		t.Fatalf("captured path=%q", path)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("uploaded screenshot was not deleted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "old.png")); err != nil {
		t.Fatalf("pre-existing screenshot was changed: %v", err)
	}
}

type screenshotRoundTripFunc func(*http.Request) (*http.Response, error)

func (f screenshotRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestIsScreenshotName(t *testing.T) {
	for _, name := range []string{"a.PNG", "a.jpg", "a.jpeg", "a.webp", "a.bmp"} {
		if !isScreenshotName(name) {
			t.Errorf("did not accept %s", name)
		}
	}
	if isScreenshotName("notes.txt") {
		t.Fatal("accepted non-image")
	}
}
