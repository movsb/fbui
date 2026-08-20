package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbui/pkg/config"
	"github.com/movsb/fbui/pkg/game_names"
	"github.com/movsb/fbui/pkg/search_window"
)

func (w *MainWindow) asyncInitEmus() {
	type _EmuItem struct {
		root  fbiw.Box
		image *fbiw.Image `css:"img"`
		text  *fbiw.Text  `css:"text"`
	}

	emus := config.LoadDir(filepath.Join(config.SDCARDRoot, `Emus`))

	w.app.Async(func() {
		scroll := w.doc.GetBoxByID[*fbiw.Scroll](`emus`)
		scroll.SetData(`emus`, emus)
		scroll.SetItems(len(emus),
			func() (fbiw.Box, *_EmuItem) {
				item := fbiw.Unmarshal[_EmuItem](w.doc, `
<block align=center padding=20>
	<img spacer>
	<text></text>
</block>
`)
				return item.root, item
			},
			func(item *_EmuItem, index int) {
				emu := emus[index]
				item.image.SetPath(emu.IconPath())
				item.text.SetText(emu.Name())
			},
		)
	})
}

type GamesNavigator struct {
	window     *MainWindow
	container  *fbiw.Stack  `css:"#games"`
	emus       *fbiw.Scroll `css:"#emus"`
	roms       *fbiw.Scroll `css:"#roms"`
	noGames    fbiw.Box     `css:"#nogames"`
	currentEmu *config.LaunchConfig

	// 当前的目录浏览栈。
	stack _RomStack
}

func NewGamesNavigator(win *MainWindow) *GamesNavigator {
	n := &GamesNavigator{
		window: win,
	}
	win.doc.Bind(n)
	n.container.Listen(fbiw.StickDownEvent, n.handleEvents, fbiw.EventOptions{})
	return n
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

func (n *GamesNavigator) activate() {
	n.emus.SetIndex(0, 0, 0)
	n.emus.Activate()
}

func (n *GamesNavigator) handleEvents(event *fbiw.Event) {
	name := event.Stick.Name

	// 模拟器界面
	if n.stack.Size() == 0 {
		// 冒泡到上一级回到标题
		if name == fbiw.B || (name == fbiw.Up && n.emus.DataRowIndex() <= 0) {
			n.emus.Deselect()
			n.window.statusBarNav.activate()
			return
		}

		// 按“Y”搜索
		if name == fbiw.Y {
			n.openSearch()
			return
		}

		// 按“A”进入游戏列表
		if name == fbiw.A {
			n.switchToRoms()
			return
		}
	} else {
		// 返回上一层。
		if name == fbiw.B {
			// 游戏列表的最上层了，返回模拟器列表。
			if n.stack.Size() <= 1 {
				n.backToEmulators()
			} else {
				n.gotoUpperDirectory()
			}
			event.StopPropagation()
			return
		}

		// 启动游戏或者进入新的目录。
		if name == fbiw.A {
			index := n.roms.DataIndex()
			// 没有选中？
			if index < 0 {
				return
			}
			info := n.stack.Top().roms[index]
			if info.isDir {
				// 选中了目录？
				n.enterDirectory(info)
			} else {
				// 选中了游戏？
				n.runGame(info)
			}
			event.StopPropagation()
			return
		}
	}
}

func (n *GamesNavigator) openSearch() {
	search_window.New(n.window.app)
}

func (n *GamesNavigator) backToEmulators() {
	n.roms.SetProp(`display`, `false`)
	n.emus.SetProp(`display`, `true`)
	n.noGames.Base().SetProp(`display`, `false`)
	n.stack.Pop()
	n.emus.Activate()
}

func (n *GamesNavigator) gotoUpperDirectory() {
	// 还有更多游戏上级列表。
	n.stack.Pop()
	top := n.stack.Top()
	n.setRomsList(top.roms, top.state)
}

func (n *GamesNavigator) switchToRoms() {
	emuIndex := n.emus.DataIndex()
	if emuIndex < 0 {
		return
	}

	emuList := n.emus.GetData(`emus`).([]*config.LaunchConfig)
	emu := emuList[n.emus.DataIndex()]
	n.currentEmu = emu

	// 隐藏模拟器，显示游戏列表
	n.emus.SetProp(`display`, `false`)
	n.roms.SetProp(`display`, `true`)

	list := n.listRomsInDir(emu, emu.RomDir())
	n.stack.Push(`.`, list, nil)
	n.setRomsList(list, nil)
	n.roms.Activate()
}

func (n *GamesNavigator) enterDirectory(info RomInfo) {
	path := n.romFinalPath(n.currentEmu, info)
	// TODO 这里是同步列举的，可能会卡界面
	list := n.listRomsInDir(n.currentEmu, path)
	n.stack.Top().state = n.roms.GetState()
	n.stack.Push(info.name, list, nil)
	n.setRomsList(list, nil)
	// 其实只需要激活一次就行。
	// 然后在退出最只有模拟器列表的时候切换激活。
	n.roms.Activate()
}

func (n *GamesNavigator) runGame(info RomInfo) {
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
}

func (n *GamesNavigator) setRomsList(roms []RomInfo, state any) {
	n.noGames.Base().SetProp(`display`, fmt.Sprint(len(roms) == 0))

	type _RomBox struct {
		root fbiw.Box
		icon fbiw.Box   `css:".icon"`
		name *fbiw.Text `css:".name"`
	}

	n.roms.SetItems(len(roms),
		func() (fbiw.Box, *_RomBox) {
			item := fbiw.Unmarshal[_RomBox](n.window.doc, `
<block padding=10>
	<inline spacer align=middle>
		<inline class="icon">
			<text class="nerd">&#xf07b;</text>
			<spacer width=10></spacer>
		</inline>
		<text class="name"></text>
	</inline>
</block>
`)
			return item.root, item
		},
		func(item *_RomBox, index int) {
			rom := roms[index]
			item.name.SetText(rom.displayName)
			item.icon.SetProp(`display`, fmt.Sprint(rom.isDir))
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
	// 可能的友好显示/翻译名？
	// 总是有值。有当前语言版本，则为当前语言版本，否则同 name。
	displayName string
}

// 基于此rom信息，往上回溯父目录，拼出完整路径。
func (n *GamesNavigator) romFinalPath(launcher *config.LaunchConfig, rom RomInfo) string {
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
//   - 不包含以“.”开头的文件
//   - 如果目录是空目录，则也不会包含
func (n *GamesNavigator) listRomsInDir(emu *config.LaunchConfig, dir string) []RomInfo {
	roms := []RomInfo{}

	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Println(err)
		return nil
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), `.`) {
			continue
		}

		fileName := entry.Name()

		if !entry.IsDir() && !emu.IncludesName(fileName) {
			continue
		}

		translated := game_names.Translate(fileName)
		displayName := fbiw.Iif(translated != ``, translated, fileName)
		roms = append(roms, RomInfo{
			isDir:       entry.IsDir(),
			name:        fileName,
			displayName: displayName,
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
