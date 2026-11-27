//go:build linux

package osdd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fswatcher/fswatcher"
	"github.com/movsb/fbui/pkg/disp2"
)

type Options struct {
	Root      string
	InputPath string
	FontPath  string
	Logger    *log.Logger
}

type Daemon struct {
	options  Options
	statuses [6]string
	renderer *Renderer
	overlay  *disp2.Overlay
	input    *os.File
	closeKey uint16
	visible  bool
	// 关闭画面后仍独占输入，直到关闭键松开。
	closingWithB bool
	toast        *Toast

	// toastSignature 用修改时间和大小识别同一内容的重复通知。不能只比较 JSON，
	// 因为连续两次相同音量也应当重新开始计时。
	toastSignature string
}

func New(options Options) (*Daemon, error) {
	if options.Root == "" {
		options.Root = "/usr/trimui/osd"
	}
	if options.InputPath == "" {
		options.InputPath = "/dev/input/event3"
	}
	if options.FontPath == "" {
		options.FontPath = filepath.Join(options.Root, "regular.ttf")
	}
	if options.Logger == nil {
		options.Logger = log.Default()
	}
	return &Daemon{options: options}, nil
}

func (d *Daemon) logf(format string, args ...any) { d.options.Logger.Printf(format, args...) }

func readTrimmed(path string) string {
	if path == "" {
		return ""
	}
	data, _ := os.ReadFile(path)
	return strings.TrimSpace(string(data))
}

func (d *Daemon) refresh() {
	d.statuses[0] = time.Now().Format("15:04")
	d.statuses[1] = "—"
	if charge := readTrimmed("/sys/class/power_supply/axp2202-battery/capacity"); charge != "" {
		d.statuses[1] = charge + "%"
	}
	d.statuses[2] = "—"
	if khz, err := strconv.Atoi(readTrimmed("/sys/devices/system/cpu/cpufreq/policy0/scaling_cur_freq")); err == nil && khz > 0 {
		d.statuses[2] = fmt.Sprintf("%d MHz", khz/1000)
	}
	d.statuses[3] = "—"
	online := 0
	for _, line := range strings.Split(readTrimmed("/proc/stat"), "\n") {
		name, _, _ := strings.Cut(line, " ")
		if id, ok := strings.CutPrefix(name, "cpu"); ok {
			if _, err := strconv.Atoi(id); err == nil {
				online++
			}
		}
	}
	if online > 0 {
		d.statuses[3] = strconv.Itoa(online)
	}
	d.statuses[4] = "—"
	if temp, err := strconv.Atoi(readTrimmed("/sys/class/power_supply/axp2202-battery/temp")); err == nil {
		d.statuses[4] = fmt.Sprintf("%.1f °C", float64(temp)/10)
	}
	d.statuses[5] = memoryUsage()
}

func memoryUsage() string {
	total, available := 0, -1
	for _, line := range strings.Split(readTrimmed("/proc/meminfo"), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total, _ = strconv.Atoi(fields[1])
		case "MemAvailable:":
			if value, err := strconv.Atoi(fields[1]); err == nil {
				available = value
			}
		}
	}
	if total <= 0 || available < 0 || available > total {
		return "—"
	}
	return fmt.Sprintf("%d / %d MiB", (total-available)/1024, total/1024)
}

func (d *Daemon) redraw() error {
	if !d.visible && d.toast == nil {
		return nil
	}
	return d.renderer.draw(d.overlay, d.visible, d.statuses, d.toast)
}

func (d *Daemon) ensureOverlay() error {
	if d.overlay == nil {
		// 使用原厂 OSD 的 channel、显存槽位、crop、像素格式和双层提交方式；
		// 这与仅在原厂 OSD 旁边增加一个额外覆盖层的配置不同。
		options := disp2.TrimUIOSDReplacementOptions()
		// fbiw 的混色会把已绘制像素的 alpha 写满。混合模式让 DISP2
		// 对可见像素应用统一透明度，同时保留面板之外的透明区域。
		options.Opacity = 224
		overlay, err := disp2.OpenOverlay(options)
		if err != nil {
			return err
		}
		d.overlay = overlay
		renderer, err := newRenderer(d.options.FontPath)
		if err != nil {
			_ = overlay.Close()
			d.overlay = nil
			return err
		}
		d.renderer = renderer
	}
	return nil
}

