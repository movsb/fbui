package main

import "github.com/movsb/fbiw"

type SearchWindow struct {
	app *fbiw.App
	doc *fbiw.Document
}

func (win *SearchWindow) HandleKeyboardEvent(name fbiw.KeyName, pressed bool) {
	if name == fbiw.B {
		win.doc.Close()
		return
	}
}
