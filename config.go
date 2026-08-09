package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/movsb/fbiw"
)

// 游戏模拟器和其它所有应用都使用的配置写法。
type Config struct {
	// 应用/模拟器名字。
	// 不是都有，取第一个不为空的。
	Label        string `json:"label"`
	LabelChinese string `json:"label.ch.lang"`

	// 图标。看起来是主要的，比 Icon 更常见。
	IconTop string `json:"icontop"`

	// 不知道这个是什么，很多是空的。
	Icon string `json:"icon"`

	Background string `json:"background"`

	// 一般是类似 `launch.sh` 的写法，
	// 所以是相对于此配置文件。
	Launch string `json:"launch"`

	// 一般来说，是类似于 ../../Roms/FC 之类的写法。
	// 所以是相对于此配置文件。
	RomPath string `json:"rompath"`
}

// 解析单个配置文件。
func parseConfig(path string) (*Config, error) {
	fp, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fp.Close()

	var config Config
	if err := json.NewDecoder(fp).Decode(&config); err != nil {
		return nil, fmt.Errorf(`解析失败：%w`, err)
	}

	return &config, nil
}

type LaunchConfig struct {
	// 此 Config 所在的目录。
	// 基于被解析的目录指定。
	// 可能是绝对目录，也可能是相对目录。
	// 如果是相对目录，是相对于此进程的启动目录。
	Dir    string
	Config *Config
}

// Rom所在目录，可直接使用的最终目录。
func (c *LaunchConfig) RomDir() string {
	if filepath.IsAbs(c.Config.RomPath) {
		return c.Config.RomPath
	}
	return filepath.Join(c.Dir, c.Config.RomPath)
}
func (c *LaunchConfig) LauncherScriptPath() string {
	if filepath.IsAbs(c.Config.Launch) {
		return c.Config.Launch
	}
	return filepath.Join(c.Dir, c.Config.Launch)
}
func (c *LaunchConfig) IconPath() string {
	return filepath.Join(c.Dir, c.Config.IconTop)
}
func (c *LaunchConfig) Name() string {
	if name := c.Config.LabelChinese; name != `` {
		return name
	}
	return c.Config.Label
}

// `sdcard` 是我开发机的软连接目录。
var _SDCARDRoot = fbiw.Iif(runtime.GOOS == `linux`, `/mnt/SDCARD`, `sdcard`)

func loadDir(dir string) []*LaunchConfig {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Println(err)
		return nil
	}

	configs := []*LaunchConfig{}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		jsonPath := filepath.Join(dir, entry.Name(), `config.json`)
		config, err := parseConfig(jsonPath)
		if err != nil {
			if !os.IsNotExist(err) {
				log.Printf(`解析配置出错: %s: %v`, jsonPath, err)
			}
			continue
		}

		if config.Launch == `` {
			log.Println(`启动脚本为空，忽略:`, jsonPath)
			continue
		}

		lc := LaunchConfig{
			Dir:    filepath.Join(dir, entry.Name()),
			Config: config,
		}

		configs = append(configs, &lc)
	}

	return configs
}
