package main

import (
	_ "embed"
	"log"
	"net/http"
	_ "net/http/pprof"
	"sync/atomic"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbui/assets/fonts"
	"github.com/movsb/fbui/pkg/config"
	"github.com/movsb/fbui/pkg/elastic_cpu"
	"github.com/movsb/fbui/pkg/helpers"
	"github.com/movsb/fbui/pkg/swap_manager"
)

func init() {
	fbiw.RegisterThemeColor("--text-shadow", "#FFFC", "#000")

	fbiw.DefineStyles(`
        text {
            text-outline-width: 1;
            text-outline-color: var(--text-shadow);
        }
    `)
}

func main() {
	if err := helpers.DisableJoypad(); err != nil {
		log.Printf("禁用摇杆失败：%v", err)
	}

	app := fbiw.NewApp()
	defer app.Close()

	var externalRunning atomic.Bool
	stopCPU := elastic_cpu.Start(func() bool {
		app.Async(func() { externalRunning.Store(app.Detached()) })
		return externalRunning.Load()
	})
	defer stopCPU()

	stopAutoUpdate := startExecutableAutoUpdate(func() bool { return !app.Detached() })
	defer stopAutoUpdate()

	fonts.Init(app)

	app.SetThemeAccent(`deepskyblue`)

	overlayWindow = NewOverlayWindow(app)
	window := NewMainWindow(app)
	http.Handle(`/api/store/assets/`, storeOpenAssetHandler(window.storeNav))
	http.Handle(`/api/store/releases/`, storeCaptureTitleHandler(window.storeNav))
	http.Handle(`/api/store/games/`, storeCaptureTitleHandler(window.storeNav))
	// pprof 性能测试：go tool pprof -web http://localhost:8888/debug/pprof/profile?seconds=30
	go func() {
		if err := http.ListenAndServe(`0.0.0.0:8888`, nil); err != nil {
			log.Printf("HTTP 服务失败：%v", err)
		}
	}()

	go func() {
		for _, err := range swap_manager.NewBackend(config.SDCARDRoot).Restore() {
			log.Printf("恢复 Swap 失败：%v", err)
		}
	}()

	app.Run()
}
