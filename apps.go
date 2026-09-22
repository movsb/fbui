package main

import (
	"log"
	"os/exec"
	"path/filepath"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbiw/input/sticks"
	"github.com/movsb/fbui/pkg/config"
)

// 加载所有的Apps列表。
// 包含机器系统目录的、存储卡内的。
func loadApps() []*config.LaunchConfig {
	launchConfigs := []*config.LaunchConfig{}

	launchConfigs = append(launchConfigs, config.LoadDir(filepath.Join(config.SDCARDRoot, `Apps`))...)
	launchConfigs = append(launchConfigs, config.LoadDir(`/usr/trimui/apps`)...)

	return launchConfigs
}

type _AppItem struct {
	root  fbiw.Box
	image *fbiw.Image `css:"img"`
	text  *fbiw.Text  `css:"text"`
}

// ScrollSelectionChanged implements [fbiw.ListSelectionAware].
func (view *_AppItem) ListSelectionChanged(selected bool) {
	view.text.SetMarqueeRunning(selected)
}

var _ fbiw.ListSelectionAware = (*_AppItem)(nil)

func (w *MainWindow) asyncInitApps() {
	apps := loadApps()
	w.doc.Async(func() {
		container := w.doc.GetBoxByID[*fbiw.List](`apps`)
		container.SetData(`apps`, apps)
		container.SetItems(len(apps),
			func() (fbiw.Box, *_AppItem) {
				item := w.doc.Instantiate[_AppItem](`app-item`)
				return item.root, item
			},
			func(item *_AppItem, index int) {
				app := apps[index]
				item.image.SetOSPath(app.IconPath())
				item.text.SetText(app.Name())
			},
		)
	})
}

func (w *MainWindow) asyncInitPorts() {
	apps := config.LoadDir(filepath.Join(config.SDCARDRoot, `Ports`))

	w.doc.Async(func() {
		scroll := w.doc.GetBoxByID[*fbiw.List](`ports`)
		scroll.SetData(`ports`, apps)
		scroll.SetItems(len(apps),
			func() (fbiw.Box, any) {
				item := w.doc.Instantiate[_AppItem](`app-item`)
				return item.root, item
			},
			func(item any, index int) {
				app := apps[index]
				appItem := item.(*_AppItem)
				appItem.image.SetOSPath(app.IconPath())
				appItem.text.SetText(app.Name())
			},
		)
	})
}

type LauncherNavigator struct {
	window  *MainWindow
	dataKey string
	scroll  *fbiw.List
}

func NewLauncherNavigator(win *MainWindow, selector string, dataKey string) *LauncherNavigator {
	n := LauncherNavigator{
		window:  win,
		dataKey: dataKey,
		scroll:  win.doc.QuerySelector[*fbiw.List](selector),
	}
	n.scroll.Listen(fbiw.InputDownEvent, n.handleKeyDown)
	return &n
}

func (n *LauncherNavigator) activate() {
	n.scroll.SetIndex(0, 0, 0)
	n.scroll.Activate()
}

func (n *LauncherNavigator) handleKeyDown(event *fbiw.Event) {
	if event.Input.Name == sticks.A && n.scroll.DataIndex() != -1 {
		n.openApp()
		event.StopPropagation()
		return
	}
	if event.Input.Name == sticks.B || (event.Input.Name == sticks.Up && n.scroll.DataRowIndex() <= 0) {
		n.scroll.Deselect()
		n.window.statusBarNav.activate()
		event.StopPropagation()
		return
	}
}

func (n *LauncherNavigator) openApp() {
	configs := n.scroll.GetData(n.dataKey).([]*config.LaunchConfig)
	config := configs[n.scroll.DataIndex()]
	n.window.app.Detach()
	go func() {
		defer n.window.app.AttachAsync()
		cmd := exec.Command(config.LauncherScriptPath())
		log.Println(`启动进程：`, cmd.String())
		cmd.Run()
	}()
}
