package main

import (
	"github.com/movsb/fbiw"
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
				name:  `文件服务器（WebDAV）`,
				click: func() { webdav_server.New(win.app) },
			},
		},
	}
	win.doc.Bind(toolsNav)
	toolsNav.scroll.SetItems(
		len(toolsNav.tools),
		func() (fbiw.Box, any) {
			box := fbiw.Unmarshal[_ToolItemView](win.doc, `
<block padding=30>
	<inline spacer align=middle>
		<text></text>
	</inline>
</block>`)
			return box.root, box
		},
		func(user any, index int) {
			item := user.(*_ToolItemView)
			item.name.SetText(win.toolsNav.tools[index].name)
		},
	)
	toolsNav.scroll.Listen(fbiw.StickDownEvent, toolsNav.handleEvents, fbiw.EventOptions{})
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
