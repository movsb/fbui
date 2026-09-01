package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbui/pkg/config"
	"github.com/movsb/fbui/pkg/game_library"
	"github.com/movsb/fbui/pkg/launcher"
	"github.com/movsb/fbui/pkg/video_player"
	"github.com/movsb/gm/protocols/clients"
	"github.com/movsb/gm/protocols/go/proto"
	"google.golang.org/grpc/status"
)

const gmServerHome = "http://192.168.10.124:8888"

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
	name  string
	value any
}

type storePage struct {
	level storeLevel
	title string
	items []storeItem
	state any
	game  *proto.Game
}

type storeItemView struct {
	root fbiw.Box
	name *fbiw.Text `css:".name"`
}

type StoreNavigator struct {
	window  *MainWindow
	root    fbiw.Box                  `css:"#store"`
	title   *fbiw.Text                `css:"#store-title"`
	list    *fbiw.Scroll              `css:"#store-list"`
	message fbiw.Box                  `css:"#store-message"`
	msgText *fbiw.Text                `css:"#store-message text"`
	preview *fbiw.Stack               `css:"#store-preview"`
	image   *fbiw.Image               `css:"#store-preview img"`
	video   *video_player.VideoPlayer `css:"#store-preview video"`

	// 游戏商店元数据来源。
	metadata *clients.ProtoClient
	// 游戏本地二进制存储。
	store *game_library.Store
	// 当前目录浏览栈。
	stack []storePage
	busy  bool
}

func NewStoreNavigator(win *MainWindow) *StoreNavigator {
	metadata := clients.NewFromHome(gmServerHome, "")
	blobs := clients.NewFromHome(gmServerHome, "")
	n := &StoreNavigator{
		window:   win,
		metadata: metadata,
		store: game_library.New(
			filepath.Join(config.SDCARDRoot, ".fbui", "gm"),
			game_library.GRPCSource{Client: blobs.BlobService},
		),
	}
	win.doc.Bind(n)
	n.root.Listen(fbiw.StickDownEvent, n.handleEvents)
	n.preview.Listen(fbiw.StickDownEvent, n.handlePreviewEvents)
	n.list.Listen(fbiw.ScrollSelectionChange, func(*fbiw.Event) { n.updatePagination() })
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
	n.list.SetIndex(0, 0, 0)
	n.list.Activate()
}

// 渲染栈顶元素。
func (n *StoreNavigator) render(state any) {
	page := &n.stack[len(n.stack)-1]
	n.title.SetText(page.title)
	n.message.SetProp("display", "false")
	n.list.SetProp("display", "true")
	n.list.SetItems(
		len(page.items),
		func() (fbiw.Box, *storeItemView) {
			view := fbiw.Unmarshal[storeItemView](n.window.doc, `<block padding="0 10"><inline spacer align=middle><text class="name"></text></inline></block>`)
			return view.root, view
		},
		func(view *storeItemView, index int) {
			view.name.SetText(page.items[index].name)
		},
	)
	if len(page.items) == 0 {
		n.showMessage("没有内容")
	}
	if state != nil {
		n.list.SetState(state)
	}
	n.updatePagination()
}

func (n *StoreNavigator) showMessage(message string) {
	n.msgText.SetText(message)
	n.message.SetProp("display", "true")
	n.list.SetProp("display", "false")
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

	index := n.list.DataIndex()
	text := ""
	if index >= 0 {
		text = fmt.Sprintf("%d/%d", index+1, n.list.DataCount())
	}
	n.window.statusBarNav.pagination.SetText(text)
}

func (n *StoreNavigator) handleEvents(event *fbiw.Event) {
	if n.busy {
		event.StopPropagation()
		return
	}
	name := event.Stick.Name
	if name == fbiw.B {
		if len(n.stack) == 1 {
			n.list.Deselect()
			n.stack[len(n.stack)-1].state = nil
			n.window.statusBarNav.showPagination(false)
			n.window.statusBarNav.activate()
		} else {
			n.stack = n.stack[:len(n.stack)-1]
			n.render(n.stack[len(n.stack)-1].state)
			n.list.Activate()
		}
		event.StopPropagation()
		return
	}
	if name == fbiw.Up && len(n.stack) == 1 && n.list.DataRowIndex() <= 0 {
		n.list.Deselect()
		n.stack[len(n.stack)-1].state = nil
		n.window.statusBarNav.showPagination(false)
		n.window.statusBarNav.activate()
		event.StopPropagation()
		return
	}
	if name != fbiw.A || n.list.DataIndex() < 0 {
		return
	}
	page := &n.stack[len(n.stack)-1]
	page.state = n.list.GetState()
	item := page.items[n.list.DataIndex()]
	switch page.level {
	case storeRoot:
		n.loadKinds(item.value.(storeLevel))
	case storePlatforms:
		platform := item.value.(*proto.Platform)
		n.loadGames(platform.GetId(), 0, displayNames(platform.GetNames()))
	case storeSeries:
		series := item.value.(*proto.Series)
		n.loadGames(0, series.GetId(), displayNames(series.GetNames()))
	case storeGames:
		n.loadReleases(item.value.(*proto.Game))
	case storeReleases:
		n.loadAssets(page.game, item.value.(*proto.Release))
	case storeAssets:
		n.openAsset(page.game, item.value.(*proto.Asset))
	}
	event.StopPropagation()
}

