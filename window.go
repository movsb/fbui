package main

import (
	"github.com/movsb/fbiw"
)

type MainWindow struct {
	app *fbiw.App
	doc *fbiw.Document

	navigators []Navigator

	appNav   *AppsNavigator
	portsNav *AppsNavigator
	gamesNav *GamesNavigator
	toolsNav *ToolsNavigator

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

	win.initTools()

	doc.SetDelegator(win)

	win.initSystemTime()
	win.initSystemPower()

	go win.watchOsdEvents()
	go win.watchCPU()

	go win.asyncInitApps()
	go win.asyncInitEmus()
	go win.asyncInitPorts()

	win.navigators = append(win.navigators, &_TitleNavigator{
		w:        win,
		catIndex: 0,
	})

	return win
}
