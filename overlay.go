package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/fswatcher/fswatcher"
	"github.com/mdlayher/kobject"
	"github.com/movsb/fbiw"
)

type OverlayWindow struct {
	app *fbiw.App
	doc *fbiw.Document

	theTitle *fbiw.Text `css:"#title"`

	txtTime              *fbiw.Text `css:"#time"`
	txtBatteryPercentage *fbiw.Text `css:"#battery"`
	boxBatteryCharging   fbiw.Box   `css:"#battery-charging-box"`

	txtVolumeOpen    *fbiw.Text `css:"#speaker"`
	txtWifiOpen      *fbiw.Text `css:"#wifi"`
	txtBluetoothOpen *fbiw.Text `css:"#bluetooth"`
	txtCpuStatus     *fbiw.Text `css:"#cpu"`
}

func NewOverlayWindow(app *fbiw.App) *OverlayWindow {
	doc := app.NewOverlay(_embed, `overlay.html`)

	win := &OverlayWindow{
		app: app,
		doc: doc,
	}

	doc.Bind(win)
	app.SetOverlay(doc)

	win.initSystemTime()
	win.initSystemPower()

	go win.watchOsdEvents()
	go win.watchCPU()

	app.Listen(fbiw.DocChange, func(e *fbiw.Event) {
		d := e.DocChange.Doc
		if d.Title() != `` {
			win.theTitle.SetText(d.Title())
		}
	}, fbiw.EventOptions{})

	return win
}

func (win *OverlayWindow) SetTitle(s string) {
	win.theTitle.SetText(s)
}

func (win *OverlayWindow) initSystemTime() {
	win.txtTime.SetText(time.Now().Format(`15:04`))
	go func() {
		last := ``
		for range time.Tick(time.Minute) {
			select {
			case <-win.app.Context().Done():
				return
			default:
				win.app.Async(func() {
					now := time.Now().Format(`15:04`)
					if now == last {
						return
					}
					last = now
					win.txtTime.SetText(now)
				})
			}
		}
	}()
}

type BatterInfo struct {
	Capacity       uint8   // 当前电量百分比
	ChargingStatus string  // 充电状态 Charging/Discharging/Full
	Temperature    float32 // 当前温度
}

func ReadPowerStatus() (*BatterInfo, error) {
	paths, err := filepath.Glob(`/sys/class/power_supply/*/type`)
	if err != nil {
		return nil, err
	}
	var batteryPath string
	for _, path := range paths {
		ty, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		ts := strings.ToLower(strings.TrimSpace(string(ty)))
		if ts == `battery` {
			batteryPath = path
			break
		}
	}
	if batteryPath == `` {
		return nil, fmt.Errorf(`没找到电池目录`)
	}

	dir, _ := filepath.Split(batteryPath)
	uevent := filepath.Join(dir, `uevent`)

	fp, err := os.Open(uevent)
	if err != nil {
		return nil, err
	}
	defer fp.Close()

	info := BatterInfo{}

	scn := bufio.NewScanner(fp)
	for scn.Scan() {
		key, value, ok := strings.Cut(scn.Text(), `=`)
		if !ok {
			continue
		}
		switch key {
		case `POWER_SUPPLY_CAPACITY`:
			n, _ := strconv.Atoi(value)
			info.Capacity = uint8(n)
		case `POWER_SUPPLY_STATUS`:
			info.ChargingStatus = value
		case `POWER_SUPPLY_TEMP`:
			n, _ := strconv.Atoi(value)
			info.Temperature = float32(n) / 10
		}
	}
	if scn.Err() != nil {
		return nil, scn.Err()
	}

	return &info, nil
}

// 监听netlink事件。回调发生在线程中。
func WatchKernelObjectEvents(ctx context.Context, callback func(event *kobject.Event)) error {
	client, err := kobject.New()
	if err != nil {
		log.Println(err)
		return err
	}

	go func() {
		defer client.Close()

		for {
			event, err := client.Receive()
			if err != nil {
				log.Println(err)
				break
			}
			select {
			case <-ctx.Done():
				return
			default:
				callback(event)
			}
		}
	}()

	return nil
}

func (win *OverlayWindow) initSystemPower() {
	last := uint8(0)
	lastCharging := false

	update := func() {
		info, err := ReadPowerStatus()
		if err != nil {
			log.Println(err)
			return
		}

		charging := info.ChargingStatus == `Charging`

		if info.Capacity == last && charging == lastCharging {
			return
		}

		win.app.Async(func() {
			win.txtBatteryPercentage.SetText(fmt.Sprintf(`%d%%`, info.Capacity))

			// 只有充电的时候显示。放电或已满均不显示。
			win.boxBatteryCharging.Base().SetProp(`display`, fmt.Sprint(charging))

			last = info.Capacity
			lastCharging = charging
		})
	}

	update()

	go func() {
		// TODO event 里面其实已经有当前的数据了
		if err := WatchKernelObjectEvents(context.Background(), func(event *kobject.Event) {
			if event.Subsystem == `power_supply` {
				update()
			}
		}); err != nil {
			for range time.Tick(time.Second * 10) {
				select {
				case <-win.app.Context().Done():
					return
				default:
					update()
				}
			}
		}
	}()
}

