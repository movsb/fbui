package file_upload

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestReceiveUploadStreamsWithoutParsingMultipartForm(t *testing.T) {
	dir := t.TempDir()
	request := newUploadRequest(t, `large.bin`, bytes.Repeat([]byte(`a`), 1<<20))
	win := &_UploadWindow{dir: dir}

	name, err := win.receiveUpload(request)
	if err != nil {
		t.Fatal(err)
	}
	if name != `large.bin` {
		t.Fatalf(`name = %q`, name)
	}
	if request.MultipartForm != nil && len(request.MultipartForm.File) > 0 {
		t.Fatal(`receiveUpload buffered multipart files`)
	}
	info, err := os.Stat(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 1<<20 {
		t.Fatalf(`uploaded size = %d`, info.Size())
	}
}

func TestReceiveUploadRemovesPartialFile(t *testing.T) {
	dir := t.TempDir()
	request := newBrokenUploadRequest(t, `partial.bin`)
	win := &_UploadWindow{dir: dir}

	if _, err := win.receiveUpload(request); err == nil {
		t.Fatal(`receiveUpload accepted a truncated upload`)
	}
	if _, err := os.Stat(filepath.Join(dir, `partial.bin`)); !os.IsNotExist(err) {
		t.Fatalf(`partial file remains: %v`, err)
	}
}

func newUploadRequest(t *testing.T, name string, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(`file`, name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, `/`, &body)
	request.Header.Set(`Content-Type`, writer.FormDataContentType())
	return request
}

func newBrokenUploadRequest(t *testing.T, name string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(`file`, name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(`partial`)); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, `/`, &body)
	request.Header.Set(`Content-Type`, writer.FormDataContentType())
	return request
}
