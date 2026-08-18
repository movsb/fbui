package webdav_server

import (
	"context"
	"embed"
	"fmt"

	"github.com/movsb/fbiw"
)

//go:embed *.html
var _embed embed.FS

type WebDavWindow struct {
	app *fbiw.App

	doc    *fbiw.Document
	btn    *fbiw.Text `css:".button"`
	status *fbiw.Text `css:".status-string"`

	open   int // 0关闭，1打开中，2已打开
	ctx    context.Context
	cancel context.CancelFunc
}

func New(app *fbiw.App) *WebDavWindow {
	win := WebDavWindow{
		app: app,
		doc: app.New(_embed, `webdav.html`),
	}
	win.doc.Bind(&win)
	win.doc.Listen(fbiw.StickDownEvent, win.handleEvents, fbiw.EventOptions{})
	app.Show(win.doc)
	return &win
}

func (t *WebDavWindow) handleEvents(event *fbiw.Event) {
	name := event.Stick.Name

	if name == fbiw.B {
		switch t.open {
		case 0:
			t.doc.Close()
			t.doc = nil
			return
		case 1:
			break
		case 2:
			t.cancel()
			t.ctx = nil
			t.cancel = nil
			t.open = 0
			t.btn.SetText(string(rune(0xf204)))
			t.status.SetText(`已关闭`)
			return
		}
		return
	}

	if name == fbiw.A {
		switch t.open {
		case 0:
			t.open = 1
			t.ctx, t.cancel = context.WithCancel(context.Background())
			t.status.SetText(`启动中...`)
			_NewWebDAVServer(t.ctx, `/mnt/SDCARD/`, func(ip string, err error) {
				t.app.Async(func() {
					if err == nil {
						t.status.ClassRemove(`warning`)
						t.status.SetText(fmt.Sprintf(`已打开。服务器地址: %s。请在支持的软件中填入此地址，空用户名、空密码。`, ip))
						t.btn.SetText(string(rune(0xf205)))
						t.open = 2
					} else {
						t.status.ClassAdd(`warning`)
						t.status.SetText(fmt.Sprintf(`启动失败: %v`, err))
						t.open = 0
					}
				})
			})
		case 2:
			t.cancel()
			t.ctx = nil
			t.cancel = nil
			t.open = 0
			t.btn.SetText(string(rune(0xf204)))
			t.status.SetText(`已关闭`)
		}
	}
}
