package power_supply

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const DefaultRoot = "/sys/class/power_supply"

type Supply struct {
	Name   string
	Path   string
	Type   string
	Values map[string]string
	Error  string
}

func (s Supply) Value(name string) (string, bool) {
	value, ok := s.Values["POWER_SUPPLY_"+strings.ToUpper(name)]
	return value, ok
}

type Reader struct {
	Root string
}

func NewReader(root string) *Reader {
	if root == "" {
		root = DefaultRoot
	}
	return &Reader{Root: root}
}

func (r *Reader) List() ([]Supply, error) {
	root := r.Root
	if root == "" {
		root = DefaultRoot
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取电源设备目录：%w", err)
	}

	var supplies []Supply
	var readErrors []error
	for _, entry := range entries {
		if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		path := filepath.Join(root, entry.Name())
		supply := Supply{Name: entry.Name(), Path: path, Values: map[string]string{}}
		file, openErr := os.Open(filepath.Join(path, "uevent"))
		if openErr != nil {
			supply.Error = openErr.Error()
			readErrors = append(readErrors, fmt.Errorf("读取 %s：%w", entry.Name(), openErr))
		} else {
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				key, value, ok := strings.Cut(scanner.Text(), "=")
				if ok && key != "" {
					supply.Values[key] = value
				}
			}
			if scanErr := scanner.Err(); scanErr != nil {
				supply.Error = scanErr.Error()
				readErrors = append(readErrors, fmt.Errorf("读取 %s：%w", entry.Name(), scanErr))
			}
			_ = file.Close()
		}
		if name, ok := supply.Value("name"); ok && name != "" {
			supply.Name = name
		}
		supply.Type, _ = supply.Value("type")
		if supply.Type == "" {
			typeValue, typeErr := os.ReadFile(filepath.Join(path, "type"))
			if typeErr != nil {
				readErrors = append(
					readErrors,
					fmt.Errorf("读取 %s/type：%w", entry.Name(), typeErr),
				)
			} else {
				supply.Type = strings.TrimSpace(string(typeValue))
				if supply.Type != "" {
					supply.Values["POWER_SUPPLY_TYPE"] = supply.Type
				}
			}
		}
		supplies = append(supplies, supply)
	}

	slices.SortStableFunc(supplies, func(a, b Supply) int {
		aBattery, bBattery := strings.EqualFold(a.Type, "Battery"), strings.EqualFold(b.Type, "Battery")
		if aBattery != bBattery {
			if aBattery {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	return supplies, errors.Join(readErrors...)
}

func FirstBattery(supplies []Supply) (Supply, bool) {
	for _, supply := range supplies {
		if strings.EqualFold(supply.Type, "Battery") {
			return supply, true
		}
	}
	return Supply{}, false
}
