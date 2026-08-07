package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/movsb/fbiw"
)

type RomInfo struct {
	path        string
	displayName string
	launcher    *LaunchConfig
}

func (w *MainWindow) asyncInitRoms() {
	type _RomBox struct {
		Root fbiw.Box
		Text *fbiw.Text `css:"text"`
	}

	now := time.Now()

	// 先找模拟器，再根据其支持的Roms路径找游戏。
	emus := LoadEmus()

	roms := []RomInfo{}

	for _, emu := range emus {
		romDir := emu.Config.RomPath
		if romDir == `` {
			continue
		}

		if !filepath.IsAbs(romDir) {
			romDir = filepath.Join(emu.Dir, romDir)
		}

		entries, err := os.ReadDir(romDir)
		if err != nil {
			log.Println(err)
			continue
		}

		for _, entry := range entries {
			roms = append(roms, RomInfo{
				launcher:    emu,
				path:        filepath.Join(romDir, entry.Name()),
				displayName: entry.Name(),
			})
		}
	}

	log.Printf(`ROM列表加载完成。总共：%d，耗时：%v`, len(roms), time.Since(now))

	w.app.Async(func() {
		container := w.doc.GetBoxByID(`games`).(*fbiw.Scroll)
		container.SetData(`roms`, roms)
		container.SetItems(len(roms),
			func() (fbiw.Box, any) {
				item := fbiw.Unmarshal[_RomBox](w.doc, `
<block padding=30>
	<inline spacer align=middle>
		<text></text>
	</inline>
</block>
`)
				return item.Root, item
			},
			func(item any, index int) {
				rom := roms[index]
				box := item.(*_RomBox)
				box.Text.SetText(rom.displayName)
			},
		)
	})
}

type _RomsNavigator struct {
	w      *MainWindow
	scroll *fbiw.Scroll
}

func (n *_RomsNavigator) Navigate(name fbiw.KeyName) any {
	if name == fbiw.B {
		log.Printf(`收到B按键`)
		n.scroll.Deselect()
		return false
	}

	if name == fbiw.A && n.scroll.DataIndex() != -1 {
		roms := n.scroll.GetData(`roms`).([]RomInfo)
		rom := roms[n.scroll.DataIndex()]
		n.w.app.Detach()
		go func() {
			defer n.w.app.Async(func() {
				n.w.app.Attach()
			})
			cmd := exec.Command(
				filepath.Join(rom.launcher.Dir, rom.launcher.Config.Launch),
				rom.path,
			)
			log.Println(`启动进程：`, cmd.String())
			cmd.Run()
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
