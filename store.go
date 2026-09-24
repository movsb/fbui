package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbiw/input/sticks"
	"github.com/movsb/fbui/pkg/config"
	"github.com/movsb/fbui/pkg/game_library"
	"github.com/movsb/fbui/pkg/launcher"
	"github.com/movsb/fbui/pkg/menu_popup"
	"github.com/movsb/fbui/pkg/progress_popup"
	"github.com/movsb/fbui/pkg/video_player"
)

const gmBlobBaseURL = "http://192.168.10.10:8888"

type storeLevel int

const (
	storeRoot storeLevel = iota
	storePlatforms
	storeSeries
	storeGames
	storeReleases
	storeAssets
)

type storeItem struct {
	name     string
	platform string
	value    any
}

type storePage struct {
	level storeLevel
	title string
	items []storeItem
	state any
	game  *game_library.Game
	// 系列中的游戏使用独立列表，并显示所属平台。
	groupByPlatform bool
}

type storeItemView struct {
	root fbiw.Box
	name *fbiw.Text `css:".name"`
}

type storeSeriesGameView struct {
	root     fbiw.Box
	name     *fbiw.Text `css:".name"`
	platform *fbiw.Text `css:".platform"`
}

func (view *storeSeriesGameView) ListSelectionChanged(selected bool) {
	view.name.SetMarqueeRunning(selected)
}

var _ fbiw.ListSelectionAware = (*storeSeriesGameView)(nil)

// ScrollSelectionChanged implements [fbiw.ListSelectionAware].
func (view *storeItemView) ListSelectionChanged(selected bool) {
	view.name.SetMarqueeRunning(selected)
}

var _ fbiw.ListSelectionAware = (*storeItemView)(nil)

type StoreNavigator struct {
	window  *MainWindow
	shadow  fbiw.Box                  `css:"#store"`
	title   *fbiw.Text                `css:"#store-title"`
	list    *fbiw.List                `css:"#store-list"`
	series  *fbiw.List                `css:"#store-series-list"`
	message fbiw.Box                  `css:"#store-message"`
	msgText *fbiw.Text                `css:"#store-message text"`
	preview *fbiw.Stack               `css:"#store-preview"`
	image   *fbiw.Image               `css:"#store-preview img"`
	video   *video_player.VideoPlayer `css:"#store-preview video"`

	// 游戏商店元数据来源。
	metadata    *game_library.Library
	metadataErr error
	// 游戏本地二进制存储。
	store *game_library.Store
	// 当前目录浏览栈。
	stack      []storePage
	busy       bool
	remoteBusy atomic.Bool
}

func NewStoreNavigator(win *MainWindow) *StoreNavigator {
	root := filepath.Join(config.SDCARDRoot, ".fbui", "gm")
	metadata, metadataErr := game_library.OpenLibrary(filepath.Join(root, "gm.db"))
	n := &StoreNavigator{
		window:      win,
		metadata:    metadata,
		metadataErr: metadataErr,
		store: game_library.New(
			root,
			game_library.NewHTTPBlobSource(gmBlobBaseURL),
		),
	}
	win.doc.Bind(n)
	n.shadow.Listen(fbiw.InputDownEvent, n.handleEvents)
	n.preview.Listen(fbiw.InputDownEvent, n.handlePreviewEvents)
	n.list.Listen(fbiw.ListSelectionChange, func(*fbiw.Event) { n.updatePagination() })
	n.series.Listen(fbiw.ListSelectionChange, func(*fbiw.Event) { n.updatePagination() })
	n.stack = []storePage{{
		level: storeRoot,
		title: "仓库",
		items: []storeItem{{name: "平台", value: storePlatforms}, {name: "系列", value: storeSeries}},
	}}
	n.render(nil)
	return n
}

func (n *StoreNavigator) activate() {
	// n.window.statusBarNav.showPagination(true)
	n.render(n.stack[len(n.stack)-1].state)
	list := n.activeList()
	list.SetIndex(0, 0, 0)
	list.Activate()
}

func (n *StoreNavigator) activeList() *fbiw.List {
	if len(n.stack) > 0 && n.stack[len(n.stack)-1].groupByPlatform {
		return n.series
	}
	return n.list
}

