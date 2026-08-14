package main

import (
	"github.com/movsb/fbiw"
)

type MainWindow struct {
	app *fbiw.App
	doc *fbiw.Document

	statusBarNav *StatusBarNavigator
	gamesNav     *GamesNavigator
	portsNav     *LauncherNavigator
	appsNav      *LauncherNavigator
	toolsNav     *ToolsNavigator

	txtTime              *fbiw.Text `css:"#time"`
	txtBatteryPercentage *fbiw.Text `css:"#battery"`
	boxBatteryCharging   fbiw.Box   `css:"#battery-charging-box"`

	txtVolumeOpen    *fbiw.Text `css:"#speaker"`
	txtWifiOpen      *fbiw.Text `css:"#wifi"`
	txtBluetoothOpen *fbiw.Text `css:"#bluetooth"`
	txtCpuStatus     *fbiw.Text `css:"#cpu"`
}

func NewMainWindow(app *fbiw.App) *MainWindow {
	doc := app.New(`main.html`, `skin`)

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

	win.initSystemTime()
	win.initSystemPower()

	go win.watchOsdEvents()
	go win.watchCPU()

	go win.asyncInitApps()
	go win.asyncInitEmus()
	go win.asyncInitPorts()

	win.statusBarNav.activate()

	return win
}
