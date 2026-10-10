package video_player

import (
	_ "embed"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/movsb/fbiw"
)

//go:embed player
var playerBinary []byte

type VideoPlayer struct {
	fbiw.BaseBox

	src string

	prevRect fbiw.Rect

	videoWidth, videoHeight int

	cmd  *exec.Cmd
	path string
}

func NewVideoPlayer(doc *fbiw.Document) *VideoPlayer {
	return &VideoPlayer{
		BaseBox: fbiw.NewBaseBox(doc, `video`),
	}
}

func init() {
	fbiw.Define(`video`, true, NewVideoPlayer)
}

func (b *VideoPlayer) SetProp(key, val string) error {
	switch key {
	case `src`:
		b.src = val
		b.Stop()
		b.prevRect = fbiw.Rect{}
		b.videoWidth = 0
		b.videoHeight = 0
		b.Document().RequestLayout()
		return nil
	default:
		return b.Base().SetProp(key, val)
	}
}

func (b *VideoPlayer) SetPath(path string) {
	u := (&url.URL{Scheme: `os`, Opaque: url.PathEscape(path)}).String()
	b.SetProp(`src`, u)
}

func (b *VideoPlayer) Draw(canvas *fbiw.Canvas) {
	b.Base().Draw(canvas)

	rect := b.GetBoundingClientRect()
	if rect.Width <= 0 || rect.Height <= 0 {
		return
	}
	if rect == b.prevRect && b.cmd != nil {
		log.Println(`正在播放，不重绘`)
		return
	}
	if rect == b.prevRect {
		return
	}
	if b.cmd != nil {
		b.Stop()
	}
	b.prevRect = rect

	if b.videoWidth == 0 || b.videoHeight == 0 {
		b.probe()
	}

	if b.videoWidth == 0 {
		log.Println(`视频没有尺寸，无法播放`)
		return
	}

	b.Play()
}

func (b *VideoPlayer) probe() {
	if b.src == `` {
		return
	}

	if !strings.Contains(b.src, `:`) {
		return
	}
	u, err := url.Parse(b.src)
	if err != nil {
		log.Println(err)
		return
	}

	if u.Scheme != `os` {
		log.Panicf(`未知协议:%s`, u.Scheme)
	}

	path, err := url.PathUnescape(u.Opaque)
	if err != nil {
		log.Panicf(`url错误: %s`, err)
	}

	b.path = path

	// 直接同步计算了，反正会cache的。
	// 并且，播放器本身是可以拿到尺寸的，我只是难得通信了。
	cmd := exec.Command(`ffprobe`, `-v`, `error`, `-select_streams`, `v:0`, `-show_entries`, `stream=width,height`, `-of`, `csv=s=x:p=0`, path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Panicln(err)
	}

	var width, height int
	if _, err := fmt.Sscanf(string(output), `%dx%d`, &width, &height); err != nil {
		log.Println(err)
		return
	}

	log.Printf(`尺寸解析成功: %dx%d`, width, height)

	b.videoWidth = width
	b.videoHeight = height
}

func (b *VideoPlayer) Play() {
	if runtime.GOOS == `linux` {
		bin := filepath.Join(os.ExpandEnv(`$HOME/.local/bin`), `player`)
		if _, err := os.Stat(bin); os.IsNotExist(err) {
			dir, _ := filepath.Split(bin)
			if err := os.MkdirAll(dir, 0755); err != nil {
				log.Println(err)
				return
			}

			if err := os.WriteFile(bin, playerBinary, 0755); err != nil {
				log.Println(err)
				return
			}
		}

		fitting := containRect(b.prevRect, b.videoWidth, b.videoHeight)
		cmd := exec.Command(bin, b.path, fmt.Sprint(fitting.X), fmt.Sprint(fitting.Y), fmt.Sprint(fitting.Width), fmt.Sprint(fitting.Height))
		cmd.Stdin = nil
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			log.Println(err)
		} else {
			b.cmd = cmd
			log.Println(`已启动播放器进程`)
			go b.wait(cmd)
		}
	} else {
		cmd := exec.Command(`ffplay`, b.path)
		if err := cmd.Start(); err != nil {
			log.Println(err)
		} else {
			b.cmd = cmd
			go b.wait(cmd)
		}
	}
}

func containRect(container fbiw.Rect, contentWidth, contentHeight int) fbiw.Rect {
	if container.Width <= 0 || container.Height <= 0 || contentWidth <= 0 || contentHeight <= 0 {
		return fbiw.Rect{X: container.X, Y: container.Y}
	}
	scaleX := float64(container.Width) / float64(contentWidth)
	scaleY := float64(container.Height) / float64(contentHeight)
	scale := min(scaleX, scaleY)
	width := int(float64(contentWidth) * scale)
	height := int(float64(contentHeight) * scale)
	return fbiw.Rect{
		X:      container.X + (container.Width-width)/2,
		Y:      container.Y + (container.Height-height)/2,
		Width:  width,
		Height: height,
	}
}

func (b *VideoPlayer) wait(cmd *exec.Cmd) {
	cmd.Wait()
	log.Println(`播放器已退出`)
	b.Document().Async(func() {
		if b.cmd == cmd {
			b.Stop()
		}
	})
}

func (b *VideoPlayer) Stop() {
	if b.cmd != nil {
		// 不要用 Kill()，它发的是 Kill 信号，播放器来不及把屏幕恢复，
		// 会导致灰屏。
		b.cmd.Process.Signal(syscall.SIGTERM)
		b.cmd = nil
	}
}