func (d *Daemon) show() (err error) {
	if d.visible {
		return nil
	}
	if err := d.ensureOverlay(); err != nil {
		return err
	}
	if d.input == nil {
		return fmt.Errorf("OSD 输入设备未打开")
	}
	if err := grabInput(d.input, 1); err != nil {
		return fmt.Errorf("独占 OSD 输入: %w", err)
	}
	d.visible = true
	defer func() {
		if err != nil {
			_ = d.hide()
		}
	}()
	d.closingWithB = false
	d.refresh()
	if err := os.MkdirAll("/tmp/trimui_osd", 0755); err != nil {
		return err
	}
	if err := os.MkdirAll("/tmp/trimui_inputd", 0755); err != nil {
		return err
	}
	if err := os.WriteFile("/tmp/trimui_inputd/grab", []byte("1\n"), 0644); err != nil {
		return err
	}
	if err := os.WriteFile("/tmp/trimui_osd/osdd_show_up", nil, 0644); err != nil {
		return err
	}
	return d.redraw()
}

func (d *Daemon) hide() error {
	if !d.visible {
		if d.closingWithB {
			d.finishBRelease()
		}
		return nil
	}
	d.visible = false
	_ = grabInput(d.input, 0)
	os.Remove("/tmp/trimui_inputd/grab")
	os.Remove("/tmp/trimui_osd/osdd_show_up")
	if d.toast != nil {
		return d.redraw()
	}
	if d.overlay != nil {
		return d.overlay.Hide()
	}
	return nil
}

// hideOnBPress 先隐藏画面，但把输入拦截保留到 B 松开。许多界面在按键松开
// 时执行动作；如果这里立刻删除 grab，关闭 OSD 的这次 B 也会触发 MainUI。
func (d *Daemon) hideOnBPress() error {
	if !d.visible {
		return nil
	}
	d.visible = false
	d.closingWithB = true
	if d.toast != nil {
		return d.redraw()
	}
	if d.overlay != nil {
		return d.overlay.Hide()
	}
	return nil
}

func (d *Daemon) finishBRelease() {
	_ = grabInput(d.input, 0)
	d.closingWithB = false
	os.Remove("/tmp/trimui_inputd/grab")
	os.Remove("/tmp/trimui_osd/osdd_show_up")
}

// checkToast 检查原厂通知文件。原脚本通过覆盖同一个 JSON 文件发送事件，没有
// socket 或 FIFO，所以必须同时观察 mtime 和 size 才能识别重复消息。
func (d *Daemon) checkToast(now time.Time) {
	const path = "/tmp/trimui_osd/osd_toast_msg"
	info, err := os.Stat(path)
	if err == nil {
		signature := fmt.Sprintf("%d/%d", info.ModTime().UnixNano(), info.Size())
		if signature != d.toastSignature {
			d.toastSignature = signature
			data, readErr := os.ReadFile(path)
			var toast Toast
			if readErr == nil {
				readErr = json.Unmarshal(data, &toast)
			}
			if readErr != nil {
				d.logf("解析 OSD 通知失败: %v", readErr)
			} else if err := d.ensureOverlay(); err != nil {
				d.logf("显示 OSD 通知失败: %v", err)
			} else {
				if toast.Duration <= 0 {
					toast.Duration = 1000
				}
				toast.Expires = now.Add(time.Duration(toast.Duration) * time.Millisecond)
				d.toast = &toast
				if err := d.redraw(); err != nil {
					d.logf("绘制 OSD 通知失败: %v", err)
				}

			}
		}
	}
}

func (d *Daemon) handleInput(event InputEvent) {
	if event.Type == EventKey && (event.Code == KeyB || event.Code == KeyHome) {
		if event.Value == 0 && d.closingWithB && event.Code == d.closeKey {
			d.finishBRelease()
			return
		}
		if event.Value == 1 && d.visible {
			d.closeKey = event.Code
			_ = d.hideOnBPress()
			return
		}
		if event.Value == 1 && event.Code == KeyHome && !d.closingWithB {
			if err := d.show(); err != nil {
				d.logf("显示 OSD 失败: %v", err)
			}
		}
		return
	}
}

func consumeCommand(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	os.Remove(path)
	return true
}

// resolveInputPath 优先采用命令行指定的设备；如果它尚未出现或编号发生变化，
// 就从 sysfs 中按名字寻找 trimui_inputd 创建的 “TRIMUI Player1”。启动早期
// event3 通常还不存在，而且设备重建后也不保证继续使用同一个 event 编号。
func resolveInputPath(preferred string) string {
	if _, err := os.Stat(preferred); err == nil {
		namePath := filepath.Join("/sys/class/input", filepath.Base(preferred), "device/name")
		name := strings.TrimSpace(readTrimmed(namePath))
		// 非 /dev/input/eventN 的自定义路径没有对应 sysfs 名称，直接尊重调用者；
		// eventN 则必须确认没有被其他输入设备占用。
		if name == "" || name == "TRIMUI Player1" {
			return preferred
		}
	}
	devices, _ := filepath.Glob("/sys/class/input/event*/device/name")
	for _, namePath := range devices {
		name := strings.TrimSpace(readTrimmed(namePath))
		if name != "TRIMUI Player1" {
			continue
		}
		eventName := filepath.Base(filepath.Dir(filepath.Dir(namePath)))
		return filepath.Join("/dev/input", eventName)
	}
	return preferred
}

