package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fswatcher/fswatcher"
	"github.com/movsb/fbui/pkg/config"
	"github.com/movsb/fbui/pkg/game_library"
)

const screenshotSettleDelay = time.Second

type screenshotUploader struct {
	dir         string
	baseURL     string
	client      *http.Client
	settleDelay time.Duration
	newEvents   func(string) (*screenshotEvents, error)
	screenshot  func() error
}

type screenshotEvents struct {
	events <-chan fswatcher.Event
	errors <-chan error
	close  func() error
}

func newScreenshotUploader() *screenshotUploader {
	return &screenshotUploader{
		dir:         filepath.Join(config.SDCARDRoot, "RetroArch", ".retroarch", "screenshots"),
		baseURL:     gmBlobBaseURL,
		client:      &http.Client{Timeout: 30 * time.Second},
		settleDelay: screenshotSettleDelay,
		newEvents:   newScreenshotEvents,
		screenshot:  sendRetroArchScreenshot,
	}
}

func sendRetroArchScreenshot() error {
	address, err := net.ResolveUDPAddr("udp", "127.0.0.1:55355")
	if err != nil {
		return err
	}
	connection, err := net.DialUDP("udp", nil, address)
	if err != nil {
		return err
	}
	defer connection.Close()
	_, err = connection.Write([]byte("SCREENSHOT"))
	return err
}

func newScreenshotEvents(dir string) (*screenshotEvents, error) {
	watcher, err := fswatcher.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := watcher.Add(dir, fswatcher.Create|fswatcher.Write|fswatcher.Rename); err != nil {
		watcher.Close()
		return nil, err
	}
	return &screenshotEvents{events: watcher.Events, errors: watcher.Errors, close: watcher.Close}, nil
}

// capture asks RetroArch for one screenshot, waits for the resulting file to
// become stable, then uploads it as the current release's title screen.
func (u *screenshotUploader) capture(ctx context.Context, kind game_library.Kind, ownerID int32) (string, error) {
	if ownerID <= 0 || (kind != game_library.KindGame && kind != game_library.KindRelease) {
		return "", errors.New("invalid title screen owner")
	}
	baseline := u.snapshot()
	newEvents := u.newEvents
	if newEvents == nil {
		newEvents = newScreenshotEvents
	}
	watcher, err := newEvents(u.dir)
	if err != nil {
		return "", fmt.Errorf("观察 RetroArch 截图目录：%w", err)
	}
	defer watcher.close()
	screenshot := u.screenshot
	if screenshot == nil {
		screenshot = sendRetroArchScreenshot
	}
	if err := screenshot(); err != nil {
		return "", fmt.Errorf("发送 RetroArch 截图命令：%w", err)
	}

	pending := map[string]fileSignature{}
	dirty := map[string]bool{}
	scan := func() (string, bool, error) {
		current := u.snapshot()
		needsAnotherCheck := false
		for path, signature := range current {
			if _, existed := baseline[path]; existed || !dirty[path] {
				continue
			}
			if previous, seen := pending[path]; !seen || previous != signature {
				pending[path] = signature
				needsAnotherCheck = true
				continue
			}
			if err := u.upload(ctx, kind, ownerID, path); err != nil {
				return "", false, fmt.Errorf("上传标题画面 %s：%w", filepath.Base(path), err)
			}
			if err := os.Remove(path); err != nil {
				return "", false, fmt.Errorf("标题画面已上传，但删除本地截图 %s 失败：%w", filepath.Base(path), err)
			}
			return path, false, nil
		}
		return "", needsAnotherCheck, nil
	}

	var timer *time.Timer
	var settled <-chan time.Time
	armTimer := func() {
		if timer == nil {
			timer = time.NewTimer(u.settleDelay)
		} else {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(u.settleDelay)
		}
		settled = timer.C
	}
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	for {
		select {
		case event, ok := <-watcher.events:
			if !ok {
				return "", errors.New("RetroArch 截图观察器已关闭")
			}
			path := filepath.Clean(event.Name)
			if filepath.Dir(path) == filepath.Clean(u.dir) && isScreenshotName(path) {
				dirty[path] = true
				armTimer()
			}
		case err, ok := <-watcher.errors:
			if ok {
				return "", fmt.Errorf("观察 RetroArch 截图目录：%w", err)
			}
		case <-settled:
			settled = nil
			path, again, err := scan()
			if err != nil {
				return "", err
			}
			if path != "" {
				return path, nil
			}
			if again {
				armTimer()
			}
		case <-ctx.Done():
			return "", fmt.Errorf("等待 RetroArch 截图：%w", ctx.Err())
		}
	}
}

type fileSignature struct {
	size    int64
	modTime int64
}

func (u *screenshotUploader) snapshot() map[string]fileSignature {
	result := map[string]fileSignature{}
	entries, err := os.ReadDir(u.dir)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("扫描 RetroArch 截图失败：%v", err)
		}
		return result
	}
	for _, entry := range entries {
		if entry.IsDir() || !isScreenshotName(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		result[filepath.Join(u.dir, entry.Name())] = fileSignature{size: info.Size(), modTime: info.ModTime().UnixNano()}
	}
	return result
}

func isScreenshotName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".webp", ".bmp":
		return true
	default:
		return false
	}
}

func (u *screenshotUploader) upload(ctx context.Context, kind game_library.Kind, ownerID int32, path string) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range map[string]string{
		"kind":    strconv.Itoa(int(kind)),
		"kind_id": strconv.Itoa(int(ownerID)),
		"type":    strconv.Itoa(int(game_library.AssetTypeTitle)),
		// "description": "RetroArch 截图",
		"debug": "fbui:retroarch-screenshot",
	} {
		if err := writer.WriteField(name, value); err != nil {
			return err
		}
	}
	part, err := writer.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(part, file)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := writer.Close(); err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(u.baseURL, "/")+"/v3/assets", &body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := u.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
		return fmt.Errorf("HTTP %s: %s", response.Status, strings.TrimSpace(string(message)))
	}
	return nil
}
