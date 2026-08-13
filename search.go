package main

import (
	"context"
	"io/fs"
	"log"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbui/assets/game_names"
	"github.com/movsb/fbui/assets/searchable"
)

type SearchWindow struct {
	app *fbiw.App
	doc *fbiw.Document

	searchBox  fbiw.Box
	resultBox  fbiw.Box
	resultList *fbiw.Scroll

	resultStatus        *fbiw.Text
	resultStatusWrapper fbiw.Box

	txtQuery *fbiw.Text
	keyboard fbiw.Box

	prevKey        fbiw.Box
	keyRow, keyCol int

	// ctx                context.Context
	// cancel             context.CancelFunc
	allSearchableItems atomic.Value
}

func NewSearchWindow(app *fbiw.App, doc *fbiw.Document) *SearchWindow {
	win := &SearchWindow{
		app: app,
		doc: doc,

		keyRow: -1,
		keyCol: -1,
	}
	win.searchBox = doc.QuerySelector(`#search`)
	win.resultBox = doc.QuerySelector(`#result`)
	win.resultList = doc.QuerySelector(`#result-list`).(*fbiw.Scroll)
	win.resultStatus = doc.QuerySelector(`#result-status`).(*fbiw.Text)
	win.resultStatusWrapper = doc.QuerySelector(`#result-status-wrapper`)
	win.txtQuery = doc.QuerySelector(`#query`).(*fbiw.Text)
	win.keyboard = doc.QuerySelector(`#keyboard`)
	win.doc.Listen(fbiw.StickDownEvent, win.handleEvents, fbiw.EventOptions{})
	go win.asyncInitAllSearchableItems()
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
		if t == `` {
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
	case fbiw.X:
		s := w.txtQuery.GetText()
		if s == `` {
			return
		}
		w.searchBox.SetProp(`display`, `false`)
		w.resultBox.SetProp(`display`, `true`)
		w.resultList.SetProp(`display`, `false`)
		w.resultStatusWrapper.SetProp(`display`, `true`)
		go w.asyncSearch(context.Background(), s)
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
	romPath     string
}

func (w *SearchWindow) handleResultEvents(event *fbiw.Event) {
	if event.Stick.Name == fbiw.B {
		w.resultBox.SetProp(`display`, `false`)
		w.searchBox.SetProp(`display`, `true`)
		return
	}

	if event.Stick.Name == fbiw.A {
		index := w.resultList.DataIndex()
		if index < 0 {
			return
		}
		matched := w.resultList.GetData(`matched`).([]_SearchResultItem)
		item := matched[index]
		w.app.Detach()
		go func() {
			defer w.app.AttachAsync()
			cmd := exec.Command(item.launcher.LauncherScriptPath(), item.romPath)
			log.Println(`启动进程：`, cmd.String())
			cmd.Run()
		}()
		return
	}
}

func (w *SearchWindow) asyncSearch(ctx context.Context, search string) {
	for w.allSearchableItems.Load() == nil {
		w.app.Async(func() {
			w.resultStatus.SetText(`游戏列表尚未初始化完成，等待中...`)
		})
		select {
		case <-ctx.Done():
			return
		default:
			time.Sleep(time.Second)
		}
	}

	w.app.Async(func() {
		w.resultStatus.SetText(`搜索中...`)
	})

	time.Sleep(time.Millisecond * 500)

	items := w.allSearchableItems.Load().([]_SearchResultItem)

	matched := []_SearchResultItem{}

	for _, item := range items {
		if searchable.Match(search, item.displayName, item.romPath) {
			matched = append(matched, item)
		}
	}

	// 名字越短说明得分越高？
	slices.SortFunc(matched, func(a, b _SearchResultItem) int {
		return len(a.displayName) - len(b.displayName)
	})

	w.app.Async(func() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		type _SearchResultItemView struct {
			root fbiw.Box
			name *fbiw.Text `css:".name"`
			path *fbiw.Text `css:".path"`
		}

		log.Println(`搜索结果:`, len(matched))
		w.resultList.SetData(`matched`, matched)
		w.resultList.SetItems(len(matched),
			func() (root fbiw.Box, user any) {
				view := fbiw.Unmarshal[_SearchResultItemView](w.doc, `
<block padding=10>
	<inline><text class="name"></text></inline>
	<inline><text class="path" font-size=15></text></inline>
</block>
				`)
				return view.root, view
			},
			func(user any, index int) {
				view := user.(*_SearchResultItemView)
				item := matched[index]
				view.name.SetText(item.displayName)
				view.path.SetText(item.romPath)
			},
		)
		w.resultList.Activate()

		w.resultStatusWrapper.SetProp(`display`, `false`)
		w.resultList.SetProp(`display`, `true`)
	})
}

// 暂时只读游戏列表。
func (w *SearchWindow) asyncInitAllSearchableItems() {
	log.Println(`异步加载所有游戏列表中...`)
	defer log.Println(`异步加载所有游戏列表完成。`)
	items := []_SearchResultItem{}
	emus := loadDir(filepath.Join(_SDCARDRoot, `Emus`))
	for _, emu := range emus {
		romDir := emu.RomDir()
		filepath.WalkDir(romDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				log.Println(`搜索遇到错误:`, err)
				return nil
			}
			if strings.HasPrefix(d.Name(), `.`) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}

			filename := d.Name()

			if !d.IsDir() && !emu.IncludesName(filename) {
				return nil
			}

			translated := game_names.Translate(filename)
			displayName := fbiw.Iif(translated != ``, translated, filename)
			items = append(items, _SearchResultItem{
				launcher:    emu,
				displayName: displayName,
				romPath:     path,
			})
			return nil
		})
	}
	w.allSearchableItems.Store(items)
}
