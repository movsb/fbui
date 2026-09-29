package game_library

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func testHTTPBlobSource(status int, encoding string, data []byte) *HTTPBlobSource {
	source := NewHTTPBlobSource("http://example.invalid")
	source.Client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		header := http.Header{}
		if encoding != "" {
			header.Set("GM-Content-Encoding", encoding)
		}
		return &http.Response{
			StatusCode: status,
			Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
			Header:     header,
			Body:       io.NopCloser(bytes.NewReader(data)),
		}, nil
	})}
	return source
}

func gzipBytes(t *testing.T, content string) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := gzip.NewWriter(&output)
	if _, err := io.WriteString(writer, content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestHTTPBlobSourceDecompressesStoredBlob(t *testing.T) {
	content := "blob contents"
	source := testHTTPBlobSource(http.StatusOK, "gzip", gzipBytes(t, content))
	reader, err := source.Open(context.Background(), strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Fatalf("content=%q", data)
	}
}

func TestHTTPBlobSourceRequiresGZIPStorageEncoding(t *testing.T) {
	_, err := testHTTPBlobSource(http.StatusOK, "", []byte("plain")).Open(context.Background(), strings.Repeat("b", 64))
	if err == nil || !strings.Contains(err.Error(), "GM-Content-Encoding") {
		t.Fatalf("error=%v", err)
	}
}

func TestHTTPBlobSourceRejectsInvalidGZIP(t *testing.T) {
	_, err := testHTTPBlobSource(http.StatusOK, "gzip", []byte("broken")).Open(context.Background(), strings.Repeat("c", 64))
	if err == nil || !strings.Contains(err.Error(), "gzip") {
		t.Fatalf("error=%v", err)
	}
}

func TestHTTPBlobSourceRejectsHTTPError(t *testing.T) {
	_, err := testHTTPBlobSource(http.StatusNotFound, "", []byte("missing")).Open(context.Background(), strings.Repeat("d", 64))
	if err == nil || !strings.Contains(err.Error(), fmt.Sprint(http.StatusNotFound)) {
		t.Fatalf("error=%v", err)
	}
}
