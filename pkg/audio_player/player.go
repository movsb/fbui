package audio_player

import (
	_ "embed"
	"log"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"syscall"

	"github.com/movsb/fbiw"
)

type AudioPlayer struct {
	fbiw.BaseBox

	src string

	cmdFF *exec.Cmd
	cmdAP *exec.Cmd

	path string

	prevWidth, prevHeight int
}

func NewAudioPlayer(doc *fbiw.Document) *AudioPlayer {
	return &AudioPlayer{
		BaseBox: fbiw.NewBaseBox(doc, `audio`),
	}
}

func init() {
	fbiw.Define(`audio`, true, NewAudioPlayer)
}

func (b *AudioPlayer) SetProp(key, val string) error {
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

func (b *AudioPlayer) SetPath(path string) {
	u := (&url.URL{Scheme: `os`, Opaque: url.PathEscape(path)}).String()
	b.SetProp(`src`, u)
}

func (b *AudioPlayer) Calc(availWidth, availHeight int, constraints fbiw.Constraints) {
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
}

func (b *AudioPlayer) Draw(canvas *fbiw.Canvas) {
	if b.cmdFF != nil {
		return
	}

	b.Start()
}

func (b *AudioPlayer) Start() {
	if runtime.GOOS == `linux` {
		cmdFF := exec.Command(`ffmpeg`, `-loglevel`, `error`, `-i`, b.path, `-f`, `s16le`, `-ac`, `2`, `-ar`, `44100`, `-`)
		cmdAP := exec.Command(`aplay`, `-f`, `S16_LE`, `-c`, `2`, `-r`, `44100`)

		pipe, err := cmdFF.StdoutPipe()
		if err != nil {
			log.Println(err)
			return
		}

		cmdAP.Stdin = pipe

		if err := cmdAP.Start(); err != nil {
			log.Println(err)
			return
		}

		if err := cmdFF.Start(); err != nil {
			cmdAP.Process.Signal(syscall.SIGTERM)
			return
		}

		b.cmdFF = cmdFF
		b.cmdAP = cmdAP

		go func() {
			cmdFF.Wait()
			cmdAP.Wait()
		}()
	} else {
		cmd := exec.Command(`ffplay`, b.path)
		if err := cmd.Start(); err != nil {
			log.Println(err)
		} else {
			b.cmdFF = cmd
			go cmd.Wait()
		}

		b.cmdFF = cmd
	}
}

func (b *AudioPlayer) Stop() {
	if b.cmdFF != nil {
		// 不要用 Kill()，它发的是 Kill 信号，播放器来不及把屏幕恢复，
		// 会导致灰屏。
		b.cmdFF.Process.Signal(syscall.SIGTERM)
		b.cmdFF = nil
	}
	if b.cmdAP != nil {
		b.cmdAP.Process.Signal(syscall.SIGTERM)
		b.cmdAP = nil
	}
}
