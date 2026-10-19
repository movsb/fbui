package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fswatcher/fswatcher"
	"github.com/movsb/fbiw"
	"github.com/movsb/fbui/pkg/helpers"
)

var overlayWindow *OverlayWindow

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

	hintContainer *fbiw.Inline `css:"#button-hints"`
	buttonHints   []*overlayButtonHint

	currentDoc       *fbiw.Document
	buttonHintsByDoc map[*fbiw.Document][]ButtonHint
}

// ButtonHint describes one button hint displayed at the bottom of the overlay.
// Button is the button name shown in the badge, and Text describes its action.
type ButtonHint struct {
	Button string
	Text   string
}

type overlayButtonHint struct {
	root   fbiw.Box
	badge  *fbiw.Inline `css:".stick"`
	button *fbiw.Text   `css:".button"`
	text   *fbiw.Text   `css:".description"`
}

func NewOverlayWindow(app *fbiw.App) *OverlayWindow {
	doc := app.NewOverlay(_embed, `overlay.html`)

	win := &OverlayWindow{
		app:              app,
		doc:              doc,
		buttonHintsByDoc: map[*fbiw.Document][]ButtonHint{},
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

// SetButtonHints replaces the button hints displayed at the bottom of the
// overlay for the current document. Passing no hints hides the hints while keeping the CPU status visible.
func (win *OverlayWindow) SetButtonHints(hints ...ButtonHint) {
	if win.currentDoc != nil {
		win.buttonHintsByDoc[win.currentDoc] = slices.Clone(hints)
	}
	win.renderButtonHints(hints)
}

func (win *OverlayWindow) renderButtonHints(hints []ButtonHint) {
	for len(win.buttonHints) < len(hints) {
		win.appendButtonHint()
	}

	for i, view := range win.buttonHints {
		visible := i < len(hints)
		view.root.SetProp(`display`, fmt.Sprint(visible))
		if visible {
			view.button.SetText(hints[i].Button)
			view.badge.ClassToggle(`single`, utf8.RuneCountInString(hints[i].Button) == 1)
			view.text.SetText(hints[i].Text)
		}
	}
	win.hintContainer.SetProp(`display`, fmt.Sprint(len(hints) > 0))
}

func (win *OverlayWindow) appendButtonHint() {
	hint := win.doc.Instantiate[overlayButtonHint](`button-hint`)
	win.hintContainer.AppendChild(hint.root)
	win.buttonHints = append(win.buttonHints, hint)
}

func (win *OverlayWindow) watchDocChange() {
	win.app.Listen(fbiw.DocChange, func(e *fbiw.Event) {
		for doc := range win.buttonHintsByDoc {
			if doc.App() != win.app {
				delete(win.buttonHintsByDoc, doc)
			}
		}

		d := e.DocChange.Doc
		win.currentDoc = d
		if d != nil && d.Title() != `` {
			win.SetTitle(d.Title())
		}
		win.renderButtonHints(win.buttonHintsByDoc[d])
	})
}

func (win *OverlayWindow) initCpuUsage() {
	go helpers.WatchCPU(win.app.Context(), time.Second*3, func(s string) {
		win.doc.Async(func() {
			win.txtCpuStatus.SetTextFormat(`CPU: %s`, s)
		})
	})
}

func (win *OverlayWindow) initPowerStatus() {
	helpers.WatchPowerStatus(win.app.Context(), func(capacity int, charging bool) {
		win.doc.Async(func() {
			win.txtBatteryPercentage.SetTextFormat(`%d%%`, capacity)
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
				win.doc.Async(func() {
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
				win.doc.Async(func() {
					log.Println(`音量状态:`, open)
					win.txtVolumeOpen.SetText(fbiw.Iif(open, string(rune(0xefcf)), ``))
				})
				statusVolumeOpen = open
			}
		case `toggle_bt/status`:
			data, _ := os.ReadFile(path)
			open := strings.TrimSpace(string(data)) != `0`
			if open != statusBluetoothOpen || force {
				win.doc.Async(func() {
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
				win.doc.Async(func() {
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
