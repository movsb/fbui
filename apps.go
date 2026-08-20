package main

import (
	"log"
	"os/exec"
	"path/filepath"

	"github.com/movsb/fbiw"
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

func (w *MainWindow) asyncInitApps() {
	type _AppItem struct {
		root  fbiw.Box
		image *fbiw.Image `css:"img"`
		text  *fbiw.Text  `css:"text"`
	}

	apps := loadApps()
	w.app.Async(func() {
		container := w.doc.GetBoxByID[*fbiw.Scroll](`apps`)
		container.SetData(`apps`, apps)
		container.SetItems(len(apps),
			func() (fbiw.Box, *_AppItem) {
				item := fbiw.Unmarshal[_AppItem](w.doc, `
<block align=center padding=30>
	<img spacer>
	<text></text>
</block>
`)
				return item.root, item
			},
			func(item *_AppItem, index int) {
				app := apps[index]
				item.image.SetPath(filepath.Join(app.Dir, app.Config.IconTop))
				item.text.SetText(fbiw.Iif(app.Config.LabelChinese != ``, app.Config.LabelChinese, app.Config.Label))
			},
		)
	})
}

func (w *MainWindow) asyncInitPorts() {
	type _AppItem struct {
		root  fbiw.Box
		image *fbiw.Image `css:"img"`
		text  *fbiw.Text  `css:"text"`
	}

	apps := config.LoadDir(filepath.Join(config.SDCARDRoot, `Ports`))

	w.app.Async(func() {
		scroll := w.doc.GetBoxByID[*fbiw.Scroll](`ports`)
		scroll.SetData(`ports`, apps)
		scroll.SetItems(len(apps),
			func() (fbiw.Box, any) {
				item := fbiw.Unmarshal[_AppItem](w.doc, `
<block align=center padding=30>
	<img spacer>
	<text></text>
</block>
`)
				return item.root, item
			},
			func(item any, index int) {
				app := apps[index]
				appItem := item.(*_AppItem)
				appItem.image.SetPath(app.IconPath())
				appItem.text.SetText(app.Name())
			},
		)
	})
}

type LauncherNavigator struct {
	window  *MainWindow
	dataKey string
	scroll  *fbiw.Scroll
}

func NewLauncherNavigator(win *MainWindow, selector string, dataKey string) *LauncherNavigator {
	n := LauncherNavigator{
		window:  win,
		dataKey: dataKey,
		scroll:  win.doc.QuerySelector[*fbiw.Scroll](selector),
	}
	n.scroll.Listen(fbiw.StickDownEvent, n.handleKeyDown, fbiw.EventOptions{})
	return &n
}

func (n *LauncherNavigator) activate() {
	n.scroll.SetIndex(0, 0, 0)
	n.scroll.Activate()
}

func (n *LauncherNavigator) handleKeyDown(event *fbiw.Event) {
	if event.Stick.Name == fbiw.A && n.scroll.DataIndex() != -1 {
		n.openApp()
		event.StopPropagation()
		return
	}
	if event.Stick.Name == fbiw.B || (event.Stick.Name == fbiw.Up && n.scroll.DataRowIndex() <= 0) {
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
