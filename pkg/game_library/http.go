package game_library

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/movsb/fbiw"
)

type HTTPBlobSource struct {
	baseURL *url.URL
	Client  *http.Client
}

// 在 baseURL 后面追加 `/v3/blobs/sha256` 来构造最终 URL。
func NewHTTPBlobSource(baseURL string) *HTTPBlobSource {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.ResponseHeaderTimeout = 15 * time.Second
	return &HTTPBlobSource{
		baseURL: fbiw.Must1(url.Parse(baseURL)),
		Client:  &http.Client{Transport: transport},
	}
}

func (s *HTTPBlobSource) Open(ctx context.Context, sha256 string) (io.ReadCloser, error) {
	if !sha256Pattern.MatchString(sha256) {
		return nil, fmt.Errorf("invalid blob sha256: %q", sha256)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL.JoinPath(`/v3/blobs`, sha256).String(), nil)
	if err != nil {
		return nil, err
	}
	response, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("download blob %s: HTTP %s", sha256, response.Status)
	}
	return response.Body, nil
}
