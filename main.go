package main

import (
	"context"
	"embed"
	_ "embed"
	"log"
	"net/http"
	_ "net/http/pprof"
	"path/filepath"

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

	navigators []Navigator
}

func (w *MainWindow) HandleKeyboardEvent(name fbiw.KeyName, pressed bool) {
	if len(w.navigators) <= 0 || !pressed {
		return
	}
	last := w.navigators[len(w.navigators)-1]

	next := last.Navigate(name)
	if next == nil {
		return
	} else if next == false {
		w.navigators = w.navigators[:len(w.navigators)-1]
		w.HandleKeyboardEvent(name, pressed)
	} else if nav, ok := next.(Navigator); ok {
		w.navigators = append(w.navigators, nav)
		w.HandleKeyboardEvent(name, pressed)
	} else {
		log.Panicf(`navigator 返回了无效值：%v`, next)
	}
}

type Navigator interface {
	// 返回值分几种情况：
	//  - 如果是nil，继续由自己导航。
	//  - 如果是Navigator，压栈此新的Navigator，并由它接管新的导航。
	//  - 如果是false，结束导航，回到前一个导航。
	Navigate(name fbiw.KeyName) any
}

func NewMainWindow(app *fbiw.App) *MainWindow {
	doc := app.New(`main.html`, `skin`)

	win := &MainWindow{
		app: app,
		doc: doc,
	}

	doc.SetDelegator(win)

	win.initSystemTime()
	win.initSystemPower()

	go win.asyncInitApps()
	go win.asyncInitEmus()
	go win.asyncInitPorts()

	win.navigators = append(win.navigators, &_TitleNavigator{
		w:        win,
		catIndex: 0,
	})

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
