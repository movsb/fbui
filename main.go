package main

import (
	"context"
	"embed"
	_ "embed"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"

	"github.com/movsb/fbiw"
)

// pprof 性能测试用。
//
// go tool pprof -web  http://localhost:8888/debug/pprof/profile?seconds=30
func init() {
	go http.ListenAndServe(`0.0.0.0:8888`, nil)
}

//go:embed *.html nerd.ttf mono.ttf
var embedded embed.FS

func main() {
	app := fbiw.NewApp(context.Background(), embedded)
	defer app.Close()

	fontDir := os.DirFS(fbiw.Iif(runtime.GOOS == `linux`, `/usr/trimui/res`, `.`))
	app.AddFont(`system`, false, false, fontDir, `full.ttf`)
	app.AddFont(`nerd`, false, false, embedded, `nerd.ttf`)
	app.AddFont(`monospace`, false, false, embedded, `mono.ttf`)

	mainWindow := NewMainWindow(app)

	app.Show(mainWindow.doc)

	app.Run()
}
