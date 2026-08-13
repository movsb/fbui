package main

import (
	"unicode/utf8"

	"github.com/movsb/fbiw"
)

type SearchWindow struct {
	app *fbiw.App
	doc *fbiw.Document

	searchBox fbiw.Box
	resultBox *fbiw.Scroll
	txtQuery  *fbiw.Text
	keyboard  fbiw.Box

	results []_SearchResultItem

	prevKey        fbiw.Box
	keyRow, keyCol int
}

func NewSearchWindow(app *fbiw.App, doc *fbiw.Document) *SearchWindow {
	win := &SearchWindow{
		app: app,
		doc: doc,

		keyRow: -1,
		keyCol: -1,
	}
	win.searchBox = doc.QuerySelector(`#search`)
	win.resultBox = doc.QuerySelector(`#result`).(*fbiw.Scroll)
	win.txtQuery = doc.QuerySelector(`#query`).(*fbiw.Text)
	win.keyboard = doc.QuerySelector(`#keyboard`)
	win.doc.Listen(fbiw.StickDownEvent, win.handleEvents, fbiw.EventOptions{})
	return win
}

func (w *SearchWindow) handleEvents(event *fbiw.Event) {
	// 按“Y”直接关闭窗口。
	if event.Stick.Name == fbiw.Y {
		w.doc.Close()
		return
	}

	if display := w.searchBox.GetComputedStyles().Display; display.Empty() || display.Bool {
		w.handleSearchEvents(event)
	} else {
		w.handleResultEvents(event)
	}
}

func (w *SearchWindow) handleSearchEvents(event *fbiw.Event) {
	if event.Stick.Name == fbiw.B {
		t := w.txtQuery.GetText()
		// 关闭窗口
		if t == `` {
			w.doc.Close()
			return
		}
		// 删除内容
		_, size := utf8.DecodeLastRuneInString(t)
		t = t[:len(t)-size]
		w.txtQuery.SetText(t)
		return
	}

	switch event.Stick.Name {
	case fbiw.Left, fbiw.Right, fbiw.Up, fbiw.Down:
		w.switchKey(event)
	case fbiw.A:
		if w.prevKey != nil {
			old := w.txtQuery.GetText()
			new := w.prevKey.Children()[0].(*fbiw.Text).GetText()
			w.txtQuery.SetText(old + new)
		}
	}
}

func (w *SearchWindow) switchKey(event *fbiw.Event) {
	set := func(r, c int) {
		if 0 <= r && r <= len(w.keyboard.Children())-1 {
			row := w.keyboard.Children()[r]
			cols := len(row.Children())
			if c == -1 {
				c = cols - 1
			}
			if c >= cols {
				c = 0
			}
			if 0 <= c && c <= cols-1 {
				col := row.Children()[c]
				if w.prevKey != nil {
					w.prevKey.ClassRemove(`selected`)
				}
				col.ClassAdd(`selected`)
				w.prevKey = col
				w.keyRow = r
				w.keyCol = c
			}
		}
	}

	if w.keyRow == -1 {
		switch event.Stick.Name {
		case fbiw.Up:
			set(2, 0)
		case fbiw.Right:
			set(0, 0)
		case fbiw.Down:
			set(0, 0)
		case fbiw.Left:
			set(0, -1)
		}
	} else {
		switch event.Stick.Name {
		case fbiw.Up:
			c := w.keyCol
			if w.keyRow == 2 {
				c++
			}
			set(w.keyRow-1, c)
		case fbiw.Right:
			set(w.keyRow, w.keyCol+1)
		case fbiw.Down:
			c := w.keyCol
			if w.keyRow == 0 && c == len(w.keyboard.Children()[0].Children())-1 {
				c--
			} else if w.keyRow == 1 {
				if c > 0 {
					if c == len(w.keyboard.Children()[1].Children())-1 {
						c -= 2
					} else {
						c--
					}
				}
			}
			set(w.keyRow+1, c)
		case fbiw.Left:
			set(w.keyRow, w.keyCol-1)
		}
	}
}

type _SearchResultItem struct {
	launcher    *LaunchConfig
	displayName string
	score       int
	romPath     string
}

func (w *SearchWindow) handleResultEvents(event *fbiw.Event) {
	if event.Stick.Name == fbiw.B {
		w.resultBox.SetProp(`display`, `false`)
		w.searchBox.SetProp(`display`, `true`)
		return
	}

	if event.Stick.Name == fbiw.A {
		index := w.resultBox.DataIndex()
		if index < 0 {
			return
		}
	}
}
