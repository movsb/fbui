package main

import (
	"context"
	"embed"
	_ "embed"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os/exec"
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

	navigators []Navigator
}

func (w *MainWindow) initSystemTime() {
	txtTime := w.doc.QuerySelector(`#time`).(*fbiw.Text)
	txtTime.SetText(time.Now().Format(`15:04:05`))
	go func() {
		last := ``
		for range time.Tick(time.Second * 1) {
			select {
			case <-w.app.Context().Done():
				return
			default:
				w.app.Async(func() {
					now := time.Now().Format(`15:04:05`)
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

func (w *MainWindow) asyncInitApps() {
	type _AppItem struct {
		Root  fbiw.Box
		Image *fbiw.Image `css:"img"`
		Text  *fbiw.Text  `css:"text"`
	}

	apps := LoadApps()
	w.app.Async(func() {
		container := w.doc.GetBoxByID(`apps`).(*fbiw.Scroll)
		container.SetData(`apps`, apps)
		container.SetItems(len(apps),
			func() (fbiw.Box, any) {
				item := fbiw.Unmarshal[_AppItem](w.doc, `
<block align=center padding=30>
	<img spacer>
	<text></text>
</block>
`)
				return item.Root, item
			},
			func(item any, index int) {
				app := apps[index]
				appItem := item.(*_AppItem)
				appItem.Image.SetPath(filepath.Join(app.Dir, app.Config.IconTop))
				appItem.Text.SetText(fbiw.Iif(app.Config.LabelChinese != ``, app.Config.LabelChinese, app.Config.Label))
			},
		)
	})
}

func (w *MainWindow) asyncInitRoms() {
	type _RomItem struct {
		Root fbiw.Box
		Text *fbiw.Text `css:"text"`
	}

	emus := LoadEmus()

	w.app.Async(func() {
		container := w.doc.GetBoxByID(`games`).(*fbiw.Scroll)
		container.SetItems(len(emus),
			func() (fbiw.Box, any) {
				item := fbiw.Unmarshal[_RomItem](w.doc, `
<block align=center padding=30>
	<img spacer>
	<text></text>
</block>
`)
				return item.Root, item
			},
			func(item any, index int) {
				app := emus[index]
				appItem := item.(*_RomItem)
				// appItem.Image.SetPath(filepath.Join(app.Dir, app.Config.IconTop))
				appItem.Text.SetText(fbiw.Iif(app.Config.LabelChinese != ``, app.Config.LabelChinese, app.Config.Label))
			},
		)
	})
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

type _TitleNavigator struct {
	w        *MainWindow
	catIndex int
}

func (n *_TitleNavigator) Navigate(name fbiw.KeyName) any {
	if n.catIndex == 0 && name == fbiw.Left {
		return nil
	}
	items := n.w.doc.QuerySelectorAll(`#cat-bar text`)
	contentBlocks := n.w.doc.GetBoxByID(`content`).Base().Children
	if n.catIndex == len(items)-1 && name == fbiw.Right {
		return nil
	}
	if name == fbiw.Left || name == fbiw.Right {
		// 原来的去掉选中
		if n.catIndex >= 0 && n.catIndex < len(items) {
			t := items[n.catIndex].(*fbiw.Text)
			t.Class.Remove(`selected`)
			b := contentBlocks[n.catIndex]
			b.Base().Class.Remove(`selected`)
		}
		switch name {
		case fbiw.Left:
			n.catIndex--
		case fbiw.Right:
			n.catIndex++
		}
		t := items[n.catIndex].(*fbiw.Text)
		t.Class.Add(`selected`)
		b := contentBlocks[n.catIndex]
		b.Base().Class.Add(`selected`)
		return nil
	}
	if name == fbiw.Down {
		t := items[n.catIndex].(*fbiw.Text)
		if t.Name == `apps` {
			return &_AppsNavigator{
				w:      n.w,
				scroll: n.w.doc.QuerySelector(`#apps`).(*fbiw.Scroll),
			}
		}
	}
	return nil
}

type _AppsNavigator struct {
	w      *MainWindow
	scroll *fbiw.Scroll
}

func (n *_AppsNavigator) Navigate(name fbiw.KeyName) any {
	if name == fbiw.B {
		log.Printf(`收到B按键`)
		return false
	}

	if name == fbiw.A && n.scroll.DataIndex() != -1 {
		apps := n.scroll.GetData(`apps`).([]*LaunchConfig)
		app := apps[n.scroll.DataIndex()]
		n.w.app.Detach()
		go func() {
			defer n.w.app.Async(func() {
				n.w.app.Attach()
			})
			exec.Command(filepath.Join(app.Dir, app.Config.Launch)).Run()
		}()
		return nil
	}

	if n.scroll.DataRowIndex() <= 0 && name == fbiw.Up {
		n.scroll.Deselect()
		return false
	}

	n.scroll.Navigate(name)

	return nil
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
	// go win.asyncInitRoms()

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
