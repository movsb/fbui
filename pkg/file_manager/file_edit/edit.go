package file_edit

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbui/pkg/alert_window"
	qrcode "github.com/skip2/go-qrcode"
)

//go:embed edit.html
var _embed embed.FS

//go:embed index.*
var _webRoot embed.FS

const _maxFileSize = 1 << 20

type _EditWindow struct {
	app *fbiw.App
	doc *fbiw.Document

	path    *fbiw.Text  `css:"#path"`
	qrCode  *fbiw.Image `css:"#qr-code"`
	address *fbiw.Text  `css:"#address"`
	status  *fbiw.Text  `css:"#status"`

	filePath string
	server   *http.Server
}

func New(app *fbiw.App, filePath, ip string) {
	win := &_EditWindow{
		app:      app,
		doc:      app.NewDesktop(_embed, `edit.html`),
		filePath: filePath,
	}
	win.doc.Bind(win)
	win.doc.Listen(fbiw.StickDownEvent, win.handleEvents)
	win.start(ip)
}

func (win *_EditWindow) start(ip string) {
	listener, err := net.Listen(`tcp4`, `0.0.0.0:0`)
	if err != nil {
		win.showError(err)
		return
	}
	port := listener.Addr().(*net.TCPAddr).Port
	address := fmt.Sprintf(`http://%s:%d/`, ip, port)

	win.path.SetTextFormat(`文件: %s`, win.filePath)
	win.address.SetText(address)
	win.status.SetText(`等待编辑…`)
	if qr, _ := qrcode.New(address, qrcode.Low); qr != nil {
		win.qrCode.SetImage(qr.Image(400))
	}

	mux := http.NewServeMux()
	mux.HandleFunc(`GET /api/file`, win.handleRead)
	mux.HandleFunc(`PUT /api/file`, win.handleSave)
	mux.HandleFunc(`GET /`, func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, _webRoot, r.URL.Path)
	})
	win.server = &http.Server{Handler: mux}
	go func() {
		if err := win.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf(`编辑服务器退出：%v`, err)
		}
	}()
}

func (win *_EditWindow) handleRead(w http.ResponseWriter, r *http.Request) {
	file, info, err := win.openFile()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		win.setStatus(fmt.Sprintf(`读取失败：%v`, err), true)
		return
	}
	defer file.Close()
	w.Header().Set(`Content-Type`, `text/plain; charset=utf-8`)
	http.ServeContent(w, r, filepath.Base(win.filePath), info.ModTime(), file)
}

func (win *_EditWindow) handleSave(w http.ResponseWriter, r *http.Request) {
	if err := win.save(r.Body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		win.setStatus(fmt.Sprintf(`保存失败：%v`, err), true)
		return
	}
	w.WriteHeader(http.StatusNoContent)
	win.setStatus(`保存成功`, false)
	time.AfterFunc(time.Second, func() {
		win.app.Async(win.close)
	})
}

func (win *_EditWindow) save(input io.Reader) (err error) {
	info, err := os.Lstat(win.filePath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New(`只能编辑普通文件`)
	}
	if info.Size() > _maxFileSize {
		return errors.New(`文件大小不能超过 1 MiB`)
	}
	temporary, err := os.CreateTemp(filepath.Dir(win.filePath), `.fbui-edit-*`)
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			if closeErr := temporary.Close(); err == nil {
				err = closeErr
			}
		}
		if err != nil {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err = temporary.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	written, err := io.Copy(temporary, io.LimitReader(input, _maxFileSize+1))
	if err != nil {
		return err
	}
	if written > _maxFileSize {
		return errors.New(`文件大小不能超过 1 MiB`)
	}
	if err = temporary.Sync(); err != nil {
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	closed = true
	return os.Rename(temporaryPath, win.filePath)
}

func (win *_EditWindow) openFile() (*os.File, os.FileInfo, error) {
	info, err := os.Lstat(win.filePath)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, errors.New(`只能编辑普通文件`)
	}
	if info.Size() > _maxFileSize {
		return nil, nil, errors.New(`文件大小不能超过 1 MiB`)
	}
	file, err := os.Open(win.filePath)
	if err != nil {
		return nil, nil, err
	}
	return file, info, nil
}

func (win *_EditWindow) setStatus(message string, isError bool) {
	win.app.Async(func() {
		win.status.SetText(message)
		win.status.ClassToggle(`warning`, isError)
	})
}

func (win *_EditWindow) showError(err error) {
	win.status.ClassAdd(`warning`)
	win.status.SetTextFormat(`启动失败：%v`, err)
}

func (win *_EditWindow) handleEvents(event *fbiw.Event) {
	if event.Stick.Name == fbiw.B {
		alert_window.Alert(win.doc, `确定要关闭编辑窗口吗？`, win.close, nil)
	}
}

func (win *_EditWindow) close() {
	if win.server != nil {
		win.server.Close()
	}
	win.doc.Close()
}
