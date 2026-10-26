package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os/exec"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbiw/input/sticks"
	"github.com/movsb/fbui/pkg/config"
	"github.com/movsb/fbui/pkg/helpers"
	"github.com/movsb/fbui/pkg/launcher"
	"github.com/movsb/fbui/pkg/mainui"
)

//go:embed *.html assets/wallpaper.png
var _embed embed.FS

type MainWindow struct {
	app *fbiw.App
	doc *fbiw.Document

	statusBarNav *StatusBarNavigator
	gamesNav     *GamesNavigator
	favoritesNav *SavedItemsNavigator
	recentNav    *SavedItemsNavigator
	portsNav     *LauncherNavigator
	appsNav      *LauncherNavigator
	toolsNav     *ToolsNavigator
	storeNav     *StoreNavigator
	lists        *mainui.Lists

	directoryGames bool
}

func NewMainWindow(app *fbiw.App) *MainWindow {
	doc := app.NewDesktop(_embed, `window.html`)

	win := &MainWindow{
		app:   app,
		doc:   doc,
		lists: mainui.NewLists(config.SDCARDRoot),

		directoryGames: loadDirectoryGames(config.SDCARDRoot),
	}

	doc.Bind(win)

	win.statusBarNav = NewStatusBarNavigator(win)
	win.gamesNav = NewGamesNavigator(win)
	win.favoritesNav = NewSavedItemsNavigator(win, `#favorites`, true)
	win.recentNav = NewSavedItemsNavigator(win, `#recent`, false)
	win.portsNav = NewLauncherNavigator(win, `#ports`, `ports`)
	win.appsNav = NewLauncherNavigator(win, `#apps`, `apps`)
	win.toolsNav = NewToolsNavigator(win)
	win.storeNav = NewStoreNavigator(win)

	go win.watchLaunchConfigs()

	win.statusBarNav.activate()

	return win
}

func (w *MainWindow) launchItem(item mainui.Item) {
	if err := item.Validate(); err != nil {
		w.showLaunchError(err)
		return
	}
	if err := w.lists.AddRecent(item); err != nil {
		log.Println("更新最近项目失败：", err)
	}
	attach := w.detachForExternalProgram()
	go func() {
		defer attach()
		var err error
		if item.ROMPath != "" {
			err = launcher.RunScript(context.Background(), item.Launch, item.ROMPath)
		} else {
			cmd := exec.Command(item.Launch)
			log.Println("启动进程：", cmd.String())
			err = cmd.Run()
		}
		if err != nil {
			log.Printf("启动失败：%s: %v", item, err)
			w.doc.Async(func() { w.showLaunchError(err) })
		}
	}()
}

// detachForExternalProgram 在把显示和输入交给游戏或外部程序时恢复摇杆，
// 并返回一个必须在外部程序退出后调用的函数。恢复 fbui 时，要先 Attach
// 再禁用摇杆，保证标记文件的状态始终和 fbui 是否在前台一致。
func (w *MainWindow) detachForExternalProgram() func() {
	w.app.Detach()
	if err := helpers.EnableJoypad(); err != nil {
		log.Printf("恢复摇杆失败：%v", err)
	}
	return func() {
		w.app.Async(func() {
			w.app.Attach()
			if err := helpers.DisableJoypad(); err != nil {
				log.Printf("禁用摇杆失败：%v", err)
			}
		})
	}
}

func (w *MainWindow) showLaunchError(err error) {
	w.app.ShowAlertDialog(w.doc, fbiw.AlertDialogOptions{
		Title: "启动失败", Description: err.Error(),
	})
}

type StatusBarNavigator struct {
	window       *MainWindow
	catBar       fbiw.Box `css:"#cat-bar"`
	catIndex     int
	pagination   *fbiw.Text `css:"#pagination"`
	catBoxes     []fbiw.Box `css:"#cat-bar text"`
	contentBoxes []fbiw.Box `css:"#content > *"`
}

