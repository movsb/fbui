package search_window

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"math"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbiw/input/sticks"
	"github.com/movsb/fbui/pkg/config"
	"github.com/movsb/fbui/pkg/game_names"
	"github.com/movsb/fbui/pkg/searchable"
)

//go:embed *.html
var _embed embed.FS

type Mode uint8

const (
	ModeKeyboard Mode = iota
	ModeResults
	ModeClosed
)

type SearchWindow struct {
	app *fbiw.App
	doc *fbiw.Document

	searchBox  fbiw.Box   `css:"#search"`
	resultBox  fbiw.Box   `css:"#result"`
	resultList *fbiw.List `css:"#result-list"`

	resultStatus        *fbiw.Text `css:"#result-status"`
	resultStatusWrapper fbiw.Box   `css:"#result-status-wrapper"`

	txtQuery *fbiw.Text `css:"#query"`
	keyboard fbiw.Box   `css:"#keyboard"`

	prevKey        fbiw.Box
	keyRow, keyCol int
	onModeChange   func(Mode)
	onSelect       func(*fbiw.Document, Item)
	onQuery        func(string)
	queryOnly      bool

	// ctx                context.Context
	// cancel             context.CancelFunc
	allSearchableItems atomic.Value
}

// Item is an entry displayed and matched by the search window. Value is passed
// back unchanged when the user selects the item.
type Item struct {
	Name        string
	Detail      string
	SearchTerms []string
	// SearchGroups are ordered from the strongest to the weakest match. When
	// empty, SearchTerms is treated as one group for backward compatibility.
	SearchGroups [][]string
	Value        any
}

type loadResult struct {
	items []Item
	err   error
}

// initEmu: 初始化模拟器配置，如果没有，表示全部。
// initDir: 初始化游戏列表，没有表示全部。
func New(app *fbiw.App, opener *fbiw.Document, initEmu *config.LaunchConfig, initDir string, onModeChange func(Mode), onLaunch func(*config.LaunchConfig, string, string)) *SearchWindow {
	return NewItems(app, opener, onModeChange, func(context.Context) ([]Item, error) {
		return loadROMItems(initEmu, initDir), nil
	}, func(doc *fbiw.Document, item Item) {
		if onModeChange != nil {
			onModeChange(ModeClosed)
		}
		doc.Close()
		rom := item.Value.(_SearchResultItem)
		onLaunch(rom.launcher, rom.displayName, rom.romPath)
	})
}

// NewItems opens a search window backed by arbitrary in-memory items.
func NewItems(app *fbiw.App, opener *fbiw.Document, onModeChange func(Mode), load func(context.Context) ([]Item, error), onSelect func(*fbiw.Document, Item)) *SearchWindow {
	win := newWindow(app, opener, onModeChange)
	win.onSelect = onSelect
	go func() {
		items, err := load(context.Background())
		win.allSearchableItems.Store(loadResult{items: items, err: err})
	}()
	return win
}

// NewQuery opens only the keyboard portion of the search window. Submitting a
// non-empty query closes the popup and passes the query to onSubmit.
func NewQuery(app *fbiw.App, opener *fbiw.Document, initialQuery string, onModeChange func(Mode), onSubmit func(string)) *SearchWindow {
	win := newWindow(app, opener, onModeChange)
	win.queryOnly = true
	win.onQuery = onSubmit
	win.txtQuery.SetText(initialQuery)
	return win
}

func newWindow(app *fbiw.App, opener *fbiw.Document, onModeChange func(Mode)) *SearchWindow {
	doc := app.NewPopup(_embed, `search.html`, opener)
	win := &SearchWindow{
		app:          app,
		doc:          doc,
		keyRow:       -1,
		keyCol:       -1,
		onModeChange: onModeChange,
	}
	doc.Bind(win)
	win.searchBox.Listen(fbiw.InputDownEvent, win.handleSearchEvents)
	win.resultBox.Listen(fbiw.InputDownEvent, win.handleResultEvents)
	win.searchBox.Activate()
	win.setMode(ModeKeyboard)
	return win
}

