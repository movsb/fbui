package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/movsb/fbui/pkg/game_library"
)

var errStoreOpenBusy = errors.New("another asset is already opening")

type assetIDOpener interface {
	OpenAssetByID(context.Context, int32) error
}

func storeOpenAssetHandler(opener assetIDOpener) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Private-Network", "true")
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			writeStoreJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		const prefix = "/api/store/assets/"
		value := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), ":open")
		if !strings.HasPrefix(r.URL.Path, prefix) || !strings.HasSuffix(r.URL.Path, ":open") || value == "" || strings.Contains(value, "/") {
			writeStoreJSONError(w, http.StatusBadRequest, "invalid asset id")
			return
		}
		parsed, err := strconv.ParseInt(value, 10, 32)
		if err != nil || parsed <= 0 {
			writeStoreJSONError(w, http.StatusBadRequest, "invalid asset id")
			return
		}
		id := int32(parsed)
		if err := opener.OpenAssetByID(r.Context(), id); err != nil {
			switch {
			case errors.Is(err, game_library.ErrAssetNotFound):
				writeStoreJSONError(w, http.StatusNotFound, err.Error())
			case errors.Is(err, game_library.ErrAssetNotROM), errors.Is(err, game_library.ErrAssetNotRelease):
				writeStoreJSONError(w, http.StatusUnsupportedMediaType, err.Error())
			case errors.Is(err, errStoreOpenBusy):
				writeStoreJSONError(w, http.StatusConflict, err.Error())
			default:
				writeStoreJSONError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "accepted", "asset_id": id})
	})
}

func writeStoreJSONError(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
