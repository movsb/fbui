package main

import (
	"log"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fswatcher/fswatcher"
	"github.com/movsb/fbiw"
	"github.com/movsb/fbiw/input/sticks"
	"github.com/movsb/fbui/pkg/config"
)

type launchConfigKind uint8

const (
	launchConfigApps launchConfigKind = 1 << iota
	launchConfigPorts
	launchConfigEmus
)

type launchConfigWatchRoot struct {
	path string
	kind launchConfigKind
}

// watchLaunchConfigs performs the initial load and keeps the three launcher
// lists in sync with installations and removals made while the UI is running.
func (w *MainWindow) watchLaunchConfigs() {
	watcher, err := fswatcher.NewWatcher()
	if err != nil {
		log.Println(`创建应用目录观察器失败:`, err)
		w.reloadLaunchConfigs(launchConfigApps | launchConfigPorts | launchConfigEmus)
		return
	}
	defer watcher.Close()

	roots := []launchConfigWatchRoot{
		{filepath.Join(config.SDCARDRoot, `Apps`), launchConfigApps},
		{`/usr/trimui/apps`, launchConfigApps},
		{filepath.Join(config.SDCARDRoot, `Ports`), launchConfigPorts},
		{filepath.Join(config.SDCARDRoot, `Emus`), launchConfigEmus},
	}
	for i := range roots {
		root, err := filepath.Abs(roots[i].path)
		if err == nil {
			roots[i].path = filepath.Clean(root)
		}
		if err := watcher.AddRecursive(roots[i].path, fswatcher.All); err != nil {
			log.Printf(`观察应用目录失败: %s: %v`, roots[i].path, err)
		}
	}

	// 先注册观察器再加载，确保初始化期间发生的修改也能触发刷新。
	w.reloadLaunchConfigs(launchConfigApps | launchConfigPorts | launchConfigEmus)

	var (
		dirty launchConfigKind
		timer *time.Timer
		wake  <-chan time.Time
	)
	for {
		select {
		case <-w.app.Context().Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Println(`观察应用目录出错:`, err)
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			for _, root := range roots {
				if pathWithin(event.Name, root.path) {
					dirty |= root.kind
				}
			}
			if dirty == 0 {
				continue
			}
			if timer == nil {
				timer = time.NewTimer(time.Second)
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(time.Second)
			}
			wake = timer.C
		case <-wake:
			changed := dirty
			dirty = 0
			wake = nil
			w.reloadLaunchConfigs(changed)
		}
	}
}

func pathWithin(path, root string) bool {
	path = filepath.Clean(path)
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

func (w *MainWindow) reloadLaunchConfigs(kinds launchConfigKind) {
	if kinds&launchConfigApps != 0 {
		w.asyncInitApps()
	}
	if kinds&launchConfigPorts != 0 {
		w.asyncInitPorts()
	}
	if kinds&launchConfigEmus != 0 {
		w.asyncInitEmus()
	}
}

// 加载所有的Apps列表。
// 包含机器系统目录的、存储卡内的。
func loadApps() []*config.LaunchConfig {
	launchConfigs := []*config.LaunchConfig{}

	launchConfigs = append(launchConfigs, config.LoadDir(filepath.Join(config.SDCARDRoot, `Apps`))...)
	launchConfigs = append(launchConfigs, config.LoadDir(`/usr/trimui/apps`)...)

	return launchConfigs
}

type _AppItem struct {
	root  fbiw.Box
	image *fbiw.Image `css:"img"`
	text  *fbiw.Text  `css:"text"`
}

// ScrollSelectionChanged implements [fbiw.ListSelectionAware].
func (view *_AppItem) ListSelectionChanged(selected bool) {
	view.text.SetMarqueeRunning(selected)
}

var _ fbiw.ListSelectionAware = (*_AppItem)(nil)

func (w *MainWindow) asyncInitApps() {
	apps := loadApps()
	w.doc.Async(func() {
		container := w.doc.GetBoxByID[*fbiw.List](`apps`)
		container.SetData(`apps`, apps)
		container.SetItems(len(apps),
			func() (fbiw.Box, *_AppItem) {
				item := w.doc.Instantiate[_AppItem](`app-item`)
				return item.root, item
			},
			func(item *_AppItem, index int) {
				app := apps[index]
				item.image.SetOSPath(app.IconPath())
				item.text.SetText(app.Name())
			},
		)
	})
}

func (w *MainWindow) asyncInitPorts() {
	apps := config.LoadDir(filepath.Join(config.SDCARDRoot, `Ports`))

	w.doc.Async(func() {
		scroll := w.doc.GetBoxByID[*fbiw.List](`ports`)
		scroll.SetData(`ports`, apps)
		scroll.SetItems(len(apps),
			func() (fbiw.Box, any) {
				item := w.doc.Instantiate[_AppItem](`app-item`)
				return item.root, item
			},
			func(item any, index int) {
				app := apps[index]
				appItem := item.(*_AppItem)
				appItem.image.SetOSPath(app.IconPath())
				appItem.text.SetText(app.Name())
			},
		)
	})
}

type LauncherNavigator struct {
	window  *MainWindow
	dataKey string
	scroll  *fbiw.List
}

func NewLauncherNavigator(win *MainWindow, selector string, dataKey string) *LauncherNavigator {
	n := LauncherNavigator{
		window:  win,
		dataKey: dataKey,
		scroll:  win.doc.QuerySelector[*fbiw.List](selector),
	}
	n.scroll.Listen(fbiw.InputDownEvent, n.handleKeyDown)
	return &n
}

func (n *LauncherNavigator) activate() {
	n.scroll.SetIndex(0, 0, 0)
	n.scroll.Activate()
}

func (n *LauncherNavigator) handleKeyDown(event *fbiw.Event) {
	if event.Input.Name == sticks.A && n.scroll.DataIndex() != -1 {
		n.openApp()
		event.StopPropagation()
		return
	}
	if event.Input.Name == sticks.B || (event.Input.Name == sticks.Up && n.scroll.DataRowIndex() <= 0) {
		n.scroll.Deselect()
		n.window.statusBarNav.activate()
		event.StopPropagation()
		return
	}
}

func (n *LauncherNavigator) openApp() {
	configs := n.scroll.GetData(n.dataKey).([]*config.LaunchConfig)
	config := configs[n.scroll.DataIndex()]
	n.window.app.Detach()
	go func() {
		defer n.window.app.AttachAsync()
		cmd := exec.Command(config.LauncherScriptPath())
		log.Println(`启动进程：`, cmd.String())
		cmd.Run()
	}()
}
