package swap_manager

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

const (
	ReserveBytes = int64(128 << 20)
	manifestName = "swap.json"
)

var Sizes = []int64{512 << 20, 1 << 30, 2 << 30}
var restoreErrors sync.Map
var inactiveEntries sync.Map

type Entry struct {
	Path      string
	Type      string
	SizeBytes int64
	UsedBytes int64
	Priority  int
	Active    bool
	Managed   bool
	Regular   bool
	Error     string
}

func (e Entry) Usage() float64 {
	if e.SizeBytes <= 0 {
		return 0
	}
	return float64(e.UsedBytes) / float64(e.SizeBytes)
}

type Runner interface {
	Run(name string, args ...string) error
	LookPath(name string) error
}

type execRunner struct{}

func (execRunner) Run(name string, args ...string) error {
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		if message := strings.TrimSpace(string(output)); message != "" {
			return fmt.Errorf("%s: %w", message, err)
		}
	}
	return err
}
func (execRunner) LookPath(name string) error {
	_, err := exec.LookPath(name)
	return err
}

type Backend struct {
	SDRoot       string
	ProcPath     string
	ManifestPath string
	Runner       Runner
}

func NewBackend(sdRoot string) *Backend {
	manifestPath := manifestName
	if executable, err := os.Executable(); err == nil {
		manifestPath = filepath.Join(filepath.Dir(executable), manifestName)
	}
	return &Backend{
		SDRoot:       sdRoot,
		ProcPath:     "/proc/swaps",
		ManifestPath: manifestPath,
		Runner:       execRunner{},
	}
}

func Parse(r io.Reader) ([]Entry, error) {
	scanner := bufio.NewScanner(r)
	first := true
	var entries []Entry
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if first {
			first = false
			if strings.HasPrefix(line, "Filename") {
				continue
			}
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			return nil, fmt.Errorf("无法解析 /proc/swaps 行：%q", line)
		}
		size, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("解析 Swap 大小：%w", err)
		}
		used, err := strconv.ParseInt(fields[3], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("解析 Swap 用量：%w", err)
		}
		priority, err := strconv.Atoi(fields[4])
		if err != nil {
			return nil, fmt.Errorf("解析 Swap 优先级：%w", err)
		}
		entries = append(entries, Entry{Path: fields[0], Type: fields[1], SizeBytes: size << 10, UsedBytes: used << 10, Priority: priority, Active: true})
	}
	return entries, scanner.Err()
}

func FormatBytes(bytes int64) string {
	const unit = int64(1024)
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < unit*unit {
		return fmt.Sprintf("%.f KiB", float64(bytes)/float64(unit))
	}
	if bytes < unit*unit*unit {
		return fmt.Sprintf("%.f MiB", float64(bytes)/float64(unit*unit))
	}
	return fmt.Sprintf("%.f GiB", float64(bytes)/float64(unit*unit*unit))
}

type manifestEntry struct {
	Path    string `json:"path"`
	Enabled bool   `json:"enabled"`
}

func (b *Backend) loadManifest() ([]manifestEntry, error) {
	data, err := os.ReadFile(b.ManifestPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []manifestEntry
	if err := json.Unmarshal(data, &entries); err == nil {
		return entries, nil
	}
	var legacy []string
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, fmt.Errorf("Swap 清单损坏：%w", err)
	}
	entries = nil
	for _, path := range legacy {
		entries = append(entries, manifestEntry{Path: path, Enabled: true})
	}
	return entries, nil
}

