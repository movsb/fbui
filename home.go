package main

import (
	"fmt"
	"log"
	"time"

	"github.com/movsb/fbiw"
)

func (w *MainWindow) initSystemTime() {
	txtTime := w.doc.QuerySelector(`#time`).(*fbiw.Text)
	txtTime.SetText(time.Now().Format(`15:04`))
	go func() {
		last := ``
		for range time.Tick(time.Minute) {
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
			t.ClassRemove(`selected`)
			b := contentBlocks[n.catIndex]
			b.Base().ClassRemove(`selected`)
		}
		switch name {
		case fbiw.Left:
			n.catIndex--
		case fbiw.Right:
			n.catIndex++
		}
		t := items[n.catIndex].(*fbiw.Text)
		t.ClassAdd(`selected`)
		b := contentBlocks[n.catIndex]
		b.Base().ClassAdd(`selected`)
		return nil
	}
	if name == fbiw.Down {
		t := items[n.catIndex].(*fbiw.Text)
		switch t.Name {
		case `apps`:
			return &_AppsNavigator{
				w:        n.w,
				scroll:   n.w.doc.QuerySelector(`#apps`).(*fbiw.Scroll),
				dataName: `apps`,
			}
		case `ports`:
			return &_AppsNavigator{
				w:        n.w,
				scroll:   n.w.doc.QuerySelector(`#ports`).(*fbiw.Scroll),
				dataName: `ports`,
			}
		case `games`:
			return &GamesNavigator{
				window: n.w,
				emus:   n.w.doc.QuerySelector(`#emus`).(*fbiw.Scroll),
				roms:   n.w.doc.QuerySelector(`#roms`).(*fbiw.Scroll),
			}
		}
	}
	return nil
}
