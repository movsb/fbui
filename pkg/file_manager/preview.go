package file_manager

import (
	"fmt"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbui/pkg/alert_window"
)

func (n *FileManagerWindow) preview(entry fs.DirEntry) {
	path := n.finalPath(entry)
	fp, err := os.Open(path)
	if err != nil {
		alert_window.Alert(n.app, fmt.Sprintf(`无法打开此文件: %v`, err), nil, nil)
		return
	}
	defer fp.Close()
	buf := make([]byte, 512)
	if count, err := fp.Read(buf); err != nil {
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
	default:
		alert_window.Alert(n.app, `暂时不支持查看此文件。`, nil, nil)
		return
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
		n.previewText.SetText(string(data))
		n.previewText.SetProp(`display`, `true`)
	}

	n.previewBox.SetProp(`display`, `true`)
	n.previewBox.Activate()
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
