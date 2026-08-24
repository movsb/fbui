package menu_popup

import (
	"embed"

	"github.com/movsb/fbiw"
)

//go:embed *.html
var _embed embed.FS

type MenuPopup struct {
	app *fbiw.App
	doc *fbiw.Document

	items []MenuItem

	scroll *fbiw.Scroll `css:"scroll"`
	header fbiw.Box     `css:"#header"`
	footer fbiw.Box     `css:"#footer"`
}

type MenuItem struct {
	Name  string
	Click func()
}

type _ItemView struct {
	root fbiw.Box
	name *fbiw.Text `css:"text"`
}

func NewMenuPopup(app *fbiw.App, items []MenuItem, header, footer fbiw.Box) *MenuPopup {
	win := &MenuPopup{
		app:   app,
		doc:   app.New(_embed, `menu_popup.html`),
		items: items,
	}

	win.doc.Bind(win)

	if header != nil {
		win.header.Base().AppendChild(header)
	}
	if footer != nil {
		win.footer.Base().AppendChild(footer)
	}

	win.scroll.SetItems(len(items),
		func() (fbiw.Box, *_ItemView) {
			item := fbiw.Unmarshal[_ItemView](win.doc, `
<block padding="0 10" align=middle>
	<text></text>
</block>
`)
			return item.root, item
		},
		func(item *_ItemView, index int) {
			item.name.SetText(items[index].Name)
		},
	)

	app.Show(win.doc)

	win.scroll.Activate()

	win.doc.Listen(fbiw.StickDownEvent, win.handleEvents)

	return win
}

func (win *MenuPopup) handleEvents(e *fbiw.Event) {
	if e.Stick.Name == fbiw.B {
		win.doc.Close()
		return
	}

	if e.Stick.Name == fbiw.A {
		if index := win.scroll.DataIndex(); index != -1 {
			item := win.items[index]
			item.Click()
			win.doc.Close()
			return
		}
	}
}
