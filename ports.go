package main

import (
	"path/filepath"

	"github.com/movsb/fbiw"
)

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
