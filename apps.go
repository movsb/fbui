package main

import (
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
