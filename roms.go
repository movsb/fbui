package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

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
		scroll := w.doc.GetBoxByID(`emus`).(*fbiw.Scroll)
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

type GamesNavigator struct {
	window     *MainWindow
	emus       *fbiw.Scroll
	roms       *fbiw.Scroll
	currentEmu *LaunchConfig

	// 当前的目录浏览栈。
	stack _RomStack
}

type _RomStack struct {
	stack []_RomDirInfo
}

func (s *_RomStack) Push(dirName string, roms []RomInfo, state any) {
	s.stack = append(s.stack, _RomDirInfo{
		name:  dirName,
		roms:  roms,
		state: state,
	})
}
func (s *_RomStack) At(index int) *_RomDirInfo {
	if index < 0 || index > s.Size()-1 {
		panic(`错误的栈索引`)
	}
	return &s.stack[index]
}
func (s *_RomStack) Pop() {
	if len(s.stack) <= 0 {
		panic(`没有栈数据`)
	}
	s.stack = s.stack[:len(s.stack)-1]
}
func (s *_RomStack) Size() int {
	return len(s.stack)
}
func (s *_RomStack) Top() *_RomDirInfo {
	if s.Size() < 1 {
		panic(`没有栈数据`)
	}
	return &s.stack[s.Size()-1]
}

type _RomDirInfo struct {
	// 当前目录名
	name string
	// 游戏列表
	roms []RomInfo

	// 滚动条状态
	state any
}

func (n *GamesNavigator) Navigate(name fbiw.KeyName) any {
	if n.stack.Size() == 0 {
		return n.navigateEmus(name)
	} else {
		return n.navigateRoms(name)
	}
}

func (n *GamesNavigator) navigateEmus(name fbiw.KeyName) any {
	// 模拟器界面，按B退出
	if name == fbiw.B {
		return false
	}
	// 按上回到标题
	if name == fbiw.Up && n.emus.DataRowIndex() == 0 {
		n.emus.Deselect()
		return false
	}

	// 按A进入游戏列表
	if name == fbiw.A {
		emuList := n.emus.GetData(`emus`).([]*LaunchConfig)
		emuIndex := n.emus.DataIndex()

		if emuIndex < 0 || emuIndex > len(emuList)-1 {
			return nil
		}

		emu := emuList[emuIndex]
		n.currentEmu = emu

		// 隐藏模拟器，显示游戏列表
		n.emus.SetProp(`display`, `false`)
		n.roms.SetProp(`display`, `true`)

		list := n.listRomsInDir(emu.RomDir())
		n.stack.Push(`.`, list, nil)
		n.setRomsList(list, nil)

		return nil
	}

	n.emus.Navigate(name)
	return nil
}

func (n *GamesNavigator) navigateRoms(name fbiw.KeyName) any {
	// 返回上一层。
	if name == fbiw.B {
		// 游戏列表的最上层了，返回模拟器列表。
		if n.stack.Size() <= 1 {
			n.roms.SetProp(`display`, `false`)
			n.emus.SetProp(`display`, `true`)
			n.stack.Pop()
			return nil
		}

		// 还有更多游戏上级列表。
		n.stack.Pop()
		top := n.stack.Top()
		n.setRomsList(top.roms, top.state)
		return nil
	}
	// 启动游戏或者进入新的目录。
	if name == fbiw.A {
		// 没有选中？
		index := n.roms.DataIndex()
		if index < 0 || index > n.roms.DataCount()-1 {
			return nil
		}

		info := n.stack.Top().roms[index]

		// 选中了目录？
		if info.isDir {
			list := n.listRomsInDir(n.romFinalPath(n.currentEmu, info))
			n.stack.Top().state = n.roms.GetState()
			n.stack.Push(info.name, list, nil)
			n.setRomsList(list, nil)
			return nil
		}

		// 选中了游戏？
		launcher := n.currentEmu.LauncherScriptPath()
		romPath := n.romFinalPath(n.currentEmu, info)
		n.window.app.Detach()
		go func() {
			defer n.window.app.AttachAsync()
			cmd := exec.Command(launcher, romPath)
			if err := cmd.Run(); err != nil {
				log.Printf(`运行失败：%s: %s: %s`, launcher, romPath, err.Error())
			}
		}()
		return nil
	}
	n.roms.Navigate(name)
	return nil
}

func (n *GamesNavigator) setRomsList(roms []RomInfo, state any) {
	type _RomBox struct {
		Root fbiw.Box
		Text *fbiw.Text `css:"text"`
	}

	n.roms.SetItems(len(roms),
		func() (fbiw.Box, any) {
			item := fbiw.Unmarshal[_RomBox](n.window.doc, `
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

	if state != nil {
		n.roms.SetState(state)
	}
}

type RomInfo struct {
	isDir bool
	// 原始文件系统文件名，不含路径。
	name string
	// 可能的友好显示名？
	displayName string
}

// 基于此rom信息，往上回溯父目录，拼出完整路径。
func (n *GamesNavigator) romFinalPath(launcher *LaunchConfig, rom RomInfo) string {
	parts := []string{launcher.RomDir()}
	for i := 0; i < n.stack.Size(); i++ {
		parts = append(parts, n.stack.At(i).name)
	}
	parts = append(parts, rom.name)
	return filepath.Join(parts...)
}

// 在当前目录枚举游戏列表（含子目录名）。
//   - 不会递归进子目录
//   - 目前放前面
func (n *GamesNavigator) listRomsInDir(dir string) []RomInfo {
	roms := []RomInfo{}

	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Println(err)
		return nil
	}

	for _, entry := range entries {
		roms = append(roms, RomInfo{
			isDir:       entry.IsDir(),
			name:        entry.Name(),
			displayName: entry.Name(),
		})
	}

	// 目录放前面。
	slices.SortFunc(roms, func(a, b RomInfo) int {
		if a.isDir && !b.isDir {
			return -1
		}
		if !a.isDir && b.isDir {
			return +1
		}
		return strings.Compare(a.displayName, b.displayName)
	})

	return roms
}
