package file_manager

import (
	"bytes"
	"errors"
	"fmt"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbui/pkg/alert_window"
)

func (n *FileManagerWindow) alert(format string, args ...any) {
	alert_window.Alert(n.app, n.doc, fmt.Sprintf(format, args...), nil, nil)
}

func (n *FileManagerWindow) preview(entry _Entry) {
	path := n.finalPath(entry.Name())
	fp, err := os.Open(path)
	if err != nil {
		n.alert(`无法打开此文件: %v`, err)
		return
	}
	defer fp.Close()

	buf := make([]byte, 512)
	// 不小心读别人的stdin……然后假死了😅
	fp.SetReadDeadline(time.Now().Add(time.Second))
	if count, err := fp.Read(buf); err != nil {
		if errors.Is(err, io.EOF) {
			n.alert(`空文件（0字节大小）`)
			return
		}
		n.alert(`无法读取此文件: %v`, err)
		return
	} else {
		buf = buf[:count]
		if _, err := fp.Seek(0, io.SeekStart); err != nil {
			n.alert(`无法重定位此文件: %v`, err)
			return
		}
	}
	fp.SetReadDeadline(time.Time{})

	ct := http.DetectContentType(buf)

	n.previewImage.SetProp(`display`, `false`)
	n.previewVideo.SetProp(`display`, `false`)
	n.previewAudio.SetProp(`display`, `false`)
	n.previewText.SetProp(`display`, `false`)

	switch {
	case strings.HasPrefix(ct, `image/`):
		n.previewImage.SetPath(path)
		n.previewImage.SetProp(`display`, `true`)
	case strings.HasPrefix(ct, `video/`):
		n.previewVideo.SetPath(path)
		n.previewVideo.SetProp(`display`, `true`)
	case strings.HasPrefix(ct, `audio/`):
		n.previewAudio.SetPath(path)
		n.previewAudio.SetProp(`display`, `true`)
	case strings.HasPrefix(ct, `text/`):
		n.previewTextContent(fp, nil)
	default:
		// 尽量猜一下先再放弃？

		viewed := false

		// 类似 /proc/1/{cmdline,environ} 这种不含除 0x00 非 printable 字符。
		if bytes.IndexByte(buf, 0) != -1 {
			extra := false
			for i := range len(buf) {
				if buf[i] > 0 && buf[i] < 0x20 {
					extra = true
					break
				}
			}
			if !extra {
				n.previewTextContent(fp, func(data []byte) {
					for i := range len(data) {
						if data[i] == 0 {
							data[i] = '\n'
						}
					}
				})
				viewed = true
			}
		}

		if !viewed {
			n.alert(`暂时不支持查看此文件。`)
			return
		}
	}

	n.previewBox.SetProp(`display`, `true`)
	n.previewBox.Activate()
}

func (n *FileManagerWindow) previewTextContent(fp *os.File, preprocess func(data []byte)) {
	info, err := fp.Stat()
	if err != nil {
		n.alert(`无法获取基本信息: %v`, err)
		return
	}
	if info.Size() > 10<<20 {
		n.alert(`文件太大，暂时不支持查看。`)
		return
	}
	fp.SetReadDeadline(time.Now().Add(time.Second * 5))
	data, err := io.ReadAll(fp)
	if err != nil {
		n.alert(`读取文件内容失败: %v`, err)
		return
	}
	// skip bom
	if len(data) > 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}

	if preprocess != nil {
		preprocess(data)
	}

	n.previewText.SetText(string(data))
	n.previewText.SetProp(`display`, `true`)
}

func (n *FileManagerWindow) handlePreviewEvent(e *fbiw.Event) {
	defer e.StopPropagation()
	if e.Stick.Name == fbiw.B {
		n.activate()
		n.previewBox.SetProp(`display`, `false`)
		n.previewVideo.Stop()
		n.previewAudio.Stop()
		n.previewText.SetText(``)
		return
	}
	if d := n.previewText.GetComputedStyles().Display; d.IsBool() && d.Bool {
		switch e.Stick.Name {
		case fbiw.Up:
			n.previewText.ScrollLineUp()
		case fbiw.Down:
			n.previewText.ScrollLineDown()
		case fbiw.Left:
			n.previewText.PageLeft()
		case fbiw.Right:
			n.previewText.PageRight()
		}
	}
}

func (n *FileManagerWindow) handleSelectionChangeEvent(e *fbiw.Event) {
	top := n.stack.Top()
	index := n.scroll.DataIndex()
	if index == -1 {
		return
	}

	entry := top.entries[index]
	info, err := entry.Info()
	if err != nil {
		n.statPerm.SetText(err.Error())
		return
	}

	n.statPerm.SetText(info.Mode().String())
	if !info.IsDir() {
		n.statSize.SetText(formatBytes(info.Size()))
	} else {
		n.statSize.SetText(``)
	}
	n.statModTime.SetText(info.ModTime().Format(time.DateTime))

	n.textPagination.SetTextFormat(`%d/%d`, index+1, n.scroll.DataCount())
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}

	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f%cB", float64(n)/float64(div), "KMGTPE"[exp])
}
