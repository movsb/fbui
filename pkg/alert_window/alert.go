package alert_window

import (
	"embed"

	"github.com/movsb/fbiw"
)

//go:embed *.html
var _embed embed.FS

func Alert(app *fbiw.App, message string, confirm, cancel func()) {
	doc := app.New(_embed, `alert.html`)

	window := struct {
		text *fbiw.Text `css:"text"`
	}{}

	doc.Bind(&window)

	window.text.SetText(message)

	app.Show(doc)

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
