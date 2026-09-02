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

// pprof 性能测试用。
//
// go tool pprof -web  http://localhost:8888/debug/pprof/profile?seconds=30
func init() {
	go http.ListenAndServe(`0.0.0.0:8888`, nil)
}

func main() {
	app := fbiw.NewApp()
	defer app.Close()

	fonts.Init(app)

	NewOverlayWindow(app)
	NewMainWindow(app)
	go func() {
		for _, err := range swap_manager.NewBackend(config.SDCARDRoot).Restore() {
			log.Printf("恢复 Swap 失败：%v", err)
		}
	}()

	app.Run()
}
