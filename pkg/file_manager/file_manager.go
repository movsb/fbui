package file_manager

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbui/pkg/alert_window"
	"github.com/movsb/fbui/pkg/audio_player"
	"github.com/movsb/fbui/pkg/file_manager/file_upload"
	"github.com/movsb/fbui/pkg/helpers"
	"github.com/movsb/fbui/pkg/menu_popup"
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

	// 文件操作剪贴板。move 为 true 时，粘贴会移动源项目。
	clipboard *_FileClipboard
}

type _FileClipboard struct {
	path string
	move bool
}

func New(app *fbiw.App) *FileManagerWindow {
	n := &FileManagerWindow{
		app: app,
		doc: app.NewDesktop(_embed, `file_manager.html`),
	}

	n.doc.Bind(n)
	n.root.Listen(fbiw.StickDownEvent, n.handleEvents)
	n.previewBox.Listen(fbiw.StickDownEvent, n.handlePreviewEvent)
	n.scroll.Listen(fbiw.ScrollSelectionChange, n.handleSelectionChangeEvent)

	if !n.initView() {
		return nil
	}

	n.activate()
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

	// 重新枚举目录后，用名称和可视行安全地恢复选中项。
	selectedName string
	rowIndex     int
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
			n.alert(`无法进入目录。`)
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

	if name == fbiw.Y {
		index := n.scroll.DataIndex()
		n.openFileMenu(index)
		event.StopPropagation()
		return
	}
}

func (n *FileManagerWindow) openFileMenu(index int) {
	items := []menu_popup.MenuItem{}
	if index >= 0 && index < len(n.stack.Top().entries) {
		entry := n.stack.Top().entries[index]
		entryPath := n.finalPath(entry.Name())

		if entry.IsDir {
			items = append(items, menu_popup.MenuItem{
				Name: `打开`,
				Click: func() {
					n.enterDirectory(entry)
				},
			})
		} else {
			items = append(items, menu_popup.MenuItem{
				Name: `预览`,
				Click: func() {
					n.preview(entry)
				},
			})
		}

		items = append(items,
			menu_popup.MenuItem{
				Name: `复制`,
				Click: func() {
					n.clipboard = &_FileClipboard{path: entryPath}
				},
			},
			menu_popup.MenuItem{
				Name: `移动`,
				Click: func() {
					n.clipboard = &_FileClipboard{path: entryPath, move: true}
				},
			},
			menu_popup.MenuItem{
				Name: `删除...`,
				Click: func() {
					n.confirmDelete(entryPath, entry.Name(), index)
				},
			},
		)
	}

	items = append(items, menu_popup.MenuItem{
		Name: `上传文件...`,
		Click: func() {
			if _, err := helpers.GetIP(); err != nil {
				n.alert("%s", err.Error())
				return
			}
			dir := n.finalPath(``)
			file_upload.New(n.app, dir)
		},
	})

	if n.clipboard != nil {
		items = append(items, menu_popup.MenuItem{Name: `粘贴`, Click: n.paste})
	}
	if len(items) > 0 {
		menu_popup.NewMenuPopup(n.app, n.doc, items, nil, nil)
	}
}

func (n *FileManagerWindow) confirmDelete(entryPath, name string, index int) {
	rowIndex := n.scroll.RowIndex()
	alert_window.Error(n.app, n.doc,
		fmt.Sprintf(`确定删除“%s”？此操作无法撤销。`, name),
		func() {
			// TODO 这里是同步的，可以导致界面死掉。
			if err := os.RemoveAll(entryPath); err != nil {
				n.alert(`删除失败：%v`, err)
				return
			}
			if n.clipboard != nil && n.clipboard.path == entryPath {
				n.clipboard = nil
			}
			if n.refreshCurrentDirectory() {
				n.selectAfterDelete(index, rowIndex)
			}
		},
		nil,
	)
}

func (n *FileManagerWindow) paste() {
	clipboard := n.clipboard
	if clipboard == nil {
		return
	}
	selectedName := ``
	selectedIndex := n.scroll.DataIndex()
	selectedRowIndex := n.scroll.RowIndex()
	if selectedIndex >= 0 && selectedIndex < len(n.stack.Top().entries) {
		selectedName = n.stack.Top().entries[selectedIndex].Name()
	}
	destination := filepath.Join(n.finalPath(``), filepath.Base(clipboard.path))
	var err error
	if clipboard.move {
		err = movePath(clipboard.path, destination)
	} else {
		err = copyPath(clipboard.path, destination)
	}
	if err != nil {
		n.alert(`粘贴失败：%v`, err)
		return
	}
	if clipboard.move {
		n.clipboard = nil
	}
	if n.refreshCurrentDirectory() {
		index := n.entryIndex(selectedName)
		if index < 0 {
			index = n.entryIndex(filepath.Base(destination))
		}
		n.selectIndex(index, selectedRowIndex)
	}
}