func (win *OverlayWindow) watchOsdEvents() {
	watcher, err := fswatcher.NewWatcher()
	if err != nil {
		log.Println(`创建观察器失败:`, err)
		return
	}
	defer watcher.Close()

	if err := watcher.AddRecursive(`/tmp/trimui_osd`, fswatcher.All); err != nil {
		log.Println(`观察OSD目录失败:`, err)
		return
	}

	ctx := context.Background()

	var (
		statusVolumeOpen    = false
		statusBluetoothOpen = false
		statusWifiOpen      = false
	)

	handleOsdEvent := func(path string, force bool) {
		suffix, found := strings.CutPrefix(path, `/tmp/trimui_osd/`)
		if !found {
			return
		}
		switch suffix {
		case `slider_volume/status`:
			data, _ := os.ReadFile(path)
			open := !strings.HasPrefix(strings.TrimSpace(string(data)), `0/`)
			if open != statusVolumeOpen || force {
				win.app.Async(func() {
					log.Println(`音量状态:`, open)
					win.txtVolumeOpen.SetText(fbiw.Iif(open, string(rune(0xefcf)), ``))
				})
				statusVolumeOpen = open
			}
		case `toggle_bt/status`:
			data, _ := os.ReadFile(path)
			open := strings.TrimSpace(string(data)) != `0`
			if open != statusBluetoothOpen || force {
				win.app.Async(func() {
					log.Println(`蓝牙状态:`, open)
					win.txtBluetoothOpen.SetText(fbiw.Iif(open, string(rune(0xf00af)), ``))
				})
				statusBluetoothOpen = open
			}
		case `toggle_wifi/status`:
			// TODO wifi打开不等于已经分配到ip，这里需要区别状态
			data, _ := os.ReadFile(path)
			open := strings.TrimSpace(string(data)) != `0`
			if open != statusWifiOpen || force {
				win.app.Async(func() {
					log.Println(`网络状态:`, open)
					win.txtWifiOpen.SetText(fbiw.Iif(open, string(rune(0xf1eb)), ``))
				})
				statusWifiOpen = open
			}
		}
	}

	handleOsdShow := func(show bool) {
		if show {
			win.app.DetachAsync()
		} else {
			win.app.AttachAsync()
		}
	}

	handleOsdEvent(`/tmp/trimui_osd/slider_volume/status`, true)
	handleOsdEvent(`/tmp/trimui_osd/toggle_bt/status`, true)
	handleOsdEvent(`/tmp/trimui_osd/toggle_wifi/status`, true)

	for {
		defer watcher.Close()
		select {
		case <-ctx.Done():
			return
		case err := <-watcher.Errors:
			log.Println(`非致命错误:`, err)
		case event := <-watcher.Events:
			// log.Println(`收到事件:`, event)
			if strings.Contains(event.Name, `osdd_show_up`) {
				handleOsdShow(event.Op.Has(fswatcher.Create))
				break
			}
			if event.Op.Has(fswatcher.Write) {
				handleOsdEvent(event.Name, false)
			}
		}
	}
}

type CPUStat struct {
	User      uint64
	Nice      uint64
	System    uint64
	Idle      uint64
	IOWait    uint64
	IRQ       uint64
	SoftIRQ   uint64
	Steal     uint64
	Guest     uint64
	GuestNice uint64
}

func readCPUStat() (CPUStat, error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return CPUStat{}, err
	}

	var stat CPUStat

	_, err = fmt.Sscanf(
		string(data),
		"cpu %d %d %d %d %d %d %d %d %d %d",
		&stat.User,
		&stat.Nice,
		&stat.System,
		&stat.Idle,
		&stat.IOWait,
		&stat.IRQ,
		&stat.SoftIRQ,
		&stat.Steal,
		&stat.Guest,
		&stat.GuestNice,
	)

	return stat, err
}

func cpuUsage(prev, curr CPUStat) float64 {
	prevIdle := prev.Idle + prev.IOWait
	currIdle := curr.Idle + curr.IOWait

	prevTotal :=
		prev.User +
			prev.Nice +
			prev.System +
			prev.Idle +
			prev.IOWait +
			prev.IRQ +
			prev.SoftIRQ +
			prev.Steal

	currTotal :=
		curr.User +
			curr.Nice +
			curr.System +
			curr.Idle +
			curr.IOWait +
			curr.IRQ +
			curr.SoftIRQ +
			curr.Steal

	totalDelta := currTotal - prevTotal
	idleDelta := currIdle - prevIdle

	if totalDelta == 0 {
		return 0
	}

	return float64(totalDelta-idleDelta) / float64(totalDelta)
}

func (win *OverlayWindow) watchCPU() {
	ctx := context.Background()
	prev, err := readCPUStat()
	if err != nil {
		log.Println(`读CPU状态失败:`, err)
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second * 2):
			curr, err := readCPUStat()
			if err != nil {
				log.Println(`读CPU状态失败:`, err)
				break
			}
			usage := cpuUsage(prev, curr)
			win.app.Async(func() {
				win.txtCpuStatus.SetText(fmt.Sprintf(`[%d/%d%%]`,
					int(usage*float64(runtime.NumCPU())*100), runtime.NumCPU()*100,
				))
			})
			prev = curr
		}
	}
}
