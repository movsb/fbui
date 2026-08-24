package main

import (
	_ "embed"
	"net/http"
	_ "net/http/pprof"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbui/assets/fonts"
	"github.com/movsb/fbui/pkg/menu_popup"
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

	app.SetSwitcher(switchDesktops)

	app.Run()
}

func switchDesktops(app *fbiw.App) *fbiw.Document {
	menus := []menu_popup.MenuItem{}
	for desktop := range app.Desktops() {
		menus = append(menus, menu_popup.MenuItem{
			Name: desktop.Name(),
			Click: func() {
				app.SwitchTo(desktop)
			},
		})
	}
	return menu_popup.NewSwitcher(app, menus)
}
