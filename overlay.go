package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/fswatcher/fswatcher"
	"github.com/movsb/fbiw"
	"github.com/movsb/fbui/pkg/helpers"
)

type OverlayWindow struct {
	app *fbiw.App
	doc *fbiw.Document

	theTitle *fbiw.Text `css:"#title"`

	txtTime              *fbiw.Text `css:"#time"`
	txtBatteryPercentage *fbiw.Text `css:"#battery"`
	boxBatteryCharging   fbiw.Box   `css:"#battery-charging-box"`

	txtVolumeOpen    *fbiw.Text `css:"#speaker"`
	txtWifiOpen      *fbiw.Text `css:"#wifi"`
	txtBluetoothOpen *fbiw.Text `css:"#bluetooth"`
	txtCpuStatus     *fbiw.Text `css:"#cpu"`
}

func NewOverlayWindow(app *fbiw.App) *OverlayWindow {
	doc := app.NewOverlay(_embed, `overlay.html`)

	win := &OverlayWindow{
		app: app,
		doc: doc,
	}

	doc.Bind(win)
	app.SetOverlay(doc)

	win.watchDocChange()

	win.initCpuUsage()
	win.initSystemTime()
	win.initPowerStatus()

	go win.watchOsdEvents()

	return win
}

func (win *OverlayWindow) SetTitle(s string) {
	win.theTitle.SetText(s)
}

func (win *OverlayWindow) watchDocChange() {
	win.app.Listen(fbiw.DocChange, func(e *fbiw.Event) {
		d := e.DocChange.Doc
		if d.Title() != `` {
			win.theTitle.SetText(d.Title())
		}
	}, fbiw.EventOptions{})
}

func (win *OverlayWindow) initCpuUsage() {
	go helpers.WatchCPU(win.app.Context(), time.Second*3, func(s string) {
		win.doc.Async(func() {
			win.txtCpuStatus.SetText(s)
		})
	})
}

func (win *OverlayWindow) initPowerStatus() {
	helpers.WatchPowerStatus(win.app.Context(), func(capacity int, charging bool) {
		win.doc.Async(func() {
			win.txtBatteryPercentage.SetText(fmt.Sprintf(`%d%%`, capacity))
			// 只有充电的时候显示。放电或已满均不显示。
			win.boxBatteryCharging.SetProp(`display`, fmt.Sprint(charging))
		})
	})
}

func (win *OverlayWindow) initSystemTime() {
	win.txtTime.SetText(time.Now().Format(`15:04`))
	go func() {
		last := ``
		for range time.Tick(time.Minute) {
			select {
			case <-win.app.Context().Done():
				return
			default:
				win.app.Async(func() {
					now := time.Now().Format(`15:04`)
					if now == last {
						return
					}
					last = now
					win.txtTime.SetText(now)
				})
			}
		}
	}()
}

func (win *OverlayWindow) watchOsdEvents() {
	watcher, err := fswatcher.NewWatcher()
	if err != nil {
		log.Println(`创建观察器失败:`, err)
		return
	}
	defer watcher.Close()

	if err := watcher.AddRecursive(`/tmp/trimui_osd`, fswatcher.All); err != nil {
		log.Println(`观察OSD目录失败:`, err)
		return
	}

	ctx := context.Background()

	var (
		statusVolumeOpen    = false
		statusBluetoothOpen = false
		statusWifiOpen      = false
	)

	handleOsdEvent := func(path string, force bool) {
		suffix, found := strings.CutPrefix(path, `/tmp/trimui_osd/`)
		if !found {
			return
		}
		switch suffix {
		case `slider_volume/status`:
			data, _ := os.ReadFile(path)
			open := !strings.HasPrefix(strings.TrimSpace(string(data)), `0/`)
			if open != statusVolumeOpen || force {
				win.app.Async(func() {
					log.Println(`音量状态:`, open)
					win.txtVolumeOpen.SetText(fbiw.Iif(open, string(rune(0xefcf)), ``))
				})
				statusVolumeOpen = open
			}
		case `toggle_bt/status`:
			data, _ := os.ReadFile(path)
			open := strings.TrimSpace(string(data)) != `0`
			if open != statusBluetoothOpen || force {
				win.app.Async(func() {
					log.Println(`蓝牙状态:`, open)
					win.txtBluetoothOpen.SetText(fbiw.Iif(open, string(rune(0xf00af)), ``))
				})
				statusBluetoothOpen = open
			}
		case `toggle_wifi/status`:
			// TODO wifi打开不等于已经分配到ip，这里需要区别状态
			data, _ := os.ReadFile(path)
			open := strings.TrimSpace(string(data)) != `0`
			if open != statusWifiOpen || force {
				win.app.Async(func() {
					log.Println(`网络状态:`, open)
					win.txtWifiOpen.SetText(fbiw.Iif(open, string(rune(0xf1eb)), ``))
				})
				statusWifiOpen = open
			}
		}
	}

	handleOsdShow := func(show bool) {
		if show {
			win.app.DetachAsync()
		} else {
			win.app.AttachAsync()
		}
	}

	handleOsdEvent(`/tmp/trimui_osd/slider_volume/status`, true)
	handleOsdEvent(`/tmp/trimui_osd/toggle_bt/status`, true)
	handleOsdEvent(`/tmp/trimui_osd/toggle_wifi/status`, true)

	for {
		defer watcher.Close()
		select {
		case <-ctx.Done():
			return
		case err := <-watcher.Errors:
			log.Println(`非致命错误:`, err)
		case event := <-watcher.Events:
			// log.Println(`收到事件:`, event)
			if strings.Contains(event.Name, `osdd_show_up`) {
				handleOsdShow(event.Op.Has(fswatcher.Create))
				break
			}
			if event.Op.Has(fswatcher.Write) {
				handleOsdEvent(event.Name, false)
			}
		}
	}
}