// 渲染栈顶元素。
func (n *StoreNavigator) render(state any) {
	page := &n.stack[len(n.stack)-1]
	n.title.SetText(page.title)
	n.message.SetProp("display", "false")
	n.list.SetProp("display", fmt.Sprint(!page.groupByPlatform))
	n.series.SetProp("display", fmt.Sprint(page.groupByPlatform))
	if page.groupByPlatform {
		n.series.SetItems(
			len(page.items),
			func() (fbiw.Box, *storeSeriesGameView) {
				view := n.window.doc.Instantiate[storeSeriesGameView](`store-series-game-item`)
				return view.root, view
			},
			func(view *storeSeriesGameView, index int) {
				view.name.SetText(page.items[index].name)
				view.platform.SetText(page.items[index].platform)
			},
		)
	} else {
		n.list.SetItems(
			len(page.items),
			func() (fbiw.Box, *storeItemView) {
				view := n.window.doc.Instantiate[storeItemView](`store-item`)
				return view.root, view
			},
			func(view *storeItemView, index int) {
				view.name.SetText(page.items[index].name)
			},
		)
	}
	if len(page.items) == 0 {
		n.showMessage("没有内容")
	}
	if state != nil {
		n.activeList().SetState(state)
	}
	n.updatePagination()
}

func (n *StoreNavigator) showMessage(message string) {
	n.msgText.SetText(message)
	n.message.SetProp("display", "true")
	n.list.SetProp("display", "false")
	n.series.SetProp("display", "false")
	n.updatePagination()
}

func (n *StoreNavigator) updatePagination() {
	// 正在浏览平台/系列，数量是确定的，此时不需要显示分页。
	if len(n.stack) == 1 {
		n.window.statusBarNav.showPagination(false)
		n.window.statusBarNav.showCatBar(true)
		return
	}

	n.window.statusBarNav.showPagination(true)
	n.window.statusBarNav.showCatBar(false)

	list := n.activeList()
	index := list.DataIndex()
	text := ""
	if index >= 0 {
		text = fmt.Sprintf("%d/%d", index+1, list.DataCount())
	}
	n.window.statusBarNav.pagination.SetText(text)
}

func (n *StoreNavigator) handleEvents(event *fbiw.Event) {
	if n.busy {
		event.StopPropagation()
		return
	}
	name := event.Input.Name
	list := n.activeList()
	if name == sticks.Menu {
		n.showMenu()
		event.StopPropagation()
		return
	}
	if name == sticks.B {
		if len(n.stack) == 1 {
			list.Deselect()
			n.stack[len(n.stack)-1].state = nil
			n.window.statusBarNav.showPagination(false)
			n.window.statusBarNav.activate()
		} else {
			n.stack = n.stack[:len(n.stack)-1]
			n.render(n.stack[len(n.stack)-1].state)
			n.activeList().Activate()
		}
		event.StopPropagation()
		return
	}
	if name == sticks.Up && len(n.stack) == 1 && list.DataRowIndex() <= 0 {
		list.Deselect()
		n.stack[len(n.stack)-1].state = nil
		n.window.statusBarNav.showPagination(false)
		n.window.statusBarNav.activate()
		event.StopPropagation()
		return
	}
	if name != sticks.A || list.DataIndex() < 0 {
		return
	}
	page := &n.stack[len(n.stack)-1]
	page.state = list.GetState()
	item := page.items[list.DataIndex()]
	switch page.level {
	case storeRoot:
		n.loadKinds(item.value.(storeLevel))
	case storePlatforms:
		platform := item.value.(*game_library.Platform)
		n.loadGames(platform.ID, 0, displayNames(platform.Names))
	case storeSeries:
		series := item.value.(*game_library.Series)
		n.loadGames(0, series.ID, displayNames(series.Names))
	case storeGames:
		n.loadReleases(item.value.(*game_library.Game))
	case storeReleases:
		n.loadAssets(page.game, item.value.(*game_library.Release))
	case storeAssets:
		n.openAsset(page.game, item.value.(*game_library.Asset))
	}
	event.StopPropagation()
}

func (n *StoreNavigator) showMenu() {
	menu_popup.NewMenuPopup(n.window.app, n.window.doc, []menu_popup.MenuItem{
		{
			Name: "更新仓库数据库...",
			Click: func() {
				n.window.app.ShowAlertDialog(n.window.doc, fbiw.AlertDialogOptions{
					Title:       "更新仓库数据库？",
					Description: "将从 GM 下载最新数据库并替换当前仓库数据库。",
					ActionText:  "更新",
					CancelText:  "取消",
					OnAction:    n.updateDatabase,
				})
			},
		},
	}, nil, nil)
}