func (d *Daemon) Close() error {
	_ = d.hide()
	if d.renderer != nil {
		d.renderer.close()
		d.renderer = nil
	}
	if d.overlay == nil {
		return nil
	}
	err := d.overlay.Close()
	d.overlay = nil
	return err
}

// Start 在 fbui 进程内启动 OSD，并返回等待其清理完成的停止函数。
func Start(parent context.Context) func() {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		root := "/usr/trimui/osd"
		for _, candidate := range []string{"/mnt/SDCARD/trimui/osd", "/mnt/UDISK/trimui/osd"} {
			if _, err := os.Stat(filepath.Join(candidate, "regular.ttf")); err == nil {
				root = candidate
				break
			}
		}
		d, _ := New(Options{Root: root})
		lock, err := os.OpenFile("/tmp/fbui-osdd.lock", os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			d.logf("打开 OSD 锁失败: %v", err)
			return
		}
		defer lock.Close()
		if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			d.logf("获取 OSD 锁失败: %v", err)
			return
		}
		stock, err := stopStockOSD(ctx)
		if err != nil {
			d.logf("接管原厂 OSD 失败: %v", err)
			return
		}
		defer func() {
			if err := stock.restore(); err != nil {
				d.logf("恢复原厂 OSD 失败: %v", err)
			}
		}()
		if err := d.Run(ctx); err != nil && ctx.Err() == nil {
			d.logf("OSD 停止: %v", err)
		}
	}()
	return func() { cancel(); <-done }
}

func (d *Daemon) Run(ctx context.Context) error {
	if err := os.MkdirAll("/tmp/trimui_osd", 0755); err != nil {
		return err
	}
	watcher, err := fswatcher.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()
	if err := watcher.Add("/tmp", fswatcher.Create|fswatcher.Write|fswatcher.Rename); err != nil {
		return err
	}
	if err := watcher.AddRecursive("/tmp/trimui_osd", fswatcher.Create|fswatcher.Write|fswatcher.Rename); err != nil {
		return err
	}
	input, err := os.OpenFile(resolveInputPath(d.options.InputPath), os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return fmt.Errorf("打开 OSD 输入: %w", err)
	}
	d.input = input
	defer input.Close()
	readCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer d.Close()
	if err := d.ensureOverlay(); err != nil {
		return err
	}
	_ = d.overlay.Hide()
	if _, err := os.Stat("/tmp/trimui_osd/osdd_show_up"); err == nil {
		d.finishBRelease()
	}
	if info, err := os.Stat("/tmp/trimui_osd/osd_toast_msg"); err == nil {
		d.toastSignature = fmt.Sprintf("%d/%d", info.ModTime().UnixNano(), info.Size())
	}
	events := make(chan InputEvent, 16)
	readErr := make(chan error, 1)
	go func() {
		readErr <- readInput(input, func(event InputEvent) {
			if event.Type != EventKey || (event.Code != KeyB && event.Code != KeyHome) {
				return
			}
			select {
			case events <- event:
			case <-readCtx.Done():
			}
		})
	}()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var musicPipe musicKeepalive
	defer musicPipe.close()
	musicPipe.refresh()
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	timer.Stop()
	var expiry <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-readErr:
			return err
		case event := <-events:
			d.handleInput(event)
		case err, ok := <-watcher.Errors:
			if !ok {
				return fmt.Errorf("OSD 文件观察器已关闭")
			}
			if err != nil {
				d.logf("OSD 文件观察器错误: %v", err)
			}
		case event, ok := <-watcher.Events:
			if !ok {
				return fmt.Errorf("OSD 文件观察器已关闭")
			}
			switch filepath.Clean(event.Name) {
			case "/tmp/show_osdd":
				if consumeCommand(event.Name) {
					if err := d.show(); err != nil {
						d.logf("显示 OSD 失败: %v", err)
					}
				}
			case "/tmp/hide_osdd":
				if consumeCommand(event.Name) {
					_ = d.hide()
				}
			case "/tmp/trimui_osd/osd_toast_msg":
				d.checkToast(time.Now())
				if d.toast != nil {
					timer.Reset(time.Until(d.toast.Expires))
					expiry = timer.C
				}
			}
		case <-expiry:
			expiry = nil
			d.toast = nil
			if d.visible {
				_ = d.redraw()
			} else {
				_ = d.overlay.Hide()
			}
		case <-ticker.C:
			musicPipe.refresh()
			if d.visible {
				d.refresh()
				_ = d.redraw()
			}
		}
	}
}