func (n *StoreNavigator) async(title string, load func(context.Context) (storePage, error)) {
	n.busy = true
	n.window.doc.SetTimeout(500, func() {
		if n.busy {
			n.showMessage(title + "...")
		}
	})
	go func() {
		page, err := load(context.Background())
		n.window.app.Async(func() {
			n.busy = false
			if err != nil {
				n.render(n.stack[len(n.stack)-1].state)
				n.window.app.ShowAlertDialog(n.window.doc, fbiw.AlertDialogOptions{
					Title:       fmt.Sprintf(`%s失败`, title),
					Description: maybeGrpcError(err),
				})
				return
			}
			n.stack = append(n.stack, page)
			n.render(nil)
			// n.list.SetIndex(0, 0, 0)
			n.list.Activate()
		})
	}()
}

func (n *StoreNavigator) loadKinds(level storeLevel) {
	n.async("加载", func(ctx context.Context) (storePage, error) {
		page := storePage{level: level}
		switch level {
		case storePlatforms:
			response, err := n.metadata.GameManager.ListPlatforms(ctx, &proto.ListPlatformsRequest{})
			if err != nil {
				return page, err
			}
			page.title = "平台"
			for _, item := range response.GetPlatforms() {
				page.items = append(page.items, storeItem{name: displayNames(item.GetNames()), value: item})
			}
		case storeSeries:
			response, err := n.metadata.GameManager.ListSeries(ctx, &proto.ListSeriesRequest{})
			if err != nil {
				return page, err
			}
			page.title = "系列"
			for _, item := range response.GetSeries() {
				page.items = append(page.items, storeItem{name: displayNames(item.GetNames()), value: item})
			}
		}
		return page, nil
	})
}

func (n *StoreNavigator) loadGames(platformID, seriesID int32, parent string) {
	n.async("加载游戏", func(ctx context.Context) (storePage, error) {
		response, err := n.metadata.GameManager.ListGames(ctx,
			&proto.ListGamesRequest{
				PlatformId: platformID,
				SeriesId:   seriesID,
			},
		)
		page := storePage{
			level: storeGames,
			title: parent,
		}
		if err != nil {
			return page, err
		}
		for _, item := range response.GetGames() {
			page.items = append(page.items, storeItem{
				name:  displayNames(item.GetNames()),
				value: item,
			})
		}
		return page, nil
	})
}

func (n *StoreNavigator) loadReleases(game *proto.Game) {
	n.async("加载发行版", func(ctx context.Context) (storePage, error) {
		response, err := n.metadata.GameManager.ListReleases(ctx,
			&proto.ListReleasesRequest{GameId: game.GetId()},
		)
		page := storePage{
			level: storeReleases,
			title: displayNames(game.GetNames()),
			game:  game,
		}
		if err != nil {
			return page, err
		}
		for _, item := range response.GetReleases() {
			page.items = append(page.items, storeItem{
				name:  displayNames(item.GetNames()),
				value: item,
			})
		}
		return page, nil
	})
}

func (n *StoreNavigator) loadAssets(game *proto.Game, release *proto.Release) {
	n.async("加载资源", func(ctx context.Context) (storePage, error) {
		response, err := n.metadata.GameManager.ListAssets(ctx,
			&proto.ListAssetsRequest{
				Kind:      proto.Kind_KIND_RELEASE,
				KindId:    release.GetId(),
				WithBlobs: true,
			},
		)
		page := storePage{
			level: storeAssets,
			title: displayNames(release.GetNames()),
			game:  game,
		}
		if err != nil {
			return page, err
		}
		for _, item := range response.GetAssets() {
			page.items = append(page.items, storeItem{
				name:  fmt.Sprintf("%s  ·  %s  ·  %s", item.GetName(), assetTypeName(item.GetType()), formatSize(item.GetSize())),
				value: item,
			})
		}
		return page, nil
	})
}

