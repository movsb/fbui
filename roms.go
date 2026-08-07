package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/movsb/fbiw"
)

func (w *MainWindow) asyncInitEmus() {
	type _EmuItem struct {
		Root  fbiw.Box
		Image *fbiw.Image `css:"img"`
		Text  *fbiw.Text  `css:"text"`
	}

	emus := loadDir(filepath.Join(_SDCARDRoot, `Emus`))

	w.app.Async(func() {
		scroll := w.doc.GetBoxByID(`games`).(*fbiw.Scroll)
		scroll.SetData(`emus`, emus)
		scroll.SetItems(len(emus),
			func() (fbiw.Box, any) {
				item := fbiw.Unmarshal[_EmuItem](w.doc, `
<block align=center padding=30>
	<img spacer>
	<text></text>
</block>
`)
				return item.Root, item
			},
			func(item any, index int) {
				emu := emus[index]
				emuItem := item.(*_EmuItem)
				emuItem.Image.SetPath(emu.IconPath())
				emuItem.Text.SetText(emu.Name())
			},
		)
	})
}

type EmusNavigator struct {
	window *MainWindow
	scroll *fbiw.Scroll
}

func (n *EmusNavigator) Navigate(name fbiw.KeyName) any {
	if name == fbiw.B {
		n.scroll.Deselect()
		return false
	}

	if name == fbiw.A && n.scroll.DataIndex() != -1 {
		emus := n.scroll.GetData(`emus`).([]*LaunchConfig)
		emu := emus[n.scroll.DataIndex()]
		n.window.app.Detach()
		go func() {
			defer n.window.app.Async(func() {
				n.window.app.Attach()
			})
			command := emu.LauncherScriptPath()
			_ = command
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

type RomInfo struct {
	// 所属的模拟器：决定由谁打开它。
	launcher *LaunchConfig

	// 相对于Roms目录的路径。
	// 如果 config.json 的 RomPath 写的是：../../Roms/FC，
	// 那么 path 可以是 xxx/重装机兵.zip。
	// 最终文件路径则是： launcher.Dir + config.json中RomPath + path。
	// 得到：/mnt/SDCARD/Emus/FC + ../../Roms/FC + xxx/重装机兵.zip。
	path        string
	displayName string
}

func (r *RomInfo) FinalPath() string {
	if filepath.IsAbs(r.launcher.Config.RomPath) {
		return filepath.Join(r.launcher.Config.RomPath, r.path)
	}
	return filepath.Join(r.launcher.Dir, r.launcher.Config.RomPath, r.path)
}

func (w *MainWindow) asyncInitRoms() {
	now := time.Now()

	// 先找模拟器，再根据其支持的Roms路径找游戏。
	emus := loadDir(filepath.Join(_SDCARDRoot, `Emus`))

	roms := []RomInfo{}

	for _, emu := range emus {
		romDir := emu.Config.RomPath
		if romDir == `` {
			continue
		}

		// 基本写的是相对路径，则相对于 config.json 所在目录。
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

	type _RomBox struct {
		Root fbiw.Box
		Text *fbiw.Text `css:"text"`
	}

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