func (n *StoreNavigator) updateDatabase() {
	if n.busy {
		return
	}
	n.busy = true
	popup := progress_popup.New(n.window.app, n.window.doc, "正在检查数据库版本")
	target := filepath.Join(config.SDCARDRoot, ".fbui", "gm", "gm.db")
	go func() {
		lastUpdate := time.Time{}
		newLibrary, err := downloadLibrarySnapshot(context.Background(), http.DefaultClient, gmBlobBaseURL, target,
			func(received, total int64) {
				if total == 0 {
					n.window.doc.Async(func() { popup.SetIndeterminate("正在生成数据库快照") })
					return
				}
				now := time.Now()
				if received != total && now.Sub(lastUpdate) < 100*time.Millisecond {
					return
				}
				lastUpdate = now
				n.window.doc.Async(func() {
					if received == total {
						popup.SetIndeterminate("正在校验并启用数据库")
					} else {
						popup.SetProgress(received, total)
					}
				})
			})
		n.window.doc.Async(func() {
			n.busy = false
			popup.Close()
			if err != nil {
				n.activeList().Activate()
				n.window.app.ShowAlertDialog(n.window.doc, fbiw.AlertDialogOptions{
					Title:       "更新仓库数据库失败",
					Description: err.Error(),
				})
				return
			}
			oldLibrary := n.metadata
			n.metadata = newLibrary
			n.metadataErr = nil
			if oldLibrary != nil {
				_ = oldLibrary.Close()
			}
			n.stack = []storePage{{
				level: storeRoot,
				title: "仓库",
				items: []storeItem{{name: "平台", value: storePlatforms}, {name: "系列", value: storeSeries}},
			}}
			n.render(nil)
			n.activeList().SetIndex(0, 0, 0)
			n.activeList().Activate()
			n.window.app.ShowAlertDialog(n.window.doc, fbiw.AlertDialogOptions{
				Title:       "仓库数据库已更新",
				Description: "最新数据库快照已启用。",
			})
		})
	}()
}

func (n *StoreNavigator) async(title string, load func(context.Context) (storePage, error)) {
	n.busy = true
	n.showMessage(title + "...")
	go func() {
		page, err := load(context.Background())
		n.window.doc.Async(func() {
			n.busy = false
			if err != nil {
				n.render(n.stack[len(n.stack)-1].state)
				n.window.app.ShowAlertDialog(n.window.doc, fbiw.AlertDialogOptions{
					Title:       fmt.Sprintf(`%s失败`, title),
					Description: err.Error(),
				})
				return
			}
			n.stack = append(n.stack, page)
			n.render(nil)
			// n.activeList().SetIndex(0, 0, 0)
			n.activeList().Activate()
		})
	}()
}

func (n *StoreNavigator) loadKinds(level storeLevel) {
	n.async("加载", func(ctx context.Context) (storePage, error) {
		page := storePage{level: level}
		if n.metadataErr != nil {
			return page, n.metadataErr
		}
		switch level {
		case storePlatforms:
			response, err := n.metadata.ListPlatforms(ctx)
			if err != nil {
				return page, err
			}
			page.title = "平台"
			for _, item := range response {
				page.items = append(page.items, storeItem{name: displayNames(item.Names), value: item})
			}
		case storeSeries:
			response, err := n.metadata.ListSeries(ctx)
			if err != nil {
				return page, err
			}
			page.title = "系列"
			for _, item := range response {
				page.items = append(page.items, storeItem{name: displayNames(item.Names), value: item})
			}
		}
		return page, nil
	})
}

func (n *StoreNavigator) loadGames(platformID, seriesID int32, parent string) {
	n.async("加载游戏", func(ctx context.Context) (storePage, error) {
		response, err := n.metadata.ListGames(ctx, platformID, seriesID)
		page := storePage{
			level:           storeGames,
			title:           parent,
			groupByPlatform: seriesID != 0,
		}
		if err != nil {
			return page, err
		}
		page.items = buildGameItems(response, seriesID != 0)
		return page, nil
	})
}

func buildGameItems(games []*game_library.Game, showPlatform bool) []storeItem {
	if showPlatform {
		sort.SliceStable(games, func(i, j int) bool {
			leftPlatform := displayNames(games[i].PlatformNames)
			rightPlatform := displayNames(games[j].PlatformNames)
			if leftPlatform != rightPlatform {
				return leftPlatform < rightPlatform
			}
			leftGame := displayNames(games[i].Names)
			rightGame := displayNames(games[j].Names)
			if leftGame != rightGame {
				return leftGame < rightGame
			}
			return games[i].ID < games[j].ID
		})
	}
	items := make([]storeItem, 0, len(games))
	for _, game := range games {
		item := storeItem{name: displayNames(game.Names), value: game}
		if showPlatform {
			item.platform = displayNames(game.PlatformNames)
		}
		items = append(items, item)
	}
	return items
}