func (n *StoreNavigator) openAsset(game *proto.Game, asset *proto.Asset) {
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
				n.window.app.Async(func() {
					if n.busy {
						n.showMessage(fmt.Sprintf("%.0f%%\n%s", progress, message))
					}
				})
			},
		)
		time.Sleep(time.Millisecond * 100)
		n.window.app.Async(func() {
			n.busy = false
			n.render(n.stack[len(n.stack)-1].state)
			n.list.Activate()
			if err != nil {
				n.window.app.ShowAlertDialog(n.window.doc,
					fbiw.AlertDialogOptions{
						Title:       `下载失败`,
						Description: err.Error(),
					},
				)
				return
			}
			switch asset.GetType() {
			case proto.AssetType_ASSET_TYPE_ROM:
				n.runROM(game.GetPlatformId(), path)
			case proto.AssetType_ASSET_TYPE_COVER, proto.AssetType_ASSET_TYPE_SCREENSHOT, proto.AssetType_ASSET_TYPE_LOGO:
				n.showImage(path)
			case proto.AssetType_ASSET_TYPE_VIDEO:
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

func (n *StoreNavigator) showImage(path string) {
	n.video.SetProp("display", "false")
	n.image.SetPath(path)
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
	if event.Stick.Name == fbiw.B {
		n.video.Stop()
		n.preview.SetProp("display", "false")
		n.list.Activate()
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

func (n *StoreNavigator) runROM(platformID int32, path string) {
	wanted := platformEmulators[platformID]
	var emulator *config.LaunchConfig
	for _, candidate := range config.LoadDir(filepath.Join(config.SDCARDRoot, "Emus")) {
		if strings.EqualFold(filepath.Base(candidate.Dir), wanted) {
			emulator = candidate
			break
		}
	}
	if wanted == "" || emulator == nil {
		n.window.app.ShowAlertDialog(n.window.doc,
			fbiw.AlertDialogOptions{
				Title:       `没有模拟器映射`,
				Description: fmt.Sprintf(`资源已下载，但是不知道如何打开平台: %d`, platformID),
			})
		return
	}
	n.window.app.Detach()
	go func() {
		defer n.window.app.AttachAsync()
		script := emulator.LauncherScriptPath()
		log.Println("启动仓库 ROM：", script, path)
		if err := launcher.RunScript(context.Background(), script, path); err != nil {
			// 已经运行，错误码不可靠，直接忽略。
			if strings.Contains(err.Error(), `exit status`) {
				return
			}
			n.window.doc.Async(func() {
				n.window.app.ShowAlertDialog(n.window.doc, fbiw.AlertDialogOptions{
					Title:       `启动失败`,
					Description: err.Error(),
				})
			})
		}
	}()
}

func displayNames(names []*proto.Name) string {
	for _, language := range []proto.Language{
		proto.Language_LANGUAGE_CHINESE,
		proto.Language_LANGUAGE_ENGLISH,
		proto.Language_LANGUAGE_JAPANESE,
	} {
		for _, name := range names {
			if name.GetLanguage() == language && name.GetName() != "" {
				return name.GetName()
			}
		}
	}
	for _, name := range names {
		if name.GetName() != "" {
			return name.GetName()
		}
	}
	return "未命名"
}

func assetTypeName(kind proto.AssetType) string {
	name := map[proto.AssetType]string{
		proto.AssetType_ASSET_TYPE_ROM:        "ROM",
		proto.AssetType_ASSET_TYPE_COVER:      "封面",
		proto.AssetType_ASSET_TYPE_SCREENSHOT: "截图",
		proto.AssetType_ASSET_TYPE_VIDEO:      "视频",
		proto.AssetType_ASSET_TYPE_LOGO:       "Logo",
		proto.AssetType_ASSET_TYPE_MANUAL:     "说明书",
		proto.AssetType_ASSET_TYPE_SYSTEM:     "系统",
		proto.AssetType_ASSET_TYPE_OTHER:      "其它",
	}[kind]
	if name == "" {
		return "未指定"
	}
	return name
}

func formatSize(size int32) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
}

func maybeGrpcError(err error) string {
	if err == nil {
		return `<nil>`
	}
	if st, ok := status.FromError(err); ok {
		return fmt.Sprintf("%s\n\n%s", st.Code(), st.Message())
	}
	return err.Error()
}
