package file_upload

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
	"strings"
	"sync"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbui/pkg/helpers"
	qrcode "github.com/skip2/go-qrcode"
)

//go:embed upload.html
var _embed embed.FS

//go:embed index.*
var _webRoot embed.FS

type _UploadWindow struct {
	app *fbiw.App
	doc *fbiw.Document

	qrCode  *fbiw.Image `css:"#qr-code"`
	address *fbiw.Text  `css:"#address"`
	status  *fbiw.Text  `css:"#status"`

	dir       string
	server    *http.Server
	listener  net.Listener
	onClose   func()
	closeOnce sync.Once
}

func New(app *fbiw.App, dir string, onClose func()) {
	win := &_UploadWindow{
		app:     app,
		doc:     app.NewDesktop(_embed, `upload.html`),
		dir:     dir,
		onClose: onClose,
	}
	win.doc.Bind(win)
	win.doc.Listen(fbiw.StickDownEvent, win.handleEvents)
	win.start()
}

func (win *_UploadWindow) start() {
	ip, err := helpers.GetIP()
	if err != nil {
		win.showError(err)
		return
	}
	listener, err := net.Listen(`tcp4`, `0.0.0.0:0`)
	if err != nil {
		win.showError(err)
		return
	}
	win.listener = listener
	port := listener.Addr().(*net.TCPAddr).Port
	address := fmt.Sprintf(`http://%s:%d/`, ip, port)

	win.address.SetText(address)
	win.status.SetText(`等待上传…`)

	if qr, _ := qrcode.New(address, qrcode.Low); qr != nil {
		win.qrCode.SetImage(qr.Image(400))
	}

	mux := http.NewServeMux()
	mux.HandleFunc(`GET /`, func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, _webRoot, r.URL.Path)
	})
	mux.HandleFunc(`POST /`, win.handleUpload)
	win.server = &http.Server{Handler: mux}
	go func() {
		if err := win.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf(`上传服务器退出：%v`, err)
		}
	}()
}

func (win *_UploadWindow) handleUpload(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile(`file`)
	if err != nil {
		http.Error(w, `没有选择文件`, http.StatusBadRequest)
		win.setStatus(`上传失败：没有选择文件`, true)
		return
	}
	defer file.Close()
	if err := win.receiveFile(file, header.Filename); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		win.setStatus(fmt.Sprintf(`上传失败：%v`, err), true)
		return
	}

	w.WriteHeader(http.StatusNoContent)
	win.setStatus(fmt.Sprintf(`已上传“%s”`, header.Filename), false)
}

func (win *_UploadWindow) receiveFile(input io.Reader, fileName string) (err error) {
	name := filepath.Base(strings.ReplaceAll(fileName, `\`, `/`))
	if name == `` || name == `.` {
		return errors.New(`无效的文件名`)
	}
	destination := filepath.Join(win.dir, name)
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf(`文件“%s”已存在`, name)
		}
		return err
	}
	defer func() {
		if closeErr := output.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(destination)
		}
	}()
	_, err = io.Copy(output, input)
	return err
}

func (win *_UploadWindow) setStatus(message string, isError bool) {
	win.app.Async(func() {
		win.status.SetText(message)
		win.status.ClassToggle(`warning`, isError)
	})
}

func (win *_UploadWindow) showError(err error) {
	win.status.ClassAdd(`warning`)
	win.status.SetTextFormat(`启动失败：%v`, err)
}

func (win *_UploadWindow) handleEvents(event *fbiw.Event) {
	if event.Stick.Name != fbiw.B {
		return
	}
	win.close()
	win.doc.Close()
}

func (win *_UploadWindow) close() {
	win.closeOnce.Do(func() {
		if win.server != nil {
			win.server.Close()
		} else if win.listener != nil {
			win.listener.Close()
		}
		if win.onClose != nil {
			win.onClose()
		}
	})
}
