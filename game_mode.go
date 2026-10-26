package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbui/pkg/config"
)

type gameMode string

const (
	gameModePlatforms gameMode = "platforms"
	gameModeSeries    gameMode = "series"
	gameModeDirectory gameMode = "directory"
)

func loadGameMode(root string) gameMode {
	data, err := os.ReadFile(filepath.Join(root, ".fbui", "game-mode"))
	if err != nil && !os.IsNotExist(err) {
		log.Printf("读取游戏模式失败：%v", err)
	}
	switch mode := gameMode(strings.TrimSpace(string(data))); mode {
	case gameModeSeries, gameModeDirectory:
		return mode
	default:
		return gameModePlatforms
	}
}

func (w *MainWindow) toggleGameMode() bool {
	if w.storeNav.busy {
		return false
	}
	mode := gameModePlatforms
	switch w.gameMode {
	case gameModePlatforms:
		mode = gameModeSeries
	case gameModeSeries:
		mode = gameModeDirectory
	}
	dir := filepath.Join(config.SDCARDRoot, ".fbui")
	err := os.MkdirAll(dir, 0o755)
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, "game-mode"), []byte(string(mode)+"\n"), 0o644)
	}
	if err != nil {
		w.app.ShowAlertDialog(w.doc, fbiw.AlertDialogOptions{
			Title: "切换游戏模式失败", Description: err.Error(),
		})
		return false
	}
	w.gameMode = mode
	w.statusBarNav.setIndex(w.statusBarNav.catIndex)
	return true
}

// 仅在游戏标题获得焦点时切换页面。
func (w *MainWindow) switchGameModeFromGames() {
	if !w.toggleGameMode() {
		return
	}
	w.storeNav.active = false
	w.storeNav.activeList().Deselect()
	w.gamesNav.emus.Deselect()
	w.statusBarNav.activate()
}
