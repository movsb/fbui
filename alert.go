package main

import "github.com/movsb/fbiw"

func Alert(app *fbiw.App, message string, confirm, cancel func()) {
	doc := app.New(`alert.html`, `.`)

	text := doc.QuerySelector(`text`).(*fbiw.Text)
	text.SetText(message)

	app.Show(doc)

	doc.Listen(fbiw.StickDownEvent, func(e *fbiw.Event) {
		switch e.Stick.Name {
		case fbiw.A:
			confirm()
		case fbiw.B:
			doc.Close()
			cancel()
		}
	}, fbiw.EventOptions{})
}