func (n *StoreNavigator) loadReleases(game *game_library.Game) {
	n.async("加载发行版", func(ctx context.Context) (storePage, error) {
		response, err := n.metadata.ListReleases(ctx, game.ID)
		page := storePage{
			level: storeReleases,
			title: displayNames(game.Names),
			game:  game,
		}
		if err != nil {
			return page, err
		}
		for _, item := range response {
			page.items = append(page.items, storeItem{
				name:  displayNames(item.Names),
				value: item,
			})
		}
		return page, nil
	})
}

func (n *StoreNavigator) loadAssets(game *game_library.Game, release *game_library.Release) {
	n.async("加载资源", func(ctx context.Context) (storePage, error) {
		response, err := n.metadata.ListAssets(ctx, release.ID)
		page := storePage{
			level: storeAssets,
			title: displayNames(release.Names),
			game:  game,
		}
		if err != nil {
			return page, err
		}
		for _, item := range response {
			page.items = append(page.items, storeItem{
				name:  fmt.Sprintf("%s  ·  %s  ·  %s", item.Name, assetTypeName(item.Type), formatSize(item.Size)),
				value: item,
			})
		}
		return page, nil
	})
}

func (n *StoreNavigator) openAsset(game *game_library.Game, asset *game_library.Asset) {
	if asset.Type == game_library.AssetTypeROM {
		launchable, err := n.metadata.GetLaunchableAsset(context.Background(), asset.ID)
		if err != nil {
			title := `无法打开 ROM`
			if errors.Is(err, game_library.ErrUnsupportedMAMEVersion) {
				title = `ROM 不受支持`
			}
			n.window.app.ShowAlertDialog(n.window.doc, fbiw.AlertDialogOptions{
				Title:       title,
				Description: err.Error(),
			})
			return
		}
		asset = launchable.Asset
	}
	n.busy = true
	go func() {
		lastMessage := ""
		lastTime := time.Time{}
		path, err := n.store.Materialize(
			context.Background(), asset,
			func(message string, progress float32) {
				if message == lastMessage && (time.Since(lastTime) < time.Millisecond*250 && progress != 100) {
					return
				}
				lastMessage = message
				lastTime = time.Now()
				n.window.doc.Async(func() {
					if n.busy {
						n.showMessage(fmt.Sprintf("%.0f%%\n%s", progress, message))
					}
				})
			},
		)
		time.Sleep(time.Millisecond * 100)
		n.window.doc.Async(func() {
			n.busy = false
			n.render(n.stack[len(n.stack)-1].state)
			n.activeList().Activate()
			if err != nil {
				n.window.app.ShowAlertDialog(n.window.doc,
					fbiw.AlertDialogOptions{
						Title:       `下载失败`,
						Description: err.Error(),
					},
				)
				return
			}
			switch asset.Type {
			case game_library.AssetTypeROM:
				n.runROM(game.PlatformID, path)
			case game_library.AssetTypeCover, game_library.AssetTypeScreenshot, game_library.AssetTypeLogo:
				n.showImage(path)
			case game_library.AssetTypeVideo:
				n.showVideo(path)
			default:
				n.window.app.ShowAlertDialog(n.window.doc,
					fbiw.AlertDialogOptions{
						Title:       `下载成功`,
						Description: path,
					},
				)
			}
		})
	}()
}

func (n *StoreNavigator) OpenAssetByID(ctx context.Context, id int32) error {
	if n.window.app.Detached() {
		return fmt.Errorf(`启动器当前不在前台，不能启动游戏。`)
	}
	if n.metadataErr != nil {
		return n.metadataErr
	}
	launchable, err := n.metadata.GetLaunchableAsset(ctx, id)
	if err != nil {
		return err
	}
	if !n.remoteBusy.CompareAndSwap(false, true) {
		return errStoreOpenBusy
	}
	go func() {
		path, err := n.store.Materialize(context.Background(), launchable.Asset, func(message string, progress float32) {
			log.Printf("远程打开资源 %d：%s %.0f%%", id, message, progress)
		})
		if err != nil {
			n.remoteBusy.Store(false)
			n.showRemoteOpenError(id, err)
			return
		}
		n.window.doc.Async(func() {
			n.runROM(launchable.PlatformID, path, func() { n.remoteBusy.Store(false) })
		})
	}()
	return nil
}