func (b *Backend) saveManifest(entries []manifestEntry) error {
	dir := filepath.Dir(b.ManifestPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	tmp := b.ManifestPath + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, b.ManifestPath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (b *Backend) List() ([]Entry, error) {
	file, err := os.Open(b.ProcPath)
	if err != nil {
		return nil, err
	}
	entries, err := Parse(file)
	_ = file.Close()
	if err != nil {
		return nil, err
	}
	managed, manifestErr := b.loadManifest()
	active := make(map[string]int, len(entries))
	for index := range entries {
		active[entries[index].Path] = index
		restoreErrors.Delete(entries[index].Path)
		inactiveEntries.Delete(entries[index].Path)
		entries[index].Managed = slices.ContainsFunc(managed, func(item manifestEntry) bool { return item.Path == entries[index].Path })
		info, statErr := os.Stat(entries[index].Path)
		entries[index].Regular = statErr == nil && info.Mode().IsRegular()
	}
	for _, item := range managed {
		path := item.Path
		if _, ok := active[path]; ok {
			continue
		}
		entry := Entry{Path: path, Type: "file", Managed: true}
		info, statErr := os.Stat(path)
		if statErr != nil {
			entry.Error = "文件不存在"
		} else {
			entry.Regular = info.Mode().IsRegular()
			entry.SizeBytes = info.Size()
		}
		if restoreErr, ok := restoreErrors.Load(path); ok {
			entry.Error = restoreErr.(string)
		}
		entries = append(entries, entry)
	}
	inactiveEntries.Range(func(key, value any) bool {
		path := key.(string)
		if _, err := os.Stat(path); err != nil {
			inactiveEntries.Delete(path)
			return true
		}
		if _, ok := active[path]; !ok && !slices.ContainsFunc(managed, func(item manifestEntry) bool { return item.Path == path }) {
			entries = append(entries, value.(Entry))
		}
		return true
	})
	return entries, manifestErr
}

func (b *Backend) CheckTools(names ...string) error {
	for _, name := range names {
		if err := b.Runner.LookPath(name); err != nil {
			return fmt.Errorf("缺少系统命令 %s", name)
		}
	}
	return nil
}

func (b *Backend) nextPath() string {
	for index := 1; ; index++ {
		name := "swapfile"
		if index > 1 {
			name = fmt.Sprintf("swapfile-%d", index)
		}
		path := filepath.Join(b.SDRoot, name)
		if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
			return path
		}
	}
}

func availableBytes(path string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}

func writeZeros(path string, size int64, progress func(p float32)) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if progress == nil {
		progress = func(p float32) {}
	}
	buffer := make([]byte, 4<<20)
	remaining := size
	progress(0)
	for remaining > 0 {
		chunk := min(int64(len(buffer)), remaining)
		if _, err := file.Write(buffer[:chunk]); err != nil {
			_ = file.Close()
			return err
		}
		remaining -= chunk
		progress(float32(size-remaining) / float32(size) * 100)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func (b *Backend) Create(size int64, progress func(p float32)) (string, error) {
	if !slices.Contains(Sizes, size) {
		return "", fmt.Errorf("不支持的 Swap 大小：%d", size)
	}
	if err := b.CheckTools("mkswap", "swapon"); err != nil {
		return "", err
	}
	available, err := availableBytes(b.SDRoot)
	if err != nil {
		return "", fmt.Errorf("读取 SD 卡剩余空间：%w", err)
	}
	if available < size+ReserveBytes {
		return "", fmt.Errorf("空间不足：创建后至少需要保留 %s", FormatBytes(ReserveBytes))
	}
	path := b.nextPath()
	tmp := path + ".tmp"
	if err := writeZeros(tmp, size, progress); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("分配 Swap 文件：%w", err)
	}
	defer os.Remove(tmp)
	_ = os.Chmod(tmp, 0600)
	if err := b.Runner.Run("mkswap", tmp); err != nil {
		return "", fmt.Errorf("格式化 Swap：%w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", fmt.Errorf("保存 Swap 文件：%w", err)
	}
	if err := b.Runner.Run("swapon", path); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("启用 Swap：%w", err)
	}
	managed, manifestErr := b.loadManifest()
	if manifestErr != nil {
		managed = nil
	}
	if !slices.ContainsFunc(managed, func(item manifestEntry) bool { return item.Path == path }) {
		managed = append(managed, manifestEntry{Path: path, Enabled: true})
	}
	if err := b.saveManifest(managed); err != nil {
		_ = b.Runner.Run("swapoff", path)
		_ = os.Remove(path)
		return "", fmt.Errorf("保存 Swap 清单：%w", err)
	}
	return path, nil
}

func (b *Backend) Delete(entry Entry) error {
	info, err := os.Stat(entry.Path)
	if errors.Is(err, fs.ErrNotExist) && entry.Managed {
		managed, manifestErr := b.loadManifest()
		if manifestErr != nil {
			return manifestErr
		}
		return b.saveManifest(slices.DeleteFunc(managed, func(item manifestEntry) bool { return item.Path == entry.Path }))
	}
	if err != nil {
		return fmt.Errorf("读取 Swap 文件：%w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("只能删除普通文件型 Swap")
	}
	if entry.Active {
		if err := b.CheckTools("swapoff"); err != nil {
			return err
		}
		if err := b.Runner.Run("swapoff", entry.Path); err != nil {
			return fmt.Errorf("停用 Swap：%w", err)
		}
	}
	if err := os.Remove(entry.Path); err != nil {
		if entry.Active {
			_ = b.Runner.Run("swapon", entry.Path)
		}
		return fmt.Errorf("删除 Swap 文件：%w", err)
	}
	inactiveEntries.Delete(entry.Path)
	managed, manifestErr := b.loadManifest()
	if manifestErr == nil && slices.ContainsFunc(managed, func(item manifestEntry) bool { return item.Path == entry.Path }) {
		if err := b.saveManifest(slices.DeleteFunc(managed, func(item manifestEntry) bool { return item.Path == entry.Path })); err != nil {
			return fmt.Errorf("更新 Swap 清单：%w", err)
		}
	}
	return nil
}

func (b *Backend) SetActive(entry Entry, active bool) error {
	if entry.Active == active {
		return nil
	}
	var managed []manifestEntry
	managedIndex := -1
	if entry.Managed {
		var err error
		managed, err = b.loadManifest()
		if err != nil {
			return err
		}
		managedIndex = slices.IndexFunc(managed, func(item manifestEntry) bool { return item.Path == entry.Path })
	}
	command := "swapon"
	if !active {
		command = "swapoff"
	}
	if err := b.CheckTools(command); err != nil {
		return err
	}
	if err := b.Runner.Run(command, entry.Path); err != nil {
		action := "启用"
		if !active {
			action = "停用"
		}
		return fmt.Errorf("%s Swap：%w", action, err)
	}
	if managedIndex >= 0 {
		managed[managedIndex].Enabled = active
		if err := b.saveManifest(managed); err != nil {
			rollback := "swapoff"
			if !active {
				rollback = "swapon"
			}
			_ = b.Runner.Run(rollback, entry.Path)
			return fmt.Errorf("保存 Swap 状态：%w", err)
		}
	}
	if active {
		inactiveEntries.Delete(entry.Path)
	} else {
		entry.Active = false
		entry.UsedBytes = 0
		inactiveEntries.Store(entry.Path, entry)
	}
	return nil
}

func (b *Backend) Restore() []error {
	managed, err := b.loadManifest()
	if err != nil {
		return []error{err}
	}
	file, err := os.Open(b.ProcPath)
	if err != nil {
		return []error{err}
	}
	entries, parseErr := Parse(file)
	_ = file.Close()
	if parseErr != nil {
		return []error{parseErr}
	}
	active := map[string]bool{}
	for _, entry := range entries {
		active[entry.Path] = true
	}
	kept := managed[:0]
	var errs []error
	for _, item := range managed {
		info, statErr := os.Stat(item.Path)
		if statErr != nil || !info.Mode().IsRegular() {
			continue
		}
		kept = append(kept, item)
		if !item.Enabled || active[item.Path] {
			continue
		}
		if err := b.Runner.Run("swapon", item.Path); err != nil {
			restoreErr := fmt.Errorf("恢复 %s：%w", item.Path, err)
			restoreErrors.Store(item.Path, "恢复失败："+err.Error())
			errs = append(errs, restoreErr)
		} else {
			restoreErrors.Delete(item.Path)
		}
	}
	if len(kept) != len(managed) {
		if err := b.saveManifest(kept); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}
