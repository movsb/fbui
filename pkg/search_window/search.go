package search_window

import (
	"context"
	"embed"
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
	"github.com/movsb/fbui/pkg/config"
	"github.com/movsb/fbui/pkg/game_names"
	"github.com/movsb/fbui/pkg/searchable"
)

//go:embed *.html
var _embed embed.FS

type SearchWindow struct {
	app *fbiw.App
	doc *fbiw.Document

	searchBox  fbiw.Box     `css:"#search"`
	resultBox  fbiw.Box     `css:"#result"`
	resultList *fbiw.Scroll `css:"#result-list"`

	resultStatus        *fbiw.Text `css:"#result-status"`
	resultStatusWrapper fbiw.Box   `css:"#result-status-wrapper"`

	txtQuery *fbiw.Text `css:"#query"`
	keyboard fbiw.Box   `css:"#keyboard"`

	prevKey        fbiw.Box
	keyRow, keyCol int

	// ctx                context.Context
	// cancel             context.CancelFunc
	allSearchableItems atomic.Value
}

func New(app *fbiw.App) *SearchWindow {
	doc := app.New(_embed, `search.html`)
	win := &SearchWindow{
		app:    app,
		doc:    doc,
		keyRow: -1,
		keyCol: -1,
	}
	doc.Bind(win)
	win.searchBox.Listen(fbiw.StickDownEvent, win.handleSearchEvents, fbiw.EventOptions{})
	win.resultBox.Listen(fbiw.StickDownEvent, win.handleResultEvents, fbiw.EventOptions{})
	win.searchBox.Activate()
	go win.asyncInitAllSearchableItems()
	app.Show(doc)
	return win
}

func (w *SearchWindow) handleSearchEvents(event *fbiw.Event) {
	// 按“Y”直接关闭窗口。
	if event.Stick.Name == fbiw.Y {
		w.doc.Close()
		return
	}

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
		w.resultBox.Activate()
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
	launcher    *config.LaunchConfig
	displayName string
	romPath     string
}

func (w *SearchWindow) handleResultEvents(event *fbiw.Event) {
	if event.Stick.Name == fbiw.B {
		w.resultBox.SetProp(`display`, `false`)
		w.searchBox.SetProp(`display`, `true`)
		w.searchBox.Activate()
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

	time.Sleep(time.Millisecond * 100)

	items := w.allSearchableItems.Load().([]_SearchResultItem)

	matched := []_SearchResultItem{}

	// 这样把拆文件名再删除后缀要快。
	// 几乎所有的文件名都成立。
	// 应用名/移植名除外。
	nameOnly := func(s string) string {
		if slash := strings.LastIndexByte(s, '/'); slash > 0 {
			s = s[slash+1:]
		}
		if dot := strings.LastIndexByte(s, '.'); dot > 0 {
			s = s[:dot]
		}
		return s
	}

	for _, item := range items {
		name := nameOnly(item.displayName)
		path := nameOnly(item.romPath)
		if searchable.Match(search, name, path) {
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
			func() (fbiw.Box, *_SearchResultItemView) {
				view := fbiw.Unmarshal[_SearchResultItemView](w.doc, `
<block padding=10>
	<inline><text class="name"></text></inline>
	<inline><text class="path" font-size=15></text></inline>
</block>
				`)
				return view.root, view
			},
			func(box *_SearchResultItemView, index int) {
				item := matched[index]
				box.name.SetText(item.displayName)
				box.path.SetText(item.romPath)
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
	emus := config.LoadDir(filepath.Join(config.SDCARDRoot, `Emus`))
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
