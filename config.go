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

type Config struct {
	Label        string `json:"label"`
	LabelChinese string `json:"label.ch.lang"`
	IconTop      string `json:"icontop"`
	Icon         string `json:"icon"`
	Background   string `json:"background"`
	Launch       string `json:"launch"`
	RomPath      string `json:"rompath"`
}

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
	Dir    string
	Config *Config
}

var _SDCARDRoot = fbiw.Iif(runtime.GOOS == `linux`, `/mnt/SDCARD`, `sdcard`)

func loadDir(dir string) []*LaunchConfig {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Println(err)
		return nil
	}

	configs := []*LaunchConfig{}

	for _, entry := range entries {
		jsonPath := filepath.Join(dir, entry.Name(), `config.json`)
		config, err := parseConfig(jsonPath)
		if err != nil {
			log.Println(err)
			continue
		}

		if config.Launch == `` {
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

func LoadApps() []*LaunchConfig {
	launchConfigs := []*LaunchConfig{}

	launchConfigs = append(launchConfigs, loadDir(filepath.Join(_SDCARDRoot, `Apps`))...)
	launchConfigs = append(launchConfigs, loadDir(`/usr/trimui/apps`)...)

	return launchConfigs
}

func LoadEmus() []*LaunchConfig {
	launchConfigs := []*LaunchConfig{}

	launchConfigs = append(launchConfigs, loadDir(filepath.Join(_SDCARDRoot, `Emus`))...)
	// launchConfigs = append(launchConfigs, loadDir(`/usr/trimui/apps`)...)

	return launchConfigs
}
