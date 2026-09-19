package progress_popup

import (
	"embed"
	"fmt"

	"github.com/movsb/fbiw"
)

//go:embed progress_popup.html
var assets embed.FS

type Popup struct {
	doc      *fbiw.Document
	message  *fbiw.Text        `css:"#message"`
	detail   *fbiw.Text        `css:"#detail"`
	progress *fbiw.ProgressBar `css:"progress"`
}

func New(app *fbiw.App, opener *fbiw.Document, message string) *Popup {
	p := &Popup{doc: app.NewPopup(assets, "progress_popup.html", opener)}
	p.doc.Bind(p)
	p.message.SetText(message)
	p.detail.SetText("")
	p.progress.SetIndeterminate(true)
	p.doc.Listen(fbiw.InputDownEvent, func(event *fbiw.Event) { event.StopPropagation() })
	return p
}

func (p *Popup) SetIndeterminate(message string) {
	p.message.SetText(message)
	p.detail.SetText("")
	p.progress.SetIndeterminate(true)
}

func (p *Popup) SetProgress(received, total int64) {
	p.progress.SetIndeterminate(false)
	value := float64(received) / float64(total)
	if value > 1 {
		value = 1
	}
	_ = p.progress.SetValue(value)
	p.message.SetText("正在下载数据库")
	p.detail.SetText(fmt.Sprintf("%.0f%%  %s / %s", value*100, formatSize(received), formatSize(total)))
}

func (p *Popup) Close() { p.doc.Close() }

func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit && exp < 3; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGT"[exp])
}
