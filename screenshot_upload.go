package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"mime/multipart"
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
	}
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

// watch uploads image files created while a Store ROM is running. A file must
// have the same size and modification time in two scans before it is uploaded,
// so RetroArch never gets read halfway through writing a screenshot.
func (u *screenshotUploader) watch(ctx context.Context, releaseID int32, ready chan<- struct{}) {
	readySignaled := false
	defer func() {
		if !readySignaled {
			close(ready)
		}
	}()
	if releaseID <= 0 {
		return
	}
	baseline := u.snapshot()
	newEvents := u.newEvents
	if newEvents == nil {
		newEvents = newScreenshotEvents
	}
	watcher, err := newEvents(u.dir)
	if err != nil {
		log.Printf("创建 RetroArch 截图观察器失败：%v", err)
		return
	}
	defer watcher.close()
	// The caller does not start RetroArch until the watch has been registered.
	close(ready)
	readySignaled = true

	pending := map[string]fileSignature{}
	uploaded := map[string]bool{}
	dirty := map[string]bool{}
	scan := func(uploadContext context.Context, includeAllNew bool) bool {
		current := u.snapshot()
		needsAnotherCheck := false
		for path, signature := range current {
			if _, existed := baseline[path]; existed || uploaded[path] || (!includeAllNew && !dirty[path]) {
				continue
			}
			if previous, seen := pending[path]; !seen || previous != signature {
				pending[path] = signature
				needsAnotherCheck = true
				continue
			}
			if err := u.upload(uploadContext, releaseID, path); err != nil {
				log.Printf("上传标题画面失败：%s: %v", path, err)
				needsAnotherCheck = true
				continue
			}
			uploaded[path] = true
			delete(pending, path)
			delete(dirty, path)
			log.Printf("已上传标题画面：%s (release=%d)", path, releaseID)
		}
		return needsAnotherCheck
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
				return
			}
			path := filepath.Clean(event.Name)
			if filepath.Dir(path) == filepath.Clean(u.dir) && isScreenshotName(path) {
				dirty[path] = true
				armTimer()
			}
		case err, ok := <-watcher.errors:
			if ok {
				log.Printf("观察 RetroArch 截图目录出错：%v", err)
			}
		case <-settled:
			settled = nil
			if scan(ctx, false) {
				armTimer()
			}
		case <-ctx.Done():
			// Give a screenshot taken immediately before RetroArch exits one last
			// opportunity to become stable.
			finalContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			scan(finalContext, true)
			timer := time.NewTimer(250 * time.Millisecond)
			<-timer.C
			scan(finalContext, true)
			cancel()
			return
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

func (u *screenshotUploader) upload(ctx context.Context, releaseID int32, path string) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range map[string]string{
		"kind":        strconv.Itoa(int(game_library.KindRelease)),
		"kind_id":     strconv.Itoa(int(releaseID)),
		"type":        strconv.Itoa(int(game_library.AssetTypeTitle)),
		"description": "RetroArch 截图",
		"debug":       "fbui:retroarch-screenshot",
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
