package main

import "github.com/movsb/fbiw"

type SearchWindow struct {
	app *fbiw.App
	doc *fbiw.Document
}

func NewSearchWindow(app *fbiw.App, doc *fbiw.Document) *SearchWindow {
	return &SearchWindow{
		app: app,
		doc: doc,
	}
}
