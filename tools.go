package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"runtime"
	"time"

	"github.com/movsb/fbiw"
	"golang.org/x/net/webdav"
)

type WebDavWindow struct {
	app *fbiw.App

	doc    *fbiw.Document
	btn    *fbiw.Text
	status *fbiw.Text

	open   int // 0关闭，1打开中，2已打开
	ctx    context.Context
	cancel context.CancelFunc
}

func NewWebDavWindow(app *fbiw.App, doc *fbiw.Document) *WebDavWindow {
	win := WebDavWindow{
		app:    app,
		doc:    doc,
		btn:    doc.QuerySelector(`.button`).(*fbiw.Text),
		status: doc.QuerySelector(`.status-string`).(*fbiw.Text),
	}
	win.doc.Listen(fbiw.KeyboardEvent, win.handleEvents, fbiw.EventOptions{})
	return &win
}

func (t *WebDavWindow) handleEvents(event *fbiw.Event) {
	if !(event.Type == fbiw.KeyboardEvent && event.Keyboard.KeyDown) {
		return
	}

	name := event.Keyboard.Name

	if name == fbiw.B {
		if t.open != 0 {
			t.cancel()
			t.ctx = nil
			t.cancel = nil
			t.open = 0
			t.btn.SetText(string(rune(0xf204)))
			t.status.SetText(`已关闭`)
			return
		} else {
			t.doc.Close()
			t.doc = nil
			return
		}
	}

	if name == fbiw.A {
		switch t.open {
		case 0:
			t.open = 1
			t.ctx, t.cancel = context.WithCancel(context.Background())
			t.status.SetText(`启动中...`)
			NewWebDAVServer(t.ctx, `/mnt/SDCARD/`, func(ip string, err error) {
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

type _ToolItemData struct {
	name  string
	click func()
}

type _ToolItemView struct {
	root fbiw.Box
	name *fbiw.Text `css:"text"`
}

func NewToolsNavigator(win *MainWindow) *ToolsNavigator {
	toolsNav := &ToolsNavigator{
		window: win,
		scroll: win.doc.QuerySelector(`#tools`).(*fbiw.Scroll),
		tools: []_ToolItemData{
			{
				name: `文件服务器（WebDAV）`,
				click: func() {
					doc := win.app.New(`webdav.html`, ``)
					NewWebDavWindow(win.app, doc)
					win.app.Show(doc)
				},
			},
		},
	}
	toolsNav.scroll.SetItems(
		len(toolsNav.tools),
		func() (fbiw.Box, any) {
			box := fbiw.Unmarshal[_ToolItemView](win.doc, `
<block padding=30>
	<inline spacer align=middle>
		<text></text>
	</inline>
</block>`)
			return box.root, box
		},
		func(user any, index int) {
			item := user.(*_ToolItemView)
			item.name.SetText(win.toolsNav.tools[index].name)
		},
	)
	toolsNav.scroll.Listen(fbiw.KeyboardEvent, toolsNav.handleEvents, fbiw.EventOptions{})
	return toolsNav
}

type ToolsNavigator struct {
	window *MainWindow
	scroll *fbiw.Scroll
	tools  []_ToolItemData
}

func (n *ToolsNavigator) activate() {
	n.scroll.SetIndex(0, 0, 0)
	n.scroll.Activate()
}

func (n *ToolsNavigator) handleEvents(event *fbiw.Event) {
	if !(event.Type == fbiw.KeyboardEvent && event.Keyboard.KeyDown) {
		return
	}
	name := event.Keyboard.Name
	if name == fbiw.B || (name == fbiw.Up && n.scroll.DataRowIndex() <= 0) {
		n.scroll.Deselect()
		n.window.statusBarNav.activate()
		return
	}

	if name == fbiw.A && n.scroll.DataIndex() != -1 {
		tool := n.tools[n.scroll.DataIndex()]
		tool.click()
		event.StopPropagation()
		return
	}
}

// callback会在服务器准备好后于线程中调用。
func NewWebDAVServer(ctx context.Context, dir string, callback func(string, error)) {
	lanIP, err := getIP()
	if err != nil {
		callback(``, err)
		return
	}

	ctx, cancel := context.WithCancel(ctx)

	server := webdav.Handler{
		FileSystem: webdav.Dir(dir),
		LockSystem: webdav.NewMemLS(),
	}

	lis, err := net.Listen(`tcp4`, `0.0.0.0:6666`)
	if err != nil {
		callback(``, err)
		log.Println(err)
		cancel()
		return
	}

	go func() {
		<-ctx.Done()
		lis.Close()
	}()

	go func() {
		defer log.Println(`文件服务器退出`)
		http.Serve(lis, &server)
	}()

	go func() {
		now := time.Now()
		for time.Since(now) < time.Second*3 {
			conn, err := net.Dial(`tcp4`, `localhost:6666`)
			if err != nil {
				time.Sleep(time.Millisecond * 500)
				continue
			}
			time.Sleep(time.Millisecond * 500)
			conn.Close()
			callback(fmt.Sprintf(`http://%s:6666`, lanIP), nil)
			return
		}
		callback(``, errors.New(`启动超时`))
		cancel()
	}()
}

var errNoIP = errors.New("未获取到IP地址，Wi-Fi启动了吗？")

func getIP() (string, error) {
	face, err := net.InterfaceByName(fbiw.Iif(runtime.GOOS == `linux`, `wlan0`, `en0`))
	if err != nil {
		return ``, err
	}

	if face.Flags&net.FlagUp == 0 {
		return ``, errNoIP
	}
	if face.Flags&net.FlagRunning == 0 {
		return ``, errNoIP
	}

	addrs, err := face.Addrs()
	if err != nil {
		return ``, err
	}

	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipnet.IP.To4()
		if ip != nil {
			return ip.String(), nil
		}
	}

	return "", errNoIP
}
