package main

import (
	"os/exec"

	"github.com/movsb/fbiw"
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
				name:  `电池信息`,
				click: func() { battery_info.New(win.app, nil) },
			},
			{
				name:  `虚拟内存/交换空间(Swap)管理`,
				click: func() { swap_manager.New(win.app, swap_manager.NewBackend(config.SDCARDRoot)) },
			},
			{
				name:  `文件服务器（WebDAV）`,
				click: func() { webdav_server.New(win.app) },
			},
			{
				name:  `文件管理器`,
				click: func() { file_manager.New(win.app) },
			},
			{
				name:  `远程登录（SSH）`,
				click: func() { ssh.New(win.app) },
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
		},
	}
	win.doc.Bind(toolsNav)
	toolsNav.scroll.SetItems(
		len(toolsNav.tools),
		func() (fbiw.Box, *_ToolItemView) {
			box := fbiw.Unmarshal[_ToolItemView](win.doc, `
<block padding=10>
	<inline spacer align=middle>
		<text></text>
	</inline>
</block>`)
			return box.root, box
		},
		func(box *_ToolItemView, index int) {
			box.name.SetText(win.toolsNav.tools[index].name)
		},
	)
	toolsNav.scroll.Listen(fbiw.StickDownEvent, toolsNav.handleEvents)
	return toolsNav
}

type ToolsNavigator struct {
	window *MainWindow
	scroll *fbiw.Scroll `css:"#tools"`
	tools  []_ToolItemData
}

func (n *ToolsNavigator) activate() {
	n.scroll.SetIndex(0, 0, 0)
	n.scroll.Activate()
}

func (n *ToolsNavigator) handleEvents(event *fbiw.Event) {
	name := event.Stick.Name
	if name == fbiw.B || (name == fbiw.Up && n.scroll.DataRowIndex() <= 0) {
		n.scroll.Deselect()
		n.window.statusBarNav.activate()
		return
	}

	if name == fbiw.A && n.scroll.DataIndex() != -1 {
		tool := n.tools[n.scroll.DataIndex()]
		tool.click()
		event.StopPropagation()
		return
	}
}
