package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/movsb/fbui/pkg/game_library"
)

type fakeAssetIDOpener struct {
	err error
	id  int32
}

func (f *fakeAssetIDOpener) OpenAssetByID(_ context.Context, id int32) error {
	f.id = id
	return f.err
}

func TestStoreOpenAssetHandler(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		err    error
		status int
	}{
		{"accepted", http.MethodPost, "/api/store/assets/42:open", nil, http.StatusAccepted},
		{"invalid", http.MethodPost, "/api/store/assets/nope:open", nil, http.StatusBadRequest},
		{"missing", http.MethodPost, "/api/store/assets/42:open", game_library.ErrAssetNotFound, http.StatusNotFound},
		{"not rom", http.MethodPost, "/api/store/assets/42:open", game_library.ErrAssetNotROM, http.StatusUnsupportedMediaType},
		{"wrong owner", http.MethodPost, "/api/store/assets/42:open", game_library.ErrAssetNotRelease, http.StatusUnsupportedMediaType},
		{"unsupported MAME version", http.MethodPost, "/api/store/assets/42:open", fmt.Errorf("%w: too new", game_library.ErrUnsupportedMAMEVersion), http.StatusUnsupportedMediaType},
		{"busy", http.MethodPost, "/api/store/assets/42:open", errStoreOpenBusy, http.StatusConflict},
		{"internal", http.MethodPost, "/api/store/assets/42:open", errors.New("broken"), http.StatusInternalServerError},
		{"options", http.MethodOptions, "/api/store/assets/42:open", nil, http.StatusNoContent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opener := &fakeAssetIDOpener{err: test.err}
			mux := http.NewServeMux()
			mux.Handle("/api/store/assets/", storeOpenAssetHandler(opener))
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
			if response.Code != test.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if response.Header().Get("Access-Control-Allow-Origin") != "*" {
				t.Fatal("missing CORS header")
			}
			if response.Header().Get("Access-Control-Allow-Private-Network") != "true" {
				t.Fatal("missing private-network CORS header")
			}
			if test.status == http.StatusAccepted && (opener.id != 42 || !strings.Contains(response.Body.String(), `"asset_id":42`)) {
				t.Fatalf("id=%d body=%s", opener.id, response.Body.String())
			}
		})
	}
}