func NewStatusBarNavigator(win *MainWindow) *StatusBarNavigator {
	n := StatusBarNavigator{
		window:   win,
		catIndex: 0,
	}
	win.doc.Bind(&n)

	// 初始选中项不一定是第一个。
	for i, cat := range n.catBoxes {
		if cat.ClassContains(`selected`) {
			n.catIndex = i
			break
		}
	}

	n.setIndex(n.catIndex)

	n.catBar.Listen(fbiw.InputDownEvent, n.handleEvents)
	return &n
}

func (n *StatusBarNavigator) showCatBar(show bool) {
	n.catBar.SetProp(`display`, fmt.Sprint(show))
}
func (n *StatusBarNavigator) showPagination(show bool) {
	n.pagination.SetProp(`display`, fmt.Sprint(show))
}

func (n *StatusBarNavigator) activate() {
	n.catBar.Activate()
	n.catBar.ClassAdd(`active`)
	overlayWindow.SetButtonHints(
		ButtonHint{Button: `↓`, Text: `进入`},
		ButtonHint{Button: `B`, Text: `退出`},
	)
}

func (n *StatusBarNavigator) activateContent() {
	if n.catIndex >= 0 && n.catIndex < len(n.catBoxes) {
		content := n.contentForCategory(n.catBoxes[n.catIndex].GetName())
		removeActive := true
		switch content.GetID() {
		case `games`:
			n.window.gamesNav.activate()
		case `favorites`:
			n.window.favoritesNav.activate()
		case `recent`:
			n.window.recentNav.activate()
		case `ports`:
			n.window.portsNav.activate()
		case `apps`:
			n.window.appsNav.activate()
		case `tools`:
			n.window.toolsNav.activate()
		case `store`:
			n.window.storeNav.activate()
		default:
			removeActive = false
		}
		if removeActive {
			n.catBar.ClassRemove(`active`)
		}
	}
}

func (n *StatusBarNavigator) handleEvents(event *fbiw.Event) {
	keyName := event.Input.Name

	if keyName == sticks.B {
		n.window.app.ShowAlertDialog(n.window.doc, fbiw.AlertDialogOptions{
			Title:       `退出？`,
			Description: `确定要退出吗？`,
			ActionText:  `退出`,
			CancelText:  `取消`,
			OnAction: func() {
				n.window.app.Quit()
			},
		})
		return
	}

	if n.catIndex <= 0 && keyName == sticks.Left {
		return
	}
	if n.catIndex >= len(n.catBoxes)-1 && keyName == sticks.Right {
		return
	}

	if keyName == sticks.Left || keyName == sticks.Right {
		newIndex := 0
		switch keyName {
		case sticks.Left:
			newIndex = n.catIndex - 1
		case sticks.Right:
			newIndex = n.catIndex + 1
		}
		n.setIndex(newIndex)
		event.StopPropagation()
		return
	}

	if keyName == sticks.Down {
		n.activateContent()
		event.StopPropagation()
	}
}

func (n *StatusBarNavigator) setIndex(newIndex int) {
	oldIndex := n.catIndex
	if oldIndex >= 0 && oldIndex < len(n.catBoxes) {
		t := n.catBoxes[oldIndex].(*fbiw.Text)
		t.ClassRemove(`selected`)
	}

	if newIndex >= 0 && newIndex < len(n.catBoxes) {
		t := n.catBoxes[newIndex].(*fbiw.Text)
		t.ClassAdd(`selected`)
		for _, box := range n.contentBoxes {
			box.Base().ClassRemove(`selected`)
		}
		n.contentForCategory(t.GetName()).Base().ClassAdd(`selected`)
		n.catIndex = newIndex

		switch t.GetName() {
		case `recent`:
			n.window.recentNav.reload()
		case `favorites`:
			n.window.favoritesNav.reload()
		}
	}
}

// 游戏入口根据所选模式复用目录或仓库页面。
func (n *StatusBarNavigator) contentForCategory(name string) fbiw.Box {
	if name == "games" && !n.window.directoryGames {
		name = "store"
	}
	for _, box := range n.contentBoxes {
		if box.GetID() == name {
			return box
		}
	}
	panic("unknown category: " + name)
}
