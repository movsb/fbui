package swap_manager

import (
	"embed"
	"fmt"
	"path/filepath"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbui/pkg/menu_popup"
)

//go:embed *.html
var assets embed.FS

type Window struct {
	app     *fbiw.App
	doc     *fbiw.Document
	backend *Backend
	scroll  *fbiw.Scroll `css:"#swaps"`
	empty   fbiw.Box     `css:"#empty"`
	status  *fbiw.Text   `css:"#status"`
	entries []Entry
	busy    bool
}

type entryView struct {
	root   fbiw.Box
	path   *fbiw.Text `css:".path"`
	detail *fbiw.Text `css:".detail"`
	state  *fbiw.Text `css:".state"`
}

func New(app *fbiw.App, backend *Backend) *Window {
	window := &Window{app: app, backend: backend, doc: app.NewDesktop(assets, "window.html")}
	window.doc.Bind(window)
	window.scroll.Listen(fbiw.StickDownEvent, window.handleEvents)
	window.scroll.Activate()
	window.refresh("正在读取交换空间…")
	return window
}

func (w *Window) setStatus(message string, warning bool) {
	w.status.SetText(message)
	if warning {
		w.status.ClassAdd("warning")
	} else {
		w.status.ClassRemove("warning")
	}
}

func (w *Window) refresh(message string) {
	w.busy = true
	w.setStatus(message, false)
	go func() {
		entries, err := w.backend.List()
		w.app.Async(func() {
			w.busy = false
			w.entries = entries
			w.render()
			if err != nil {
				w.setStatus(err.Error(), true)
			} else {
				w.setStatus(fmt.Sprintf("共 %d 项", len(entries)), false)
			}
		})
	}()
}

func (w *Window) render() {
	w.scroll.SetItems(len(w.entries), func() (fbiw.Box, *entryView) {
		view := fbiw.Unmarshal[entryView](w.doc, `
<block padding="0 10" align=middle>
	<inline align="both"><text class="path" font-size="small"></text><spacer></spacer><text class="state" font-size="x-small"></text></inline>
	<inline class="mono muted" font-size="x-small"><text class="detail"></text></inline>
</block>`)
		return view.root, view
	}, func(view *entryView, index int) {
		entry := w.entries[index]
		view.path.SetText(entry.Path)
		usage := fmt.Sprintf("%s / %s / %.0f%%", FormatBytes(entry.SizeBytes), FormatBytes(entry.UsedBytes), entry.Usage()*100)
		view.detail.SetText(usage)
		state := "已启用"
		if !entry.Active {
			state = "未启用"
		}
		if entry.Error != "" {
			state = entry.Error
		}
		view.state.SetText(state)
	})
	w.scroll.SetProp("display", fmt.Sprint(len(w.entries) > 0))
	w.empty.SetProp("display", fmt.Sprint(len(w.entries) == 0))
	if len(w.entries) > 0 {
		w.scroll.Activate()
	}
}

func (w *Window) handleEvents(event *fbiw.Event) {
	if w.busy {
		return
	}
	switch event.Stick.Name {
	case fbiw.B:
		w.doc.Close()
	case fbiw.A:
		index := w.scroll.DataIndex()
		if index >= 0 && index < len(w.entries) {
			w.confirmSetActive(w.entries[index])
		}
	case fbiw.X:
		w.openSizeMenu()
	case fbiw.Y:
		index := w.scroll.DataIndex()
		if index >= 0 && index < len(w.entries) {
			w.confirmDelete(w.entries[index])
		}
	}
}

func (w *Window) confirmSetActive(entry Entry) {
	active := !entry.Active
	action := "启用"
	description := fmt.Sprintf("确定启用“%s”？", entry.Path)
	variant := fbiw.ButtonPrimary
	if !active {
		action = "停用"
		description = fmt.Sprintf("确定停用“%s”？正在使用的交换数据会被移回内存。", entry.Path)
		variant = fbiw.ButtonDestructive
	}
	w.app.ShowAlertDialog(w.doc, fbiw.AlertDialogOptions{
		Title:         action + " Swap？",
		Description:   description,
		ActionText:    action,
		ActionVariant: variant,
		CancelText:    "取消",
		OnAction:      func() { w.setActive(entry, active) },
	})
}

func (w *Window) setActive(entry Entry, active bool) {
	w.busy = true
	action := "停用"
	if active {
		action = "启用"
	}
	w.setStatus("正在"+action+" Swap…", false)
	go func() {
		err := w.backend.SetActive(entry, active)
		w.app.Async(func() {
			w.busy = false
			if err != nil {
				w.setStatus(action+"失败："+err.Error(), true)
				return
			}
			w.refresh(action + "成功，正在刷新…")
		})
	}()
}

func (w *Window) openSizeMenu() {
	if err := w.backend.CheckTools("mkswap", "swapon"); err != nil {
		w.setStatus(err.Error(), true)
		return
	}
	items := make([]menu_popup.MenuItem, 0, len(Sizes))
	for _, value := range Sizes {
		size := value
		items = append(items, menu_popup.MenuItem{
			Name:  FormatBytes(size),
			Click: func() { w.create(size) },
		})
	}
	menu_popup.NewMenuPopup(w.app, w.doc, items, nil, nil)
}

func (w *Window) create(size int64) {
	w.busy = true
	w.setStatus("正在创建并启用 Swap，请稍候…", false)
	go func() {
		path, err := w.backend.Create(size, func(p float32) {
			w.app.Async(func() {
				w.setStatus(fmt.Sprintf("正在创建并启用 Swap，请稍候…%d%%", int(p)), false)
			})
		})
		w.app.Async(func() {
			w.busy = false
			if err != nil {
				w.setStatus("创建失败："+err.Error(), true)
				return
			}
			w.refresh("已创建 " + filepath.Base(path) + "，正在刷新…")
		})
	}()
}

func (w *Window) confirmDelete(entry Entry) {
	if entry.Managed && entry.Error == "文件不存在" {
		w.delete(entry)
		return
	}
	if !entry.Regular {
		w.setStatus("只能删除当前存在的普通文件型 Swap", true)
		return
	}
	w.app.ShowAlertDialog(w.doc, fbiw.AlertDialogOptions{
		Title: "删除 Swap 文件？", Description: fmt.Sprintf("确定停用并删除“%s”？此操作无法撤销。", entry.Path),
		ActionText: "删除", ActionVariant: fbiw.ButtonDestructive, CancelText: "取消",
		OnAction: func() { w.delete(entry) },
	})
}

func (w *Window) delete(entry Entry) {
	w.busy = true
	w.setStatus("正在停用并删除 Swap…", false)
	go func() {
		err := w.backend.Delete(entry)
		w.app.Async(func() {
			w.busy = false
			if err != nil {
				w.setStatus("删除失败："+err.Error(), true)
				return
			}
			w.refresh("删除成功，正在刷新…")
		})
	}()
}