func (n *FileManagerWindow) refreshCurrentDirectory() bool {
	top := n.stack.Top()
	entries, err := n.list(n.finalPath(``))
	if err != nil {
		n.alert(`刷新目录失败：%v`, err)
		return false
	}
	top.entries = entries
	top.state = nil
	n.setFileList(entries, nil)
	return true
}

func (n *FileManagerWindow) selectAfterDelete(index, rowIndex int) {
	count := len(n.stack.Top().entries)
	if count == 0 {
		return
	}
	index = min(index, count-1)
	n.selectIndex(index, rowIndex)
}

func (n *FileManagerWindow) entryIndex(name string) int {
	if name == `` {
		return -1
	}
	return slices.IndexFunc(n.stack.Top().entries, func(entry _Entry) bool {
		return entry.Name() == name
	})
}

func (n *FileManagerWindow) selectIndex(index, rowIndex int) {
	if index < 0 || index >= len(n.stack.Top().entries) {
		return
	}
	rowIndex = min(max(rowIndex, 0), index)
	n.scroll.SetIndex(rowIndex, 0, index-rowIndex)
}

func (n *FileManagerWindow) leaveDirectory() {
	n.stack.Pop()
	top := n.stack.Top()
	if entries, err := n.list(n.finalPath(``)); err == nil {
		top.entries = entries
		n.setFileList(top.entries, nil)
		n.selectIndex(n.entryIndex(top.selectedName), top.rowIndex)
	} else {
		n.alert(`刷新目录失败：%v`, err)
		n.setFileList(top.entries, top.state)
	}
	n.setPath(n.finalPath(``))
	// n.clearStats()
}

func (n *FileManagerWindow) enterDirectory(entry _Entry) bool {
	path := n.finalPath(entry.Name())
	// TODO 这里是同步列举的，可能会卡界面
	list, err := n.list(path)
	if err != nil {
		n.alert("%v", err.Error())
		return false
	}
	if n.stack.Size() > 0 {
		top := n.stack.Top()
		top.state = n.scroll.GetState()
		top.rowIndex = n.scroll.RowIndex()
		index := n.scroll.DataIndex()
		if index >= 0 && index < len(top.entries) {
			top.selectedName = top.entries[index].Name()
		} else {
			top.selectedName = ``
		}
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

func movePath(source, destination string) error {
	if err := validateDestination(source, destination); err != nil {
		return err
	}
	if err := os.Rename(source, destination); err != nil {
		if !errors.Is(err, syscall.EXDEV) {
			return fmt.Errorf(`无法移动“%s”：%w`, filepath.Base(source), err)
		}
		if err := copyPath(source, destination); err != nil {
			return err
		}
		if err := os.RemoveAll(source); err != nil {
			return fmt.Errorf(`已复制项目，但无法删除源项目：%w`, err)
		}
	}
	return nil
}

func copyPath(source, destination string) error {
	if err := validateDestination(source, destination); err != nil {
		return err
	}
	info, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf(`无法读取源项目：%w`, err)
	}
	if err := copyPathWithInfo(source, destination, info); err != nil {
		// destination 在调用前已确认不存在，因此这里只清理由本次复制创建的内容。
		_ = os.RemoveAll(destination)
		return err
	}
	return nil
}

func validateDestination(source, destination string) error {
	source, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	destination, err = filepath.Abs(destination)
	if err != nil {
		return err
	}
	if source == destination {
		return fmt.Errorf(`源位置与目标位置相同`)
	}
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf(`目标位置已存在“%s”`, filepath.Base(destination))
	} else if !os.IsNotExist(err) {
		return fmt.Errorf(`无法检查目标位置：%w`, err)
	}
	if relative, err := filepath.Rel(source, destination); err == nil && relative != `..` && !strings.HasPrefix(relative, `..`+string(filepath.Separator)) {
		return fmt.Errorf(`不能将目录粘贴到其自身内部`)
	}
	return nil
}

func copyPathWithInfo(source, destination string, info fs.FileInfo) error {
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(source)
		if err != nil {
			return err
		}
		return os.Symlink(target, destination)
	case info.IsDir():
		if err := os.Mkdir(destination, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			childSource := filepath.Join(source, entry.Name())
			childInfo, err := os.Lstat(childSource)
			if err != nil {
				return err
			}
			if err := copyPathWithInfo(childSource, filepath.Join(destination, entry.Name()), childInfo); err != nil {
				return err
			}
		}
		return os.Chtimes(destination, info.ModTime(), info.ModTime())
	case info.Mode().IsRegular():
		return copyRegularFile(source, destination, info)
	default:
		return fmt.Errorf(`不支持复制此文件类型：“%s”`, filepath.Base(source))
	}
}

func copyRegularFile(source, destination string, info fs.FileInfo) (err error) {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := output.Close(); err == nil {
			err = closeErr
		}
	}()
	if _, err = io.Copy(output, input); err != nil {
		return err
	}
	return os.Chtimes(destination, info.ModTime(), info.ModTime())
}
