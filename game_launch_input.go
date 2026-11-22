package main

import (
	"github.com/movsb/fbiw"
	"github.com/movsb/fbiw/input/sticks"
	"github.com/movsb/fbui/pkg/launcher"
	"github.com/movsb/fbui/pkg/mainui"
	"github.com/movsb/fbui/pkg/menu_popup"
)

// Each press captures its game, so repeats cannot launch or restart the timer.
type gameLaunchPress struct {
	cancel func()
	launch func()
}

func (p *gameLaunchPress) down(w *MainWindow, item mainui.Item) {
	if p.cancel != nil {
		return
	}
	p.launch = func() { w.launchItem(item) }
	p.cancel = w.doc.SetTimeout(600, func() {
		p.reset()
		w.chooseGameCore(item, true)
	})
}

func (p *gameLaunchPress) actions(doc *fbiw.Document, short, long func()) {
	if p.cancel != nil {
		return
	}
	p.launch = short
	p.cancel = doc.SetTimeout(600, func() { p.reset(); long() })
}

func (p *gameLaunchPress) up() {
	launch := p.launch
	p.reset()
	if launch != nil {
		launch()
	}
}

func (p *gameLaunchPress) reset() {
	if p.cancel != nil {
		p.cancel()
	}
	p.cancel = nil
	p.launch = nil
}

func (w *MainWindow) chooseGameCore(item mainui.Item, held ...bool) {
	cores := launcher.CoreNames(item.ROMPath, item.Launch)
	if len(cores) == 0 {
		w.app.ShowAlertDialog(w.doc, fbiw.AlertDialogOptions{Title: "没有可选模拟器", Description: "该游戏尚未配置可选的模拟器核心。"})
		return
	}
	items := make([]menu_popup.MenuItem, 0, len(cores))
	for _, core := range cores {
		items = append(items, menu_popup.MenuItem{Name: core, Click: func() { w.launchItemWithCore(item, core) }})
	}
	popup := menu_popup.NewMenuPopup(w.app, w.doc, items, nil, nil)
	// Consume the opening press (including repeats) until its release.
	if len(held) > 0 && held[0] {
		popup.WaitForRelease(sticks.A)
	}
}
