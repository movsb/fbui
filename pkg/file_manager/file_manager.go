package file_manager

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
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

	doc         *fbiw.Document
	root        fbiw.Box
	emptyDir    fbiw.Box     `css:"#empty"`
	scroll      *fbiw.Scroll `css:"#scroll"`
	path        *fbiw.Text   `css:"#path"`
	statSize    *fbiw.Text   `css:"#stat .size"`
	statPerm    *fbiw.Text   `css:"#stat .perm"`
	statModTime *fbiw.Text   `css:"#stat .time"`

	textPagination *fbiw.Text `css:"#pagination"`

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
	n.root.Listen(fbiw.StickDownEvent, n.handleEvents)
	n.previewBox.Listen(fbiw.StickDownEvent, n.handlePreviewEvent)
	n.scroll.Listen(fbiw.ScrollSelectionChange, n.handleSelectionChangeEvent)

	if !n.initView() {
		return nil
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
	dir _Entry

	// 其下的子项。
	entries []_Entry

	// 滚动条状态。
	state any
}

func (n *FileManagerWindow) initView() bool {
	dir := fbiw.Iif(runtime.GOOS == `linux`, `/mnt/SDCARD`, os.ExpandEnv(`$HOME/Downloads`))
	components := []string{}
	for component := range strings.SplitSeq(path.Clean(dir), `/`) {
		if component == `` {
			component = `/`
		}
		components = append(components, component)
		info := fbiw.Must1(os.Stat(filepath.Join(components...)))
		entry := _Entry{
			DirEntry: fs.FileInfoToDirEntry(info),
			IsDir:    true,
		}
		if !n.enterDirectory(entry) {
			alert_window.Alert(n.app, `无法进入目录。`, nil, nil)
			n.doc.Close()
			return false
		}
	}
	return true
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
			n.leaveDirectory()
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
		if entry.IsDir {
			n.enterDirectory(entry)
		} else {
			n.preview(entry)
		}
		event.StopPropagation()
		return
	}
}

func (n *FileManagerWindow) leaveDirectory() {
	n.stack.Pop()
	top := n.stack.Top()
	n.setFileList(top.entries, top.state)
	n.setPath(n.finalPath(``))
	// n.clearStats()
}

func (n *FileManagerWindow) enterDirectory(entry _Entry) bool {
	path := n.finalPath(entry.Name())
	// TODO 这里是同步列举的，可能会卡界面
	list, err := n.list(path)
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
	n.setPath(path)
	// n.clearStats()
	return true
}

func (n *FileManagerWindow) clearStats() {
	for _, t := range []*fbiw.Text{
		n.statModTime,
		n.statPerm,
		n.statSize,
	} {
		t.SetText(``)
	}
}

func (n *FileManagerWindow) setPath(path string) {
	if path != `/` {
		path += `/`
	}
	n.path.SetText(path)
}

func (n *FileManagerWindow) setFileList(files []_Entry, state any) {
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
			box.icon.SetProp(`display`, fbiw.Iif(file.IsDir, `true`, `false`))
		},
	)

	if state != nil {
		n.scroll.SetState(state)
	}
}

// 基于文件信息，往上回溯父目录，拼出完整路径。
//
// 如果参数为空，只拼目录路径。
func (n *FileManagerWindow) finalPath(currentName string) string {
	parts := []string{}
	for i := 0; i < n.stack.Size(); i++ {
		parts = append(parts, n.stack.At(i).dir.Name())
	}
	if currentName != `` {
		parts = append(parts, currentName)
	}
	return filepath.Join(parts...)
}

// 在当前目录枚举文件列表（含子目录名）。
//   - 不会递归进子目录
//   - 目前放前面
//   - 结果中不含 . 和 ..
func (n *FileManagerWindow) list(dir string) ([]_Entry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	isDir := func(entry fs.DirEntry) bool {
		if entry.IsDir() {
			return true
		}
		if entry.Type()&fs.ModeSymlink > 0 {
			info, err := os.Stat(filepath.Join(dir, entry.Name()))
			if err != nil {
				log.Println(err)
				return false
			}
			if info.IsDir() {
				return true
			}
		}
		return false
	}

	out := make([]_Entry, 0, len(entries))
	for entry := range slices.Values(entries) {
		out = append(out, _Entry{
			DirEntry: entry,
			IsDir:    isDir(entry),
		})
	}

	// 目录放前面。
	slices.SortFunc(out, func(a, b _Entry) int {
		if a.IsDir && !b.IsDir {
			return -1
		}
		if !a.IsDir && b.IsDir {
			return +1
		}
		return strings.Compare(a.Name(), b.Name())
	})

	return out, nil
}

type _Entry struct {
	fs.DirEntry
	IsDir bool
}
