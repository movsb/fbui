package ssh

import (
	"context"
	"embed"
	"fmt"
	"net"

	"github.com/movsb/fbiw"
)

//go:embed *.html
var _embed embed.FS

type SSHWindow struct {
	app *fbiw.App

	doc    *fbiw.Document
	btn    *fbiw.Text `css:".button"`
	status *fbiw.Text `css:".status-string"`

	open   int // 0关闭，1打开中，2已打开
	ctx    context.Context
	cancel context.CancelFunc
}

func New(app *fbiw.App) *SSHWindow {
	win := SSHWindow{
		app: app,
		doc: app.NewDesktop(_embed, `ssh.html`),
	}
	win.doc.Bind(&win)
	win.doc.Listen(fbiw.StickDownEvent, win.handleEvents)
	return &win
}

func (t *SSHWindow) handleEvents(event *fbiw.Event) {
	name := event.Stick.Name

	if name == fbiw.B {
		switch t.open {
		case 0:
			t.doc.Close()
			t.doc = nil
			return
		case 2:
			t.close(`已关闭。`)
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
			Serve(t.ctx, func(addr string, err error) {
				t.app.Async(func() {
					if err == nil {
						t.status.ClassRemove(`warning`)
						host, port, _ := net.SplitHostPort(addr)
						t.status.SetTextFormat("已打开。\n\n地址: %s\n端口: %s\n用户: root\n密码: (无)\n\n你可以按“Select”键切换到其它桌面以保持服务器在后台运行。", host, port)
						t.btn.SetText(string(rune(0xf205)))
						t.open = 2
					} else {
						t.status.ClassAdd(`warning`)
						t.close(fmt.Sprintf(`错误: %v`, err))
					}
				})
			})
		case 2:
			t.close(`已关闭。`)
		}
	}
}

func (t *SSHWindow) close(reason string) {
	if t.cancel != nil {
		t.cancel()
		t.ctx = nil
		t.cancel = nil
	}
	t.open = 0
	t.btn.SetText(string(rune(0xf204)))
	t.status.SetText(reason)
}
