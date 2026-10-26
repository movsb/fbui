package mainui

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
)

const (
	ROMType        = 23
	AppType        = 3
	MaxRecentItems = 20
)

type record struct {
	Fields map[string]json.RawMessage
}

func newRecord() *record { return &record{Fields: map[string]json.RawMessage{}} }

func (r *record) stringValue(name string) string {
	var value string
	_ = json.Unmarshal(r.Fields[name], &value)
	return value
}

func (r *record) intValue(name string) int {
	var value int
	_ = json.Unmarshal(r.Fields[name], &value)
	return value
}

func (r *record) set(name string, value any) {
	raw, _ := json.Marshal(value)
	r.Fields[name] = raw
}

func (r *record) label() string     { return r.stringValue("label") }
func (r *record) launch() string    { return r.stringValue("launch") }
func (r *record) romPath() string   { return r.stringValue("rompath") }
func (r *record) imagePath() string { return r.stringValue("imgpath") }
func (r *record) subLabel() string  { return r.stringValue("sublabel") }
func (r *record) order() int        { return r.intValue("order") }
func (r *record) itemType() int     { return r.intValue("type") }

type mainUIRecordLine struct {
	record *record
	raw    []byte
}

func readMainUIRecords(path string) ([]mainUIRecordLine, error) {
	fp, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer fp.Close()

	var lines []mainUIRecordLine
	scanner := bufio.NewScanner(fp)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		raw := bytes.Clone(scanner.Bytes())
		if len(bytes.TrimSpace(raw)) == 0 {
			lines = append(lines, mainUIRecordLine{raw: raw})
			continue
		}
		var fields map[string]json.RawMessage
		// MainUI has been observed writing invalid UTF-8 in imgpath. Replacing
		// invalid bytes lets the rest of that record remain usable.
		clean := []byte(strings.ToValidUTF8(string(raw), "�"))
		if json.Unmarshal(clean, &fields) != nil || fields == nil {
			lines = append(lines, mainUIRecordLine{raw: raw})
			continue
		}
		lines = append(lines, mainUIRecordLine{record: &record{Fields: fields}, raw: raw})
	}
	return lines, scanner.Err()
}

