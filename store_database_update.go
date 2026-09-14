package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/movsb/fbui/pkg/game_library"
)

type snapshotProgress func(received, total int64)

func downloadLibrarySnapshot(ctx context.Context, client *http.Client, baseURL, target string, progress snapshotProgress) (*game_library.Library, error) {
	if err := checkSnapshotCompatibility(ctx, client, baseURL); err != nil {
		return nil, err
	}
	if progress != nil {
		progress(0, 0)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/v3/backups:download", nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("下载数据库快照：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("下载数据库快照：HTTP %s：%s", response.Status, strings.TrimSpace(string(message)))
	}
	total := response.ContentLength
	if total <= 0 {
		return nil, errors.New("下载数据库快照：服务器未提供有效的 Content-Length")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return nil, err
	}
	temporary := target + ".part"
	_ = os.Remove(temporary)
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		file.Close()
		if !keep {
			_ = os.Remove(temporary)
		}
	}()

	written, err := io.Copy(file, io.TeeReader(response.Body, &progressWriter{total: total, progress: progress}))
	if err != nil {
		return nil, fmt.Errorf("下载数据库快照：%w", err)
	}
	if written != total {
		return nil, fmt.Errorf("下载数据库快照不完整：收到 %d 字节，预期 %d 字节", written, total)
	}
	if err := file.Sync(); err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}

	validated, err := game_library.OpenLibrary(temporary)
	if err != nil {
		return nil, fmt.Errorf("校验数据库快照：%w", err)
	}
	if err := validated.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(temporary, target); err != nil {
		return nil, err
	}
	keep = true
	library, err := game_library.OpenLibrary(target)
	if err != nil {
		return nil, fmt.Errorf("启用数据库快照：%w", err)
	}
	return library, nil
}

func checkSnapshotCompatibility(ctx context.Context, client *http.Client, baseURL string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/v3/backups/version", nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("检查数据库版本：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("检查数据库版本：HTTP %s：%s", response.Status, strings.TrimSpace(string(message)))
	}
	var result struct {
		DatabaseVersion int `json:"database_version"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&result); err != nil {
		return fmt.Errorf("检查数据库版本：%w", err)
	}
	if result.DatabaseVersion != game_library.SupportedDatabaseVersion {
		return fmt.Errorf("数据库版本不兼容：服务器版本 %d，设备支持版本 %d", result.DatabaseVersion, game_library.SupportedDatabaseVersion)
	}
	return nil
}

type progressWriter struct {
	total    int64
	written  int64
	progress snapshotProgress
}

func (w *progressWriter) Write(data []byte) (int, error) {
	w.written += int64(len(data))
	if w.progress != nil {
		w.progress(w.written, w.total)
	}
	return len(data), nil
}
