package main

import (
	"fmt"
	"log"
	"os"
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

type savedItemsColumn struct {
	list       *fbiw.List
	empty      fbiw.Box
	items      []mainui.Item
	state      any
	stateCount int
}

type SavedItemsNavigator struct {
	window    *MainWindow
	container fbiw.Box
	columns   map[bool]*savedItemsColumn
	indices   map[bool]int
	favorites bool
	active    bool
}

func (n *SavedItemsNavigator) current() *savedItemsColumn {
	return n.columns[n.favorites]
}

func NewSavedItemsNavigator(window *MainWindow) *SavedItemsNavigator {
	n := &SavedItemsNavigator{
		window: window, favorites: loadCommonFavorites(config.SDCARDRoot),
		container: window.doc.QuerySelector[fbiw.Box]("#common"),
		columns:   make(map[bool]*savedItemsColumn),
		indices:   make(map[bool]int),
	}
	for favorites, selector := range map[bool]string{true: "#common-favorites", false: "#common-recent"} {
		column := &savedItemsColumn{
			list:  window.doc.QuerySelector[*fbiw.List](selector + " list"),
			empty: window.doc.QuerySelector[fbiw.Box](selector + " .empty"),
		}
		n.columns[favorites] = column
		column.list.Listen(fbiw.ListSelectionChange, func(*fbiw.Event) { n.updatePagination() })
	}
	n.container.ListenOptions(fbiw.InputDownEvent, n.handleDown, fbiw.EventOptions{Capture: true})
	n.container.Listen(fbiw.InputUpEvent, n.handleUp)
	n.reload()
	return n
}

func loadCommonFavorites(root string) bool {
	data, err := os.ReadFile(filepath.Join(root, ".fbui", "common-tab"))
	if err != nil && !os.IsNotExist(err) {
		log.Printf("读取常用分类失败：%v", err)
	}
	return strings.TrimSpace(string(data)) != "recent"
}

func (n *SavedItemsNavigator) activate() {
	n.active = true
	n.reload()
	n.focusList()
	n.window.statusBarNav.showPagination(true)
	n.window.statusBarNav.showCatBar(false)
	n.updatePagination()
	n.setButtonHints()
}

func (n *SavedItemsNavigator) focusList() {
	if len(n.current().items) == 0 {
		n.container.Activate()
	} else {
		n.current().list.Activate()
	}
}

func (n *SavedItemsNavigator) switchColumn(favorites bool) {
	if n.favorites == favorites {
		return
	}
	n.rememberSelection(n.favorites)
	n.current().list.Deselect()
	n.favorites = favorites
	dir := filepath.Join(config.SDCARDRoot, ".fbui")
	value := "recent"
	if favorites {
		value = "favorites"
	}
	err := os.MkdirAll(dir, 0o755)
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, "common-tab"), []byte(value+"\n"), 0o644)
	}
	if err != nil {
		log.Printf("保存常用分类失败：%v", err)
	}
	n.activate()
}

func (n *SavedItemsNavigator) setButtonHints() {
	overlayWindow.SetButtonHints(
		ButtonHint{Button: `A`, Text: `启动`},
		ButtonHint{Button: `↔`, Text: `最近/收藏`},
		ButtonHint{Button: `Menu`, Text: `菜单`},
		ButtonHint{Button: `B`, Text: `返回`},
	)
}

func (n *SavedItemsNavigator) reload() {
	for _, favorites := range []bool{true, false} {
		n.reloadColumn(favorites)
	}
}

func (n *SavedItemsNavigator) reloadColumn(favorites bool) {
	column := n.columns[favorites]
	column.list.ClassToggle("active-column", n.active && favorites == n.favorites)
	items, err := n.window.lists.Items(favorites)
	if err != nil {
		log.Println("读取 MainUI 列表失败：", err)
		items = nil
	}
	for i := range items {
		if items[i].SubLabel == "" {
			items[i].SubLabel = inferSubLabel(items[i])
		}
	}
	column.items = items
	column.empty.SetProp("display", fmt.Sprint(len(items) == 0))
	column.list.SetProp("display", fmt.Sprint(len(items) != 0))
	column.list.SetItems(len(items),
		func() (fbiw.Box, *_SavedItemView) {
			view := n.window.doc.Instantiate[_SavedItemView]("saved-item")
			return view.root, view
		},
		func(view *_SavedItemView, index int) {
			view.name.SetText(items[index].Label)
			view.sublabel.SetText(items[index].SubLabel)
		},
	)
	if n.active && favorites == n.favorites && len(items) > 0 {
		if column.state != nil && column.stateCount == len(items) {
			column.list.SetState(column.state)
		} else {
			index := max(0, min(n.indices[favorites], len(items)-1))
			column.list.SetIndex(0, 0, index)
		}
	}
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
	index := n.current().list.DataIndex()
	text := ""
	if index >= 0 {
		text = fmt.Sprintf("%d/%d", index+1, len(n.current().items))
	}
	n.window.statusBarNav.pagination.SetText(text)
}

func (n *SavedItemsNavigator) handleDown(event *fbiw.Event) {
	name := event.Input.Name
	if name == sticks.Left || name == sticks.Right {
		n.switchColumn(name == sticks.Right)
		event.StopPropagation()
		return
	}
	if name == sticks.A {
		if index := n.current().list.DataIndex(); index >= 0 && index < len(n.current().items) {
			n.window.launchItem(n.current().items[index])
			event.StopPropagation()
		}
	} else if name == sticks.B || (name == sticks.Up && n.current().list.DataRowIndex() <= 0) {
		n.deactivate()
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
	index := n.current().list.DataIndex()
	if index >= 0 && index < len(n.current().items) {
		item := n.current().items[index]
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
			n.rememberSelection(n.favorites)
			n.reload()
			n.focusList()
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
				n.indices[n.favorites] = 0
				n.current().state = nil
				n.reload()
				n.focusList()
			},
		})
	}})
	menu_popup.NewMenuPopup(n.window.app, n.window.doc, items, nil, nil)
}

func (n *SavedItemsNavigator) rememberSelection(favorites bool) {
	column := n.columns[favorites]
	if index := column.list.DataIndex(); index >= 0 {
		n.indices[favorites] = index
		column.state = column.list.GetState()
		column.stateCount = len(column.items)
	}
}

func (n *SavedItemsNavigator) deactivate() {
	for favorites, column := range n.columns {
		n.rememberSelection(favorites)
		column.list.Deselect()
		column.list.ClassRemove("active-column")
	}
	n.active = false
}
