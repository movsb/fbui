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

	prevWidth, prevHeight int

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

func (b *VideoPlayer) Calc(availWidth, availHeight int, constraints fbiw.Constraints) {
	b.Base().Calc(availWidth, availHeight, constraints)

	if b.GetLayoutBox().Width == b.prevWidth && b.GetLayoutBox().Height == b.prevHeight {
		return
	}

	b.prevWidth = b.GetLayoutBox().Width
	b.prevHeight = b.GetLayoutBox().Height

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

func (b *VideoPlayer) Draw(canvas *fbiw.Canvas) {
	if b.cmd != nil {
		return
	}
	if b.videoWidth == 0 {
		return
	}

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

		// 缩放到scale-down大小。
		// 参考图片的缩放算法。
		fittingWidth, fittingHeight := 0, 0
		scaleX := float64(b.prevWidth) / float64(b.videoWidth)
		scaleY := float64(b.prevHeight) / float64(b.videoHeight)
		scale := min(scaleX, scaleY)
		if scale < 1 {
			fittingWidth = int(float64(b.videoWidth) * scale)
			fittingHeight = int(float64(b.videoHeight) * scale)
		} else {
			fittingWidth = b.videoWidth
			fittingHeight = b.videoHeight
		}

		cmd := exec.Command(bin, b.path, `0`, `0`, fmt.Sprint(fittingWidth), fmt.Sprint(fittingHeight))
		if err := cmd.Start(); err != nil {
			log.Println(err)
		} else {
			b.cmd = cmd
			go cmd.Wait()
		}
	} else {
		cmd := exec.Command(`ffplay`, b.path)
		if err := cmd.Start(); err != nil {
			log.Println(err)
		} else {
			b.cmd = cmd
			go cmd.Wait()
		}
	}
}

func (b *VideoPlayer) Stop() {
	if b.cmd != nil {
		// 不要用 Kill()，它发的是 Kill 信号，播放器来不及把屏幕恢复，
		// 会导致灰屏。
		b.cmd.Process.Signal(syscall.SIGTERM)
		b.cmd = nil
	}
}
