package file_download

import (
	"embed"
	"errors"
	"fmt"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbui/pkg/alert_window"
	qrcode "github.com/skip2/go-qrcode"
)

//go:embed download.html
var _embed embed.FS

type _DownloadWindow struct {
	app *fbiw.App
	doc *fbiw.Document

	path    *fbiw.Text  `css:"#path"`
	qrCode  *fbiw.Image `css:"#qr-code"`
	address *fbiw.Text  `css:"#address"`
	status  *fbiw.Text  `css:"#status"`

	filePath string
	server   *http.Server
	conns    atomic.Int32
}

func New(app *fbiw.App, opener *fbiw.Document, filePath, ip string) {
	win := &_DownloadWindow{
		app:      app,
		doc:      app.NewPopup(_embed, `download.html`, opener),
		filePath: filePath,
	}
	win.doc.Bind(win)
	win.doc.Listen(fbiw.StickDownEvent, win.handleEvents)
	win.start(ip)
}

func (win *_DownloadWindow) start(ip string) {
	listener, err := net.Listen(`tcp4`, `0.0.0.0:0`)
	if err != nil {
		win.showError(err)
		return
	}
	port := listener.Addr().(*net.TCPAddr).Port
	address := fmt.Sprintf(`http://%s:%d/`, ip, port)

	win.path.SetTextFormat(`文件: %s`, filepath.Base(win.filePath))
	win.address.SetText(address)
	win.status.SetText(`等待下载…`)
	if qr, _ := qrcode.New(address, qrcode.Low); qr != nil {
		win.qrCode.SetImage(qr.Image(200))
	}

	mux := http.NewServeMux()
	mux.HandleFunc(`GET /`, win.handleDownload)
	win.server = &http.Server{Handler: mux}
	go func() {
		if err := win.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf(`下载服务器退出：%v`, err)
		}
	}()
}

func (win *_DownloadWindow) handleDownload(w http.ResponseWriter, r *http.Request) {
	win.conns.Add(+1)
	defer win.conns.Add(-1)

	file, err := os.Open(win.filePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		win.setStatus(fmt.Sprintf(`下载失败：%v`, err), true)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		win.setStatus(fmt.Sprintf(`下载失败：%v`, err), true)
		return
	}

	name := filepath.Base(win.filePath)
	w.Header().Set(`Content-Disposition`, mime.FormatMediaType(`attachment`, map[string]string{
		`filename`: name,
	}))
	win.setStatus(fmt.Sprintf(`正在下载“%s”…`, name), false)
	http.ServeContent(w, r, name, info.ModTime(), file)
	win.setStatus(fmt.Sprintf(`已完成“%s”`, name), false)
}

func (win *_DownloadWindow) setStatus(message string, isError bool) {
	win.app.Async(func() {
		win.status.SetText(message)
		win.status.ClassToggle(`warning`, isError)
	})
}

func (win *_DownloadWindow) showError(err error) {
	win.status.ClassAdd(`warning`)
	win.status.SetTextFormat(`启动失败：%v`, err)
}

func (win *_DownloadWindow) handleEvents(event *fbiw.Event) {
	if event.Stick.Name != fbiw.B {
		return
	}
	if win.conns.Load() > 0 {
		alert_window.Alert(win.doc, `当前有下载任务，确定要关闭吗？`, win.close, nil)
	} else {
		win.close()
	}
}

func (win *_DownloadWindow) close() {
	if win.server != nil {
		win.server.Close()
	}
	win.doc.Close()
}
