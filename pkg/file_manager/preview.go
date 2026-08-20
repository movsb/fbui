package file_manager

import (
	"fmt"
	_ "image/jpeg"
	_ "image/png"
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
	}

	ct := http.DetectContentType(buf)

	n.previewImage.SetProp(`display`, `false`)
	n.previewVideo.SetProp(`display`, `false`)

	switch {
	default:
		alert_window.Alert(n.app, `二进制内容文件暂时不支持查看。`, nil, nil)
		return
	case strings.HasPrefix(ct, `image/`):
		n.previewImage.SetPath(path)
		n.previewImage.SetProp(`display`, `true`)
	case strings.HasPrefix(ct, `video/`):
		n.previewVideo.SetPath(path)
		n.previewVideo.SetProp(`display`, `true`)
	}

	n.previewBox.SetProp(`display`, `true`)
	n.previewBox.Activate()
}

func (n *FileManagerWindow) handlePreviewEvent(e *fbiw.Event) {
	if e.Stick.Name == fbiw.B {
		n.activate()
		n.previewBox.SetProp(`display`, `false`)
		n.previewVideo.Stop()
		e.StopPropagation()
		return
	}
}
