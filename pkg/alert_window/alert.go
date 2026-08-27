package alert_window

import (
	"embed"

	"github.com/movsb/fbiw"
)

//go:embed *.html
var _embed embed.FS

func Error(opener *fbiw.Document, message string, confirm, cancel func()) {
	alert(opener, message, confirm, cancel, func(doc *fbiw.Document) {
		doc.Root().SetProp(`border-color`, `red`)
	})
}

func Alert(opener *fbiw.Document, message string, confirm, cancel func()) {
	alert(opener, message, confirm, cancel, nil)
}

func alert(opener *fbiw.Document, message string, confirm, cancel func(), beforeShow func(doc *fbiw.Document)) {
	doc := opener.App().NewPopup(_embed, `alert.html`, opener)

	window := struct {
		text *fbiw.Text `css:"text"`
	}{}

	doc.Bind(&window)

	window.text.SetText(message)

	if beforeShow != nil {
		beforeShow(doc)
	}

	doc.Listen(fbiw.StickDownEvent, func(e *fbiw.Event) {
		switch e.Stick.Name {
		case fbiw.A:
			doc.Close()
			if confirm != nil {
				confirm()
			}
		case fbiw.B:
			doc.Close()
			if cancel != nil {
				cancel()
			}
		}
	})
}