func (w *SearchWindow) setMode(mode Mode) {
	if w.onModeChange != nil {
		w.onModeChange(mode)
	}
}

func (w *SearchWindow) handleSearchEvents(event *fbiw.Event) {
	// 按“Y”直接关闭窗口。
	if event.Input.Name == sticks.Y {
		w.setMode(ModeClosed)
		w.doc.Close()
		go func() {
			time.Sleep(time.Second)
			runtime.GC()
		}()
		return
	}

	if event.Input.Name == sticks.B {
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

	switch event.Input.Name {
	case sticks.Left, sticks.Right, sticks.Up, sticks.Down:
		w.switchKey(event)
	case sticks.A:
		if w.prevKey != nil {
			old := w.txtQuery.GetText()
			new := w.prevKey.Children()[0].(*fbiw.Text).GetText()
			w.txtQuery.SetText(old + new)
		}
	case sticks.X:
		s := w.txtQuery.GetText()
		if s == `` {
			return
		}
		if w.queryOnly {
			w.setMode(ModeClosed)
			w.doc.Close()
			w.onQuery(s)
			return
		}
		w.searchBox.SetProp(`display`, `false`)
		w.resultBox.SetProp(`display`, `true`)
		w.resultList.SetProp(`display`, `false`)
		w.resultStatusWrapper.SetProp(`display`, `true`)
		w.resultBox.Activate()
		w.setMode(ModeResults)
		go w.asyncSearch(context.Background(), s)
	}
}

func (w *SearchWindow) switchKey(event *fbiw.Event) {
	rows := w.keyboard.Children()
	set := func(r, c int) {
		if 0 <= r && r < len(rows) {
			row := rows[r]
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
		switch event.Input.Name {
		case sticks.Up:
			set(len(rows)-1, 0)
		case sticks.Right:
			set(0, 0)
		case sticks.Down:
			set(0, 0)
		case sticks.Left:
			set(0, -1)
		}
	} else {
		switch event.Input.Name {
		case sticks.Up:
			if w.keyRow > 0 {
				set(w.keyRow-1, centeredKeyColumn(w.keyCol, len(rows[w.keyRow].Children()), len(rows[w.keyRow-1].Children())))
			}
		case sticks.Right:
			set(w.keyRow, w.keyCol+1)
		case sticks.Down:
			if w.keyRow+1 < len(rows) {
				set(w.keyRow+1, centeredKeyColumn(w.keyCol, len(rows[w.keyRow].Children()), len(rows[w.keyRow+1].Children())))
			}
		case sticks.Left:
			set(w.keyRow, w.keyCol-1)
		}
	}
}

func centeredKeyColumn(column, fromColumns, toColumns int) int {
	if fromColumns <= 0 || toColumns <= 0 {
		return 0
	}
	mapped := int(math.Floor(float64(column) + float64(toColumns-fromColumns)/2))
	return min(max(mapped, 0), toColumns-1)
}

type _SearchResultItem struct {
	launcher    *config.LaunchConfig
	displayName string
	romPath     string
}

func (w *SearchWindow) handleResultEvents(event *fbiw.Event) {
	if event.Input.Name == sticks.B {
		w.resultBox.SetProp(`display`, `false`)
		w.searchBox.SetProp(`display`, `true`)
		w.searchBox.Activate()
		w.setMode(ModeKeyboard)
		return
	}

	if event.Input.Name == sticks.A {
		index := w.resultList.DataIndex()
		if index < 0 {
			return
		}
		matched := w.resultList.GetData(`matched`).([]Item)
		item := matched[index]
		w.onSelect(w.doc, item)
		return
	}
}

type _SearchResultItemView struct {
	root fbiw.Box
	name *fbiw.Text `css:".name"`
	path *fbiw.Text `css:".path"`
}

// ScrollSelectionChanged implements [fbiw.ListSelectionAware].
func (view *_SearchResultItemView) ListSelectionChanged(selected bool) {
	view.name.SetMarqueeRunning(selected)
	view.path.SetMarqueeRunning(selected)
}

func (w *SearchWindow) asyncSearch(ctx context.Context, search string) {
	for w.allSearchableItems.Load() == nil {
		w.doc.Async(func() {
			w.resultStatus.SetText(`游戏列表尚未初始化完成，等待中...`)
		})
		select {
		case <-ctx.Done():
			return
		default:
			time.Sleep(time.Second)
		}
	}

	w.doc.Async(func() {
		w.resultStatus.SetText(`搜索中...`)
	})

	time.Sleep(time.Millisecond * 100)

	loaded := w.allSearchableItems.Load().(loadResult)
	if loaded.err != nil {
		w.doc.Async(func() { w.resultStatus.SetText(`加载搜索数据失败：` + loaded.err.Error()) })
		return
	}
	items := loaded.items

	matched := MatchItems(items, search)

	w.doc.Async(func() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		log.Println(`搜索结果:`, len(matched))
		w.resultList.SetData(`matched`, matched)
		w.resultList.SetItems(len(matched),
			func() (fbiw.Box, *_SearchResultItemView) {
				view := w.doc.Instantiate[_SearchResultItemView](`search-result-item`)
				return view.root, view
			},
			func(box *_SearchResultItemView, index int) {
				item := matched[index]
				box.name.SetText(item.Name)
				box.path.SetText(item.Detail)
			},
		)

		if len(matched) > 0 {
			w.resultList.Activate()
			w.resultStatusWrapper.SetProp(`display`, `false`)
			w.resultList.SetProp(`display`, `true`)
		} else {
			w.resultStatus.SetText(`没有结果。`)
		}
	})
}

// MatchItems filters and ranks items by their first matching search group.
func MatchItems(items []Item, search string) []Item {
	type rankedItem struct {
		item  Item
		rank  int
		order int
	}
	ranked := make([]rankedItem, 0, len(items))
	for order, item := range items {
		groups := item.SearchGroups
		if len(groups) == 0 {
			groups = [][]string{item.SearchTerms}
		}
		for rank, terms := range groups {
			if searchable.Match(search, terms...) {
				ranked = append(ranked, rankedItem{item: item, rank: rank, order: order})
				break
			}
		}
	}
	slices.SortFunc(ranked, func(a, b rankedItem) int {
		if a.rank != b.rank {
			return a.rank - b.rank
		}
		if len(a.item.Name) != len(b.item.Name) {
			return len(a.item.Name) - len(b.item.Name)
		}
		return a.order - b.order
	})
	matched := make([]Item, len(ranked))
	for i := range ranked {
		matched[i] = ranked[i].item
	}
	return matched
}

// 暂时只读游戏列表。
func loadROMItems(initEmu *config.LaunchConfig, initDir string) []Item {
	log.Println(`异步加载戏列表中...`)
	defer log.Println(`异步加载游戏列表完成。`)

	items := []Item{}

	if initEmu != nil {
		addROMDir(&items, initEmu, initDir)
	} else {
		emus := config.LoadDir(filepath.Join(config.SDCARDRoot, `Emus`))
		for _, emu := range emus {
			addROMDir(&items, emu, emu.RomDir())
		}
	}

	return items
}

// 这样拆文件名再删除后缀比在每次搜索时处理更快。
func romSearchName(s string) string {
	if slash := strings.LastIndexByte(s, '/'); slash > 0 {
		s = s[slash+1:]
	}
	if dot := strings.LastIndexByte(s, '.'); dot > 0 {
		s = s[:dot]
	}
	return s
}

func addROMDir(out *[]Item, emu *config.LaunchConfig, dir string) {
	log.Println(`搜索目录:`, dir)
	items := []Item{}
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
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
		rom := _SearchResultItem{launcher: emu, displayName: displayName, romPath: path}
		items = append(items, Item{
			Name:        displayName,
			Detail:      path,
			SearchTerms: []string{romSearchName(displayName), romSearchName(path)},
			Value:       rom,
		})
		return nil
	})
	*out = append(*out, items...)
}
