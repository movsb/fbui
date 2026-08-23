package main

import (
	"embed"
	"fmt"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbui/pkg/alert_window"
)

//go:embed *.html
var _embed embed.FS

type MainWindow struct {
	app *fbiw.App
	doc *fbiw.Document

	statusBarNav *StatusBarNavigator
	gamesNav     *GamesNavigator
	portsNav     *LauncherNavigator
	appsNav      *LauncherNavigator
	toolsNav     *ToolsNavigator
}

func NewMainWindow(app *fbiw.App) *MainWindow {
	doc := app.New(_embed, `window.html`)

	win := &MainWindow{
		app: app,
		doc: doc,
	}

	doc.Bind(win)

	win.statusBarNav = NewStatusBarNavigator(win)
	win.gamesNav = NewGamesNavigator(win)
	win.portsNav = NewLauncherNavigator(win, `#ports`, `ports`)
	win.appsNav = NewLauncherNavigator(win, `#apps`, `apps`)
	win.toolsNav = NewToolsNavigator(win)

	go win.asyncInitApps()
	go win.asyncInitEmus()
	go win.asyncInitPorts()

	win.statusBarNav.activate()

	app.Listen(fbiw.DocChange, func(e *fbiw.Event) {
		d := e.DocChange.Doc
		win.statusBarNav.showCatBar(d == doc)
	})

	app.Show(doc)

	return win
}

type StatusBarNavigator struct {
	window       *MainWindow
	catBar       fbiw.Box `css:"#cat-bar"`
	catIndex     int
	catBoxes     []fbiw.Box `css:"#cat-bar text"`
	contentBoxes []fbiw.Box `css:"#content > *"`
}

func NewStatusBarNavigator(win *MainWindow) *StatusBarNavigator {
	n := StatusBarNavigator{
		window:   win,
		catIndex: 0,
	}
	win.doc.Bind(&n)
	n.catBar.Listen(fbiw.StickDownEvent, n.handleEvents)
	return &n
}

func (n *StatusBarNavigator) showCatBar(show bool) {
	n.catBar.SetProp(`display`, fmt.Sprint(show))
}

func (n *StatusBarNavigator) activate() {
	n.catBar.Activate()
	n.catBar.ClassAdd(`active`)
}

func (n *StatusBarNavigator) activateContent() {
	if n.catIndex >= 0 && n.catIndex <= len(n.contentBoxes)-1 {
		content := n.contentBoxes[n.catIndex]
		removeActive := true
		switch content.Base().ID {
		case `games`:
			n.window.gamesNav.activate()
		case `ports`:
			n.window.portsNav.activate()
		case `apps`:
			n.window.appsNav.activate()
		case `tools`:
			n.window.toolsNav.activate()
		default:
			removeActive = false
		}
		if removeActive {
			n.catBar.ClassRemove(`active`)
		}
	}
}

func (n *StatusBarNavigator) handleEvents(event *fbiw.Event) {
	keyName := event.Stick.Name

	if keyName == fbiw.B {
		alert_window.Alert(n.window.app, `确定要退出吗？`, func() {
			n.window.app.Quit()
		}, func() {})
		return
	}

	if n.catIndex <= 0 && keyName == fbiw.Left {
		return
	}
	if n.catIndex >= len(n.catBoxes)-1 && keyName == fbiw.Right {
		return
	}

	if keyName == fbiw.Left || keyName == fbiw.Right {
		// 原来的去掉选中
		if n.catIndex >= 0 && n.catIndex < len(n.catBoxes) {
			t := n.catBoxes[n.catIndex].(*fbiw.Text)
			t.ClassRemove(`selected`)
			b := n.contentBoxes[n.catIndex]
			b.Base().ClassRemove(`selected`)
		}

		switch keyName {
		case fbiw.Left:
			n.catIndex--
		case fbiw.Right:
			n.catIndex++
		}

		t := n.catBoxes[n.catIndex].(*fbiw.Text)
		t.ClassAdd(`selected`)
		b := n.contentBoxes[n.catIndex]
		b.Base().ClassAdd(`selected`)

		event.StopPropagation()
		return
	}

	if keyName == fbiw.Down {
		n.activateContent()
		event.StopPropagation()
	}
}
