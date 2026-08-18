package main

import (
	_ "embed"
	"net/http"
	_ "net/http/pprof"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbui/assets/fonts"
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

	NewMainWindow(app)

	app.Run()
}
