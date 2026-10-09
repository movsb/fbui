package main

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbiw/input/sticks"
	"github.com/movsb/fbui/pkg/config"
	"github.com/movsb/fbui/pkg/mainui"
	"github.com/movsb/fbui/pkg/menu_popup"
)

type _SavedItemView struct {
	root     fbiw.Box
	name     *fbiw.Text `css:".name"`
	sublabel *fbiw.Text `css:".sublabel"`
}

func (v *_SavedItemView) ListSelectionChanged(selected bool) {
	v.name.SetMarqueeRunning(selected)
}

var _ fbiw.ListSelectionAware = (*_SavedItemView)(nil)

type SavedItemsNavigator struct {
	window    *MainWindow
	list      *fbiw.List
	empty     fbiw.Box
	favorites bool
	items     []mainui.Item
}

func NewSavedItemsNavigator(window *MainWindow, selector string, favorites bool) *SavedItemsNavigator {
	n := &SavedItemsNavigator{
		window: window, favorites: favorites,
		list:  window.doc.QuerySelector[*fbiw.List](selector + ` list`),
		empty: window.doc.QuerySelector[fbiw.Box](selector + ` .empty`),
	}
	n.list.Listen(fbiw.InputDownEvent, n.handleDown)
	n.list.Listen(fbiw.InputUpEvent, n.handleUp)
	n.list.Listen(fbiw.ListSelectionChange, func(*fbiw.Event) { n.updatePagination() })
	n.reload()
	return n
}

func (n *SavedItemsNavigator) activate() {
	n.reload()
	n.list.SetIndex(0, 0, 0)
	n.list.Activate()
	n.window.statusBarNav.showPagination(true)
	n.window.statusBarNav.showCatBar(false)
	n.updatePagination()
	n.setButtonHints()
}

func (n *SavedItemsNavigator) setButtonHints() {
	overlayWindow.SetButtonHints(
		ButtonHint{Button: `A`, Text: `启动`},
		ButtonHint{Button: `Menu`, Text: `菜单`},
		ButtonHint{Button: `B`, Text: `返回`},
	)
}

func (n *SavedItemsNavigator) reload() {
	items, err := n.window.lists.Items(n.favorites)
	if err != nil {
		log.Println("读取 MainUI 列表失败：", err)
		items = nil
	}
	for i := range items {
		if items[i].SubLabel == "" {
			items[i].SubLabel = inferSubLabel(items[i])
		}
	}
	n.items = items
	n.empty.Base().SetProp(`display`, fmt.Sprint(len(items) == 0))
	n.list.SetProp(`display`, fmt.Sprint(len(items) != 0))
	n.list.SetItems(len(items),
		func() (fbiw.Box, *_SavedItemView) {
			view := n.window.doc.Instantiate[_SavedItemView](`saved-item`)
			return view.root, view
		},
		func(view *_SavedItemView, index int) {
			view.name.SetText(items[index].Label)
			view.sublabel.SetText(items[index].SubLabel)
		},
	)
}

func inferSubLabel(item mainui.Item) string {
	clean := filepath.ToSlash(filepath.Clean(item.Launch))
	if strings.Contains(clean, "/Ports/") {
		return "独立"
	}
	if strings.Contains(clean, "/Apps/") || strings.Contains(clean, "/usr/trimui/apps/") {
		return "应用"
	}
	for _, emu := range config.LoadDir(filepath.Join(config.SDCARDRoot, "Emus")) {
		if filepath.Clean(emu.LauncherScriptPath()) == filepath.Clean(item.Launch) {
			return emu.Name()
		}
	}
	return ""
}

func (n *SavedItemsNavigator) updatePagination() {
	index := n.list.DataIndex()
	text := ""
	if index >= 0 {
		text = fmt.Sprintf("%d/%d", index+1, len(n.items))
	}
	n.window.statusBarNav.pagination.SetText(text)
}

func (n *SavedItemsNavigator) handleDown(event *fbiw.Event) {
	name := event.Input.Name
	if name == sticks.A {
		if index := n.list.DataIndex(); index >= 0 && index < len(n.items) {
			n.window.launchItem(n.items[index])
			event.StopPropagation()
		}
	} else if name == sticks.B || (name == sticks.Up && n.list.DataRowIndex() <= 0) {
		n.list.Deselect()
		n.window.statusBarNav.showPagination(false)
		n.window.statusBarNav.showCatBar(true)
		n.window.statusBarNav.activate()
		event.StopPropagation()
	}
}

func (n *SavedItemsNavigator) handleUp(event *fbiw.Event) {
	if event.Input.Name != sticks.Menu {
		return
	}
	n.showMenu()
	event.StopPropagation()
}

func (n *SavedItemsNavigator) showMenu() {
	items := []menu_popup.MenuItem{}
	index := n.list.DataIndex()
	if index >= 0 && index < len(n.items) {
		item := n.items[index]
		favorite, err := n.window.lists.IsFavorite(item)
		if err != nil {
			log.Println("读取收藏失败：", err)
		}
		name := "加入收藏"
		if favorite {
			name = "取消收藏"
		}
		items = append(items, menu_popup.MenuItem{Name: name, Click: func() {
			if _, err := n.window.lists.ToggleFavorite(item); err != nil {
				log.Println("更新收藏失败：", err)
			}
			n.reload()
		}})
	}
	clearName := "清空最近"
	title := "清空最近项目？"
	description := "只会清空最近列表，不会删除游戏、应用或存档。"
	if n.favorites {
		clearName = "清空收藏"
		title = "清空收藏？"
		description = "只会清空收藏列表，不会删除游戏、应用或存档。"
	}
	items = append(items, menu_popup.MenuItem{Name: clearName + "...", Click: func() {
		n.window.app.ShowAlertDialog(n.window.doc, fbiw.AlertDialogOptions{
			Title: title, Description: description, ActionText: "清空", CancelText: "取消",
			OnAction: func() {
				if err := n.window.lists.Clear(n.favorites); err != nil {
					log.Println("清空 MainUI 列表失败：", err)
				}
				n.reload()
			},
		})
	}})
	menu_popup.NewMenuPopup(n.window.app, n.window.doc, items, nil, nil)
}
