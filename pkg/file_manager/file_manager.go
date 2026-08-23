package file_manager

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbui/pkg/alert_window"
	"github.com/movsb/fbui/pkg/audio_player"
	"github.com/movsb/fbui/pkg/video_player"
)

//go:embed *.html
var _embed embed.FS

type FileManagerWindow struct {
	app *fbiw.App

	doc      *fbiw.Document
	root     fbiw.Box
	emptyDir fbiw.Box     `css:"#empty"`
	scroll   *fbiw.Scroll `css:"#scroll"`

	previewBox   *fbiw.Stack               `css:"#preview"`
	previewImage *fbiw.Image               `css:"#preview img"`
	previewVideo *video_player.VideoPlayer `css:"#preview video"`
	previewAudio *audio_player.AudioPlayer `css:"#preview audio"`
	previewText  *fbiw.Text                `css:"#preview text"`

	// 当前的目录浏览栈
	stack _Stack
}

func New(app *fbiw.App) *FileManagerWindow {
	n := &FileManagerWindow{
		app: app,
		doc: app.New(_embed, `file_manager.html`),
	}
	n.doc.Bind(n)
	n.root.Listen(fbiw.StickDownEvent, n.handleEvents, fbiw.EventOptions{})
	n.previewBox.Listen(fbiw.StickDownEvent, n.handlePreviewEvent, fbiw.EventOptions{})

	dir := fbiw.Iif(runtime.GOOS == `linux`, `/mnt/SDCARD`, os.ExpandEnv(`$HOME/Downloads`))
	components := []string{}
	for component := range strings.SplitSeq(path.Clean(dir), `/`) {
		if component == `` {
			component = `/`
		}
		components = append(components, component)
		if !n.enterDirectory(fs.FileInfoToDirEntry(fbiw.Must1(os.Stat(filepath.Join(components...))))) {
			alert_window.Alert(app, `无法进入目录。`, nil, nil)
			n.doc.Close()
			return nil
		}
	}

	n.activate()
	app.Show(n.doc)
	return n
}

type _Stack struct {
	stack []_StackItem
}

func (s *_Stack) Push(item _StackItem) {
	s.stack = append(s.stack, item)
}
func (s *_Stack) At(index int) *_StackItem {
	if index < 0 || index > s.Size()-1 {
		panic(`错误的栈索引`)
	}
	return &s.stack[index]
}
func (s *_Stack) Pop() {
	if len(s.stack) <= 0 {
		panic(`没有栈数据`)
	}
	s.stack = s.stack[:len(s.stack)-1]
}
func (s *_Stack) Size() int {
	return len(s.stack)
}
func (s *_Stack) Top() *_StackItem {
	if s.Size() < 1 {
		panic(`没有栈数据`)
	}
	return &s.stack[s.Size()-1]
}

type _StackItem struct {
	// 当前目录。
	dir fs.DirEntry

	// 其下的子项。
	entries []fs.DirEntry

	// 滚动条状态。
	state any
}

func (n *FileManagerWindow) activate() {
	// n.scroll.SetIndex(0, 0, 0)
	n.scroll.Activate()
}

func (n *FileManagerWindow) handleEvents(event *fbiw.Event) {
	name := event.Stick.Name

	// 返回上一层。
	if name == fbiw.B {
		if n.stack.Size() <= 1 {
			n.doc.Close()
			return
		} else {
			n.gotoUpperDirectory()
			event.StopPropagation()
			return
		}
	}

	// 预览文件或者进入新的目录。
	if name == fbiw.A {
		index := n.scroll.DataIndex()
		// 没有选中？
		if index < 0 {
			return
		}
		entry := n.stack.Top().entries[index]
		if entry.IsDir() {
			n.enterDirectory(entry)
		} else {
			n.preview(entry)
		}
		event.StopPropagation()
		return
	}
}

func (n *FileManagerWindow) gotoUpperDirectory() {
	n.stack.Pop()
	top := n.stack.Top()
	n.setFileList(top.entries, top.state)
}

func (n *FileManagerWindow) enterDirectory(entry fs.DirEntry) bool {
	// TODO 这里是同步列举的，可能会卡界面
	list, err := n.list(n.finalPath(entry))
	if err != nil {
		alert_window.Alert(n.app, err.Error(), nil, nil)
		return false
	}
	if n.stack.Size() > 0 {
		n.stack.Top().state = n.scroll.GetState()
	}
	n.stack.Push(_StackItem{
		dir:     entry,
		entries: list,
		state:   nil,
	})
	n.setFileList(list, nil)
	// 其实只需要激活一次就行。
	// 然后在退出最只有模拟器列表的时候切换激活。
	// n.scroll.Activate()
	return true
}

func (n *FileManagerWindow) setFileList(files []fs.DirEntry, state any) {
	n.emptyDir.SetProp(`display`, fmt.Sprint(len(files) == 0))

	type _FileBox struct {
		root fbiw.Box
		icon fbiw.Box   `css:".icon"`
		name *fbiw.Text `css:".name"`
	}

	n.scroll.SetItems(len(files),
		func() (fbiw.Box, *_FileBox) {
			item := fbiw.Unmarshal[_FileBox](n.doc, `
<block padding="0 20">
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
		func(box *_FileBox, index int) {
			file := files[index]
			box.name.SetText(file.Name())
			box.icon.SetProp(`display`, fbiw.Iif(file.IsDir(), `true`, `false`))
		},
	)

	if state != nil {
		n.scroll.SetState(state)
	}
}

// 基于文件信息，往上回溯父目录，拼出完整路径。
func (n *FileManagerWindow) finalPath(entry fs.DirEntry) string {
	parts := []string{}
	for i := 0; i < n.stack.Size(); i++ {
		parts = append(parts, n.stack.At(i).dir.Name())
	}
	parts = append(parts, entry.Name())
	return filepath.Join(parts...)
}

// 在当前目录枚举文件列表（含子目录名）。
//   - 不会递归进子目录
//   - 目前放前面
//   - 结果中不含 . 和 ..
func (n *FileManagerWindow) list(dir string) ([]fs.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	// 目录放前面。
	slices.SortFunc(entries, func(a, b fs.DirEntry) int {
		if a.IsDir() && !b.IsDir() {
			return -1
		}
		if !a.IsDir() && b.IsDir() {
			return +1
		}
		return strings.Compare(a.Name(), b.Name())
	})

	return entries, nil
}
