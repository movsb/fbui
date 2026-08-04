package main

import (
	"context"
	"embed"
	_ "embed"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"path/filepath"
	"time"

	"github.com/movsb/fbiw"
)

// pprof 性能测试用。
//
// go tool pprof -web  http://localhost:8888/debug/pprof/profile?seconds=30
func init() {
	go http.ListenAndServe(`0.0.0.0:8888`, nil)
}

func initFonts(app *fbiw.App) {
	if err := app.AddFont(
		`system`, false, false,
		`./fonts/MapleMonoNormalNL-NF-CN-Regular.ttf`,
	); err != nil {
		if err := app.AddFont(`system`, false, false, `/usr/trimui/res/full.ttf`); err != nil {
			log.Panic(`加载默认字体失败：`, err)
		}
	}
	for dir, faces := range map[string][]struct {
		FileName string
		Family   string
		Bold     bool
		Italic   bool
	}{
		`fonts/`: {
			{
				FileName: `MapleMonoNormalNL-NF-CN-Italic.ttf`,
				Family:   `system`,
				Bold:     false,
				Italic:   true,
			},
			{
				FileName: `MapleMonoNormalNL-NF-CN-Bold.ttf`,
				Family:   `system`,
				Bold:     true,
				Italic:   false,
			},
			{
				FileName: `MapleMonoNormalNL-NF-CN-BoldItalic.ttf`,
				Family:   `system`,
				Bold:     true,
				Italic:   true,
			},
		},
	} {
		for _, face := range faces {
			if err := app.AddFont(
				face.Family, face.Bold, face.Italic,
				filepath.Join(dir, face.FileName),
			); err != nil {
				log.Printf(`加载字体失败：%v`, err)
			}
		}
	}
}

//go:embed *.html
var embedded embed.FS

type MainWindow struct {
	app *fbiw.App
	doc *fbiw.Document

	// 当前选中的分类
	catIndex             int
	navigatingCategories bool
}

func (w *MainWindow) initSystemTime() {
	txtTime := w.doc.QuerySelector(`#time`).(*fbiw.Text)
	txtTime.SetText(time.Now().Format(`15:04`))
	go func() {
		last := ``
		for range time.Tick(time.Second * 5) {
			select {
			case <-w.app.Context().Done():
				return
			default:
				w.app.Async(func() {
					now := time.Now().Format(`15:04`)
					if now == last {
						return
					}
					last = now
					txtTime.SetText(now)
				})
			}
		}
	}()
}

func (w *MainWindow) initSystemPower() {
	txtPower := w.doc.QuerySelector(`#battery`).(*fbiw.Text)

	last := uint8(0)

	update := func() {
		info, err := ReadPowerStatus()
		if err != nil {
			log.Println(err)
			return
		}
		if info.Capacity == last {
			return
		}
		w.app.Async(func() {
			txtPower.SetText(fmt.Sprintf(`%d%%`, info.Capacity))
			last = info.Capacity
		})
	}

	update()

	go func() {
		for range time.Tick(time.Second * 10) {
			select {
			case <-w.app.Context().Done():
				return
			default:
				update()
			}
		}
	}()
}

func (w *MainWindow) HandleKeyboardEvent(name fbiw.KeyName, pressed bool) {
	if w.navigatingCategories && pressed {
		w.handleNavigating(name)
		return
	}
}

func (w *MainWindow) handleNavigating(name fbiw.KeyName) {
	if w.catIndex == 0 && name == fbiw.Left {
		return
	}
	items := w.doc.QuerySelectorAll(`#cat-bar text`)
	contentBlocks := w.doc.QuerySelectorAll(`#content block`)
	if w.catIndex == len(items)-1 && name == fbiw.Right {
		return
	}
	if name == fbiw.Left || name == fbiw.Right {
		// 原来的去掉选中
		if w.catIndex >= 0 && w.catIndex < len(items) {
			t := items[w.catIndex].(*fbiw.Text)
			t.Class.Remove(`selected`)
			b := contentBlocks[w.catIndex].(*fbiw.Block)
			b.Class.Remove(`selected`)
		}
		switch name {
		case fbiw.Left:
			w.catIndex--
		case fbiw.Right:
			w.catIndex++
		}
		t := items[w.catIndex].(*fbiw.Text)
		t.Class.Add(`selected`)
		b := contentBlocks[w.catIndex].(*fbiw.Block)
		b.Class.Add(`selected`)
	}
}

func NewMainWindow(app *fbiw.App) *MainWindow {
	doc := app.New(`main.html`, `skin`)

	win := &MainWindow{
		app: app,
		doc: doc,

		catIndex:             0,
		navigatingCategories: true,
	}

	doc.SetDelegator(win)

	win.initSystemTime()
	win.initSystemPower()

	return win
}

func main() {
	app := fbiw.NewApp(context.Background(), embedded)
	defer app.Close()

	initFonts(app)

	mainWindow := NewMainWindow(app)

	app.Show(mainWindow.doc)

	app.Run()
}
