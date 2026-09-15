package main

import (
	_ "embed"
	"log"
	"net/http"
	_ "net/http/pprof"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbui/assets/fonts"
	"github.com/movsb/fbui/pkg/config"
	"github.com/movsb/fbui/pkg/swap_manager"
)

func main() {
	stopAutoUpdate := startExecutableAutoUpdate()
	defer stopAutoUpdate()

	app := fbiw.NewApp()
	defer app.Close()

	fonts.Init(app)

	app.SetThemeAccent(`deepskyblue`)

	NewOverlayWindow(app)
	window := NewMainWindow(app)
	http.Handle(`/api/store/assets/`, storeOpenAssetHandler(window.storeNav))
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
