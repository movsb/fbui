package main

import (
	"os/exec"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbiw/input/sticks"
	"github.com/movsb/fbui/pkg/battery_info"
	"github.com/movsb/fbui/pkg/config"
	"github.com/movsb/fbui/pkg/file_manager"
	"github.com/movsb/fbui/pkg/ssh"
	"github.com/movsb/fbui/pkg/swap_manager"
	"github.com/movsb/fbui/pkg/webdav_server"
)

type _ToolItemData struct {
	name  string
	click func()
}

type _ToolItemView struct {
	root fbiw.Box
	name *fbiw.Text `css:"text"`
}

func NewToolsNavigator(win *MainWindow) *ToolsNavigator {
	toolsNav := &ToolsNavigator{
		window: win,
		tools: []_ToolItemData{
			{
				name: `电池信息`,
				click: func() {
					battery_info.New(win.app, nil)
					overlayWindow.SetButtonHints(
						ButtonHint{Button: `X`, Text: `刷新`},
						ButtonHint{Button: `B`, Text: `返回`},
					)
				},
			},
			{
				name: `虚拟内存/交换空间(Swap)管理`,
				click: func() {
					swap_manager.New(win.app, swap_manager.NewBackend(config.SDCARDRoot))
					overlayWindow.SetButtonHints(
						ButtonHint{Button: `A`, Text: `启用/停用`},
						ButtonHint{Button: `X`, Text: `创建`},
						ButtonHint{Button: `Y`, Text: `删除`},
						ButtonHint{Button: `B`, Text: `返回`},
					)
				},
			},
			{
				name: `文件服务器（WebDAV）`,
				click: func() {
					webdav_server.New(win.app)
					overlayWindow.SetButtonHints(
						ButtonHint{Button: `A`, Text: `开启/关闭`},
						ButtonHint{Button: `B`, Text: `返回/关闭`},
					)
				},
			},
			{
				name: `文件管理器`,
				click: func() {
					file_manager.New(win.app, func(mode file_manager.Mode) {
						switch mode {
						case file_manager.ModeBrowse:
							overlayWindow.SetButtonHints(
								ButtonHint{Button: `A`, Text: `打开`},
								ButtonHint{Button: `Y`, Text: `菜单`},
								ButtonHint{Button: `B`, Text: `返回`},
							)
						case file_manager.ModePreview:
							overlayWindow.SetButtonHints(ButtonHint{Button: `B`, Text: `关闭`})
						}
					})
				},
			},
			{
				name: `远程登录（SSH）`,
				click: func() {
					ssh.New(win.app)
					overlayWindow.SetButtonHints(
						ButtonHint{Button: `A`, Text: `开启/关闭`},
						ButtonHint{Button: `B`, Text: `返回/关闭`},
					)
				},
			},
			{
				name: `重启系统`,
				click: func() {
					win.app.ShowAlertDialog(win.doc, fbiw.AlertDialogOptions{
						Title:         `重启系统？`,
						Description:   `确定要立即重启系统吗？`,
						ActionText:    `重启`,
						ActionVariant: fbiw.ButtonDestructive,
						CancelText:    `取消`,
						OnAction: func() {
							exec.Command(`reboot`).Start()
						},
					})
				},
			},
			{
				name: `关机`,
				click: func() {
					win.app.ShowAlertDialog(win.doc, fbiw.AlertDialogOptions{
						Title:         `关机？`,
						Description:   `确定要立即关机吗？`,
						ActionText:    `关机`,
						ActionVariant: fbiw.ButtonDestructive,
						CancelText:    `取消`,
						OnAction: func() {
							exec.Command(`poweroff`).Start()
						},
					})
				},
			},
		},
	}
	win.doc.Bind(toolsNav)
	toolsNav.scroll.SetItems(
		len(toolsNav.tools),
		func() (fbiw.Box, *_ToolItemView) {
			box := win.doc.Instantiate[_ToolItemView](`tool-item`)
			return box.root, box
		},
		func(box *_ToolItemView, index int) {
			box.name.SetText(win.toolsNav.tools[index].name)
		},
	)
	toolsNav.scroll.Listen(fbiw.InputDownEvent, toolsNav.handleEvents)
	return toolsNav
}

type ToolsNavigator struct {
	window *MainWindow
	scroll *fbiw.List `css:"#tools"`
	tools  []_ToolItemData
}

func (n *ToolsNavigator) activate() {
	n.scroll.SetIndex(0, 0, 0)
	n.scroll.Activate()
	overlayWindow.SetButtonHints(
		ButtonHint{Button: `A`, Text: `打开`},
		ButtonHint{Button: `B`, Text: `返回`},
	)
}

func (n *ToolsNavigator) handleEvents(event *fbiw.Event) {
	name := event.Input.Name
	if name == sticks.B || (name == sticks.Up && n.scroll.DataRowIndex() <= 0) {
		n.scroll.Deselect()
		n.window.statusBarNav.activate()
		return
	}

	if name == sticks.A && n.scroll.DataIndex() != -1 {
		tool := n.tools[n.scroll.DataIndex()]
		tool.click()
		event.StopPropagation()
		return
	}
}
