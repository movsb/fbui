package main

import (
	"log"

	"github.com/movsb/fbiw"
)

type MainWindow struct {
	app *fbiw.App
	doc *fbiw.Document

	navigators []Navigator

	appNav   *AppsNavigator
	portsNav *AppsNavigator
	gamesNav *GamesNavigator
}

func (w *MainWindow) HandleKeyboardEvent(name fbiw.KeyName, pressed bool) {
	if len(w.navigators) <= 0 || !pressed {
		return
	}
	last := w.navigators[len(w.navigators)-1]

	next := last.Navigate(name)
	if next == nil {
		return
	} else if next == false {
		w.navigators = w.navigators[:len(w.navigators)-1]
		w.HandleKeyboardEvent(name, pressed)
	} else if nav, ok := next.(Navigator); ok {
		w.navigators = append(w.navigators, nav)
		w.HandleKeyboardEvent(name, pressed)
	} else {
		log.Panicf(`navigator 返回了无效值：%v`, next)
	}
}

type Navigator interface {
	// 返回值分几种情况：
	//  - 如果是nil，继续由自己导航。
	//  - 如果是Navigator，压栈此新的Navigator，并由它接管新的导航。
	//  - 如果是false，结束导航，回到前一个导航。
	Navigate(name fbiw.KeyName) any
}

func NewMainWindow(app *fbiw.App) *MainWindow {
	doc := app.New(`main.html`, `skin`)

	win := &MainWindow{
		app: app,
		doc: doc,
	}

	win.appNav = &AppsNavigator{
		window:   win,
		scroll:   doc.QuerySelector(`#apps`).(*fbiw.Scroll),
		dataName: `apps`,
	}
	win.portsNav = &AppsNavigator{
		window:   win,
		scroll:   doc.QuerySelector(`#ports`).(*fbiw.Scroll),
		dataName: `ports`,
	}
	win.gamesNav = &GamesNavigator{
		window:  win,
		emus:    doc.QuerySelector(`#emus`).(*fbiw.Scroll),
		roms:    doc.QuerySelector(`#roms`).(*fbiw.Scroll),
		noGames: doc.QuerySelector(`#nogames`),
	}

	doc.SetDelegator(win)

	win.initSystemTime()
	win.initSystemPower()

	go win.asyncInitApps()
	go win.asyncInitEmus()
	go win.asyncInitPorts()

	win.navigators = append(win.navigators, &_TitleNavigator{
		w:        win,
		catIndex: 0,
	})

	return win
}