func (n *StoreNavigator) showRemoteOpenError(id int32, err error) {
	log.Printf("远程打开资源 %d 失败：%v", id, err)
	n.window.doc.Async(func() {
		n.window.app.ShowAlertDialog(n.window.doc, fbiw.AlertDialogOptions{
			Title:       `远程启动失败`,
			Description: err.Error(),
		})
	})
}

func (n *StoreNavigator) showImage(path string) {
	n.video.SetProp("display", "false")
	n.image.SetOSPath(path)
	n.image.SetProp("display", "true")
	n.preview.SetProp("display", "true")
	n.preview.Activate()
}

func (n *StoreNavigator) showVideo(path string) {
	n.image.SetProp("display", "false")
	n.video.SetPath(path)
	n.video.SetProp("display", "true")
	n.preview.SetProp("display", "true")
	n.preview.Activate()
}

func (n *StoreNavigator) handlePreviewEvents(event *fbiw.Event) {
	if event.Input.Name == sticks.B {
		n.video.Stop()
		n.preview.SetProp("display", "false")
		n.activeList().Activate()
	}
	event.StopPropagation()
}

var platformEmulators = map[int32]string{
	1: "ARCADE",
	2: "FC",
	3: "GBC",
	4: "GB",
	5: "GBA",
	6: "MD",
	7: "SFC",
	8: "ATARI2600",
}

func (n *StoreNavigator) runROM(platformID int32, path string, done ...func()) {
	finish := func() {
		if len(done) > 0 && done[0] != nil {
			done[0]()
		}
	}
	wanted := platformEmulators[platformID]
	var emulator *config.LaunchConfig
	for _, candidate := range config.LoadDir(filepath.Join(config.SDCARDRoot, "Emus")) {
		if strings.EqualFold(filepath.Base(candidate.Dir), wanted) {
			emulator = candidate
			break
		}
	}
	if wanted == "" || emulator == nil {
		finish()
		log.Printf("无法打开仓库 ROM：平台 %d 没有模拟器映射", platformID)
		n.window.app.ShowAlertDialog(n.window.doc,
			fbiw.AlertDialogOptions{
				Title:       `没有模拟器映射`,
				Description: fmt.Sprintf(`资源已下载，但是不知道如何打开平台: %d`, platformID),
			})
		return
	}
	n.window.app.Detach()
	go func() {
		defer finish()
		defer n.window.app.AttachAsync()
		script := emulator.LauncherScriptPath()
		log.Println("启动仓库 ROM：", script, path)
		if err := launcher.RunScript(context.Background(), script, path); err != nil {
			// 已经运行，错误码不可靠，直接忽略。
			if strings.Contains(err.Error(), `exit status`) {
				return
			}
			log.Printf("启动仓库 ROM 失败：%v", err)
			n.window.doc.Async(func() {
				n.window.app.ShowAlertDialog(n.window.doc, fbiw.AlertDialogOptions{
					Title:       `启动失败`,
					Description: err.Error(),
				})
			})
		}
	}()
}

func displayNames(names []game_library.Name) string {
	for _, name := range names {
		if name.Language == game_library.LanguageChinese && strings.TrimSpace(name.Source) == "" && name.Name != "" {
			return name.Name
		}
	}
	for _, language := range []game_library.Language{
		game_library.LanguageChinese,
		game_library.LanguageEnglish,
		game_library.LanguageJapanese,
	} {
		for _, name := range names {
			if name.Language == language && name.Name != "" {
				return name.Name
			}
		}
	}
	for _, name := range names {
		if name.Name != "" {
			return name.Name
		}
	}
	return "未命名"
}

func assetTypeName(kind game_library.AssetType) string {
	name := map[game_library.AssetType]string{
		game_library.AssetTypeROM:        "ROM",
		game_library.AssetTypeCover:      "封面",
		game_library.AssetTypeScreenshot: "截图",
		game_library.AssetTypeVideo:      "视频",
		game_library.AssetTypeLogo:       "Logo",
		game_library.AssetTypeManual:     "说明书",
		game_library.AssetTypeSystem:     "系统",
		game_library.AssetTypeOther:      "其它",
	}[kind]
	if name == "" {
		return "未指定"
	}
	return name
}

func formatSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
}
