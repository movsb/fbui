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

	txtTime              *fbiw.Text
	txtBatteryPercentage *fbiw.Text
	boxBatteryCharging   fbiw.Box

	txtVolumeOpen    *fbiw.Text
	txtWifiOpen      *fbiw.Text
	txtBluetoothOpen *fbiw.Text
	txtCpuStatus     *fbiw.Text
}

func NewMainWindow(app *fbiw.App) *MainWindow {
	doc := app.New(`main.html`, `skin`)

	win := &MainWindow{
		app: app,
		doc: doc,

		txtTime:              doc.QuerySelector(`#time`).(*fbiw.Text),
		txtBatteryPercentage: doc.QuerySelector(`#battery`).(*fbiw.Text),
		boxBatteryCharging:   doc.QuerySelector(`#battery-charging-box`),

		txtVolumeOpen:    doc.QuerySelector(`#speaker`).(*fbiw.Text),
		txtWifiOpen:      doc.QuerySelector(`#wifi`).(*fbiw.Text),
		txtBluetoothOpen: doc.QuerySelector(`#bluetooth`).(*fbiw.Text),
		txtCpuStatus:     doc.QuerySelector(`#cpu`).(*fbiw.Text),
	}

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
