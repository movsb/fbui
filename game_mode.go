package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbui/pkg/config"
)

func loadDirectoryGames(root string) bool {
	data, err := os.ReadFile(filepath.Join(root, ".fbui", "game-mode"))
	if err != nil && !os.IsNotExist(err) {
		log.Printf("读取游戏模式失败：%v", err)
	}
	return strings.TrimSpace(string(data)) == "directory"
}

func gameModeToolName(directory bool) string {
	if directory {
		return "游戏模式：目录（切换为仓库）"
	}
	return "游戏模式：仓库（切换为目录）"
}

func (w *MainWindow) toggleGameMode() {
	directory := !w.directoryGames
	mode := "repository"
	if directory {
		mode = "directory"
	}
	dir := filepath.Join(config.SDCARDRoot, ".fbui")
	err := os.MkdirAll(dir, 0o755)
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, "game-mode"), []byte(mode+"\n"), 0o644)
	}
	if err != nil {
		w.app.ShowAlertDialog(w.doc, fbiw.AlertDialogOptions{
			Title: "切换游戏模式失败", Description: err.Error(),
		})
		return
	}
	w.directoryGames = directory
	w.toolsNav.tools[0].name = gameModeToolName(directory)
	// 重新绑定列表以立即刷新当前模式文字。
	w.toolsNav.scroll.SetItems(len(w.toolsNav.tools),
		func() (fbiw.Box, *_ToolItemView) {
			view := w.doc.Instantiate[_ToolItemView]("tool-item")
			return view.root, view
		},
		func(view *_ToolItemView, index int) {
			view.name.SetText(w.toolsNav.tools[index].name)
		},
	)
	w.toolsNav.activate()
}