func encodeMainUIRecords(lines []mainUIRecordLine) ([]byte, error) {
	var out bytes.Buffer
	for _, line := range lines {
		var raw []byte
		var err error
		if line.record == nil {
			raw = line.raw
		} else {
			raw, err = json.Marshal(line.record.Fields)
			if err != nil {
				return nil, err
			}
		}
		out.Write(raw)
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".fbui-mainui-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err = tmp.Chmod(0o666); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

type Item struct {
	Label     string
	SubLabel  string
	Launch    string
	ROMPath   string
	ImagePath string
	Type      int
}

func (i Item) identity() string {
	launch := filepath.Clean(i.Launch)
	if i.ROMPath == "" {
		return "app\x00" + launch
	}
	return "rom\x00" + launch + "\x00" + filepath.Clean(i.ROMPath)
}

func (i Item) recentIdentity() string {
	if i.ROMPath != "" {
		return "rom\x00" + filepath.Clean(i.ROMPath)
	}
	return "app\x00" + filepath.Clean(i.Launch)
}

func (i Item) record() *record {
	r := newRecord()
	r.set("label", i.Label)
	r.set("launch", i.Launch)
	typeValue := i.Type
	if typeValue == 0 {
		typeValue = AppType
		if i.ROMPath != "" {
			typeValue = ROMType
		}
	}
	r.set("type", typeValue)
	if i.ROMPath != "" {
		r.set("rompath", i.ROMPath)
		r.set("imgpath", i.ImagePath)
	}
	if i.SubLabel != "" {
		r.set("sublabel", i.SubLabel)
	}
	return r
}

func itemFromRecord(r *record) Item {
	return Item{
		Label: r.label(), SubLabel: r.subLabel(), Launch: r.launch(),
		ROMPath: r.romPath(), ImagePath: r.imagePath(), Type: r.itemType(),
	}
}

func (r *record) managed() bool { return r.label() != "" && r.launch() != "" }

func (i Item) available() bool { return i.ValidateFiles() == nil }

// ValidateFiles also checks files immediately before launching, since a ROM can
// be removed after the list was displayed.
func (i Item) ValidateFiles() error {
	if err := i.Validate(); err != nil {
		return err
	}
	for _, path := range []string{i.Launch, i.ROMPath} {
		if path == "" {
			continue
		}
		info, err := os.Stat(filepath.Clean(path))
		if err != nil {
			return fmt.Errorf("启动文件不可用：%s：%w", path, err)
		}
		if info.IsDir() {
			return fmt.Errorf("启动文件是目录：%s", path)
		}
	}
	return nil
}

type Lists struct {
	mu        sync.Mutex
	favorites string
	recent    string
}

func NewLists(sdcardRoot string) *Lists {
	root := filepath.Join(sdcardRoot, "Roms")
	return &Lists{
		favorites: filepath.Join(root, "favourite2.json"),
		recent:    filepath.Join(root, "recentlist.json"),
	}
}

func recordIdentity(r *record) string { return itemFromRecord(r).identity() }

func (s *Lists) IsFavorite(item Item) (bool, error) {
	if err := item.Validate(); err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lines, err := readMainUIRecords(s.favorites)
	if err != nil {
		return false, err
	}
	for _, line := range lines {
		if line.record != nil && recordIdentity(line.record) == item.identity() {
			return true, nil
		}
	}
	return false, nil
}

func (s *Lists) ToggleFavorite(item Item) (bool, error) {
	if err := item.Validate(); err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lines, err := readMainUIRecords(s.favorites)
	if err != nil {
		return false, err
	}
	id := item.identity()
	removed := false
	out := make([]mainUIRecordLine, 0, len(lines)+1)
	for _, line := range lines {
		if line.record != nil && recordIdentity(line.record) == id {
			removed = true
			continue
		}
		out = append(out, line)
	}
	if !removed {
		out = append([]mainUIRecordLine{{record: item.record()}}, out...)
	}
	order := 1
	for _, line := range out {
		if line.record != nil && line.record.managed() {
			line.record.set("order", order)
			order++
		}
	}
	if err := s.writeFavorites(out); err != nil {
		return false, err
	}
	return !removed, nil
}

func (s *Lists) writeFavorites(lines []mainUIRecordLine) error {
	data, err := encodeMainUIRecords(lines)
	if err != nil {
		return err
	}
	if err := writeAtomic(s.favorites, data); err != nil {
		return err
	}
	return writeAtomic(s.favorites+".bak", data)
}

func (s *Lists) AddRecent(item Item) error {
	if err := item.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lines, err := readMainUIRecords(s.recent)
	if err != nil {
		return err
	}
	id := item.recentIdentity()
	out := []mainUIRecordLine{{record: item.record()}}
	valid := 1
	for _, line := range lines {
		if line.record == nil {
			out = append(out, line)
			continue
		}
		if !line.record.managed() {
			out = append(out, line)
			continue
		}
		if itemFromRecord(line.record).recentIdentity() == id {
			continue
		}
		if valid >= MaxRecentItems {
			continue
		}
		out = append(out, line)
		valid++
	}
	data, err := encodeMainUIRecords(out)
	if err != nil {
		return err
	}
	return writeAtomic(s.recent, data)
}

func (s *Lists) Items(favorites bool) ([]Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.recent
	if favorites {
		path = s.favorites
	}
	lines, err := readMainUIRecords(path)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(lines))
	orders := make(map[string]int, len(lines))
	seenRecent := make(map[string]bool, len(lines))
	for _, line := range lines {
		if line.record == nil {
			continue
		}
		item := itemFromRecord(line.record)
		if !item.available() {
			continue
		}
		if !favorites {
			id := item.recentIdentity()
			if seenRecent[id] {
				continue
			}
			seenRecent[id] = true
		}
		items = append(items, item)
		orders[item.identity()] = line.record.order()
	}
	if favorites {
		slices.SortStableFunc(items, func(a, b Item) int {
			ao, bo := orders[a.identity()], orders[b.identity()]
			if ao <= 0 {
				ao = int(^uint(0) >> 1)
			}
			if bo <= 0 {
				bo = int(^uint(0) >> 1)
			}
			return ao - bo
		})
	}
	return items, nil
}

func (s *Lists) Clear(favorites bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.recent
	if favorites {
		path = s.favorites
	}
	lines, err := readMainUIRecords(path)
	if err != nil {
		return err
	}
	out := lines[:0]
	for _, line := range lines {
		if line.record == nil || !line.record.managed() {
			out = append(out, line)
		}
	}
	if favorites {
		return s.writeFavorites(out)
	}
	data, err := encodeMainUIRecords(out)
	if err != nil {
		return err
	}
	return writeAtomic(path, data)
}

func (i Item) String() string {
	if i.ROMPath == "" {
		return i.Label + " (" + i.Launch + ")"
	}
	return i.Label + " (" + i.Launch + " " + strconv.Quote(i.ROMPath) + ")"
}

func (i Item) Validate() error {
	if i.Label == "" || i.Launch == "" {
		return fmt.Errorf("启动项目缺少名称或启动程序")
	}
	return nil
}
