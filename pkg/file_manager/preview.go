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

func (n *FileManagerWindow) preview(entry _Entry) {
	path := n.finalPath(entry.Name())
	fp, err := os.Open(path)
	if err != nil {
		alert_window.Alert(n.app, fmt.Sprintf(`无法打开此文件: %v`, err), nil, nil)
		return
	}
	defer fp.Close()
	buf := make([]byte, 512)
	if count, err := fp.Read(buf); err != nil {
		if errors.Is(err, io.EOF) {
			alert_window.Alert(n.app, `空文件（0字节大小）`, nil, nil)
			return
		}
		alert_window.Alert(n.app, fmt.Sprintf(`无法读取此文件: %v`, err), nil, nil)
		return
	} else {
		buf = buf[:count]
		if _, err := fp.Seek(0, io.SeekStart); err != nil {
			alert_window.Alert(n.app, fmt.Sprintf(`无法重定位此文件: %v`, err), nil, nil)
			return
		}
	}

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
			alert_window.Alert(n.app, `暂时不支持查看此文件。`, nil, nil)
			return
		}
	}

	n.previewBox.SetProp(`display`, `true`)
	n.previewBox.Activate()
}

func (n *FileManagerWindow) previewTextContent(fp *os.File, preprocess func(data []byte)) {
	info, err := fp.Stat()
	if err != nil {
		alert_window.Alert(n.app, fmt.Sprintf(`无法获取基本信息: %v`, err), nil, nil)
		return
	}
	if info.Size() > 10<<20 {
		alert_window.Alert(n.app, `文件太大，暂时不支持查看。`, nil, nil)
		return
	}
	data, err := io.ReadAll(fp)
	if err != nil {
		alert_window.Alert(n.app, fmt.Sprintf(`读取文件内容失败: %v`, err), nil, nil)
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
