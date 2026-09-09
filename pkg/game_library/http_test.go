package game_library

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestHTTPBlobSourceUsesSHAPath(t *testing.T) {
	sha := strings.Repeat("a", 64)
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/blobs/"+sha {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader("blob"))}, nil
	})}
	source := &HTTPBlobSource{BaseURL: "https://example.test/blobs", Client: client}
	reader, err := source.Open(context.Background(), sha)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil || string(content) != "blob" {
		t.Fatalf("content=%q err=%v", content, err)
	}
}

func TestHTTPBlobSourceRejectsInvalidSHAAndStatus(t *testing.T) {
	source := NewHTTPBlobSource("http://unused")
	if _, err := source.Open(context.Background(), "bad"); err == nil {
		t.Fatal("expected invalid SHA error")
	}
	source = &HTTPBlobSource{BaseURL: "https://example.test", Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNotFound, Status: "404 Not Found", Body: io.NopCloser(strings.NewReader(""))}, nil
	})}}
	if _, err := source.Open(context.Background(), strings.Repeat("b", 64)); err == nil {
		t.Fatal("expected HTTP status error")
	}
}
