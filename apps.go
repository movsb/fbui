package main

import (
	"log"
	"os/exec"
	"path/filepath"

	"github.com/movsb/fbiw"
)

// 加载所有的Apps列表。
// 包含机器系统目录的、存储卡内的。
func loadApps() []*LaunchConfig {
	launchConfigs := []*LaunchConfig{}

	launchConfigs = append(launchConfigs, loadDir(filepath.Join(_SDCARDRoot, `Apps`))...)
	launchConfigs = append(launchConfigs, loadDir(`/usr/trimui/apps`)...)

	return launchConfigs
}

func (w *MainWindow) asyncInitApps() {
	type _AppItem struct {
		Root  fbiw.Box
		Image *fbiw.Image `css:"img"`
		Text  *fbiw.Text  `css:"text"`
	}

	apps := loadApps()
	w.app.Async(func() {
		container := w.doc.GetBoxByID(`apps`).(*fbiw.Scroll)
		container.SetData(`apps`, apps)
		container.SetItems(len(apps),
			func() (fbiw.Box, any) {
				item := fbiw.Unmarshal[_AppItem](w.doc, `
<block align=center padding=30>
	<img spacer>
	<text></text>
</block>
`)
				return item.Root, item
			},
			func(item any, index int) {
				app := apps[index]
				appItem := item.(*_AppItem)
				appItem.Image.SetPath(filepath.Join(app.Dir, app.Config.IconTop))
				appItem.Text.SetText(fbiw.Iif(app.Config.LabelChinese != ``, app.Config.LabelChinese, app.Config.Label))
			},
		)
	})
}

type _AppsNavigator struct {
	w        *MainWindow
	scroll   *fbiw.Scroll
	dataName string
}

func (n *_AppsNavigator) Navigate(name fbiw.KeyName) any {
	if name == fbiw.B {
		log.Printf(`收到B按键`)
		n.scroll.Deselect()
		return false
	}

	if name == fbiw.A && n.scroll.DataIndex() != -1 {
		apps := n.scroll.GetData(n.dataName).([]*LaunchConfig)
		app := apps[n.scroll.DataIndex()]
		n.w.app.Detach()
		go func() {
			defer n.w.app.Async(func() {
				n.w.app.Attach()
			})
			cmd := exec.Command(app.LauncherScriptPath())
			log.Println(`启动进程：`, cmd.String())
			cmd.Run()
		}()
		return nil
	}

	if n.scroll.DataRowIndex() <= 0 && name == fbiw.Up {
		n.scroll.Deselect()
		return false
	}

	n.scroll.Navigate(name)

	return nil
}

func (w *MainWindow) asyncInitPorts() {
	type _AppItem struct {
		Root  fbiw.Box
		Image *fbiw.Image `css:"img"`
		Text  *fbiw.Text  `css:"text"`
	}

	apps := loadDir(filepath.Join(_SDCARDRoot, `Ports`))

	w.app.Async(func() {
		scroll := w.doc.GetBoxByID(`ports`).(*fbiw.Scroll)
		scroll.SetData(`ports`, apps)
		scroll.SetItems(len(apps),
			func() (fbiw.Box, any) {
				item := fbiw.Unmarshal[_AppItem](w.doc, `
<block align=center padding=30>
	<img spacer>
	<text></text>
</block>
`)
				return item.Root, item
			},
			func(item any, index int) {
				app := apps[index]
				appItem := item.(*_AppItem)
				appItem.Image.SetPath(app.IconPath())
				appItem.Text.SetText(app.Name())
			},
		)
	})
}
