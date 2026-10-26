package mainui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func testLists(t *testing.T) (*Lists, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Roms"), 0o755); err != nil {
		t.Fatal(err)
	}
	return NewLists(root), root
}

func testFile(t *testing.T, root, name string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("test"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadRecordsToleratesBadLinesAndInvalidUTF8(t *testing.T) {
	lists, root := testLists(t)
	launch := testFile(t, root, "Apps/demo/launch.sh")
	path := filepath.Join(root, "Roms", "recentlist.json")
	data := append([]byte("not-json\n"), []byte(fmt.Sprintf("{\"label\":\"Demo\",\"launch\":%q,\"imgpath\":\"", launch))...)
	data = append(data, 0xff)
	data = append(data, []byte("\",\"type\":3,\"future\":{\"enabled\":true}}\n")...)
	if err := os.WriteFile(path, data, 0o666); err != nil {
		t.Fatal(err)
	}

	items, err := lists.Items(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Label != "Demo" {
		t.Fatalf("unexpected items: %#v", items)
	}
	if err := lists.AddRecent(Item{Label: "Other", Launch: testFile(t, root, "Apps/other/launch.sh")}); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(written, []byte("not-json\n")) || !bytes.Contains(written, []byte(`"future":{"enabled":true}`)) {
		t.Fatalf("bad line or unknown field was lost: %q", written)
	}
}

func TestFavoriteToggleWritesBackupAndRenumbers(t *testing.T) {
	lists, root := testLists(t)
	launch := testFile(t, root, "Emus/FC/launch.sh")
	romA := testFile(t, root, "Roms/FC/a.nes")
	romB := testFile(t, root, "Roms/FC/b.nes")
	a := Item{Label: "A", Launch: launch, ROMPath: romA, SubLabel: "FC"}
	b := Item{Label: "B", Launch: launch, ROMPath: romB, SubLabel: "FC"}

	for _, item := range []Item{a, b} {
		added, err := lists.ToggleFavorite(item)
		if err != nil || !added {
			t.Fatalf("add favorite: added=%v err=%v", added, err)
		}
	}
	items, err := lists.Items(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Label != "B" || items[1].Label != "A" {
		t.Fatalf("newest favorite should be first: %#v", items)
	}
	removed, err := lists.ToggleFavorite(a)
	if err != nil || removed {
		t.Fatalf("remove favorite: added=%v err=%v", removed, err)
	}

	mainData, err := os.ReadFile(lists.favorites)
	if err != nil {
		t.Fatal(err)
	}
	backupData, err := os.ReadFile(lists.favorites + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(mainData, backupData) {
		t.Fatal("favorite backup differs from main file")
	}
	lines, err := readMainUIRecords(lists.favorites)
	if err != nil || len(lines) != 1 || lines[0].record.order() != 1 || lines[0].record.itemType() != ROMType {
		t.Fatalf("unexpected favorite records: %#v err=%v", lines, err)
	}
}

func TestRecentDeduplicatesAndLimitsAllKinds(t *testing.T) {
	lists, root := testLists(t)
	for i := 0; i < MaxRecentItems+5; i++ {
		item := Item{Label: fmt.Sprintf("App %02d", i), Launch: testFile(t, root, fmt.Sprintf("Apps/%02d/launch.sh", i))}
		if err := lists.AddRecent(item); err != nil {
			t.Fatal(err)
		}
	}
	items, err := lists.Items(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != MaxRecentItems || items[0].Label != "App 24" || items[len(items)-1].Label != "App 05" {
		t.Fatalf("unexpected recent order/limit: %#v", items)
	}
	if err := lists.AddRecent(items[len(items)-1]); err != nil {
		t.Fatal(err)
	}
	items, _ = lists.Items(false)
	if len(items) != MaxRecentItems || items[0].Label != "App 05" {
		t.Fatalf("deduplicated item was not moved to front: %#v", items)
	}
}

func TestRecentDeduplicatesROMAcrossLaunchers(t *testing.T) {
	lists, root := testLists(t)
	launchA := testFile(t, root, "Emus/FC/launch.sh")
	launchB := testFile(t, root, "Emus/FC/launch-alt.sh")
	rom := testFile(t, root, "Roms/FC/game.nes")
	old := Item{Label: "Old", Launch: launchA, ROMPath: rom}
	newest := Item{Label: "Newest", Launch: launchB, ROMPath: filepath.Join(filepath.Dir(rom), ".", filepath.Base(rom))}
	if err := lists.AddRecent(old); err != nil {
		t.Fatal(err)
	}
	if err := lists.AddRecent(newest); err != nil {
		t.Fatal(err)
	}
	items, err := lists.Items(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Label != "Newest" || items[0].Launch != launchB {
		t.Fatalf("same ROM should keep only its newest record: %#v", items)
	}
}

func TestRecentReadHidesExistingDuplicates(t *testing.T) {
	lists, root := testLists(t)
	launch := testFile(t, root, "Apps/demo/launch.sh")
	lines := []mainUIRecordLine{
		{record: Item{Label: "Newest", Launch: launch}.record()},
		{record: Item{Label: "Old duplicate", Launch: filepath.Join(filepath.Dir(launch), ".", filepath.Base(launch))}.record()},
	}
	data, err := encodeMainUIRecords(lines)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lists.recent, data, 0o666); err != nil {
		t.Fatal(err)
	}
	items, err := lists.Items(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Label != "Newest" {
		t.Fatalf("existing duplicate should be hidden: %#v", items)
	}
}

func TestIdentityUsesCleanPathsAndDistinguishesROMs(t *testing.T) {
	a := Item{Launch: "/a/emu/../launch.sh", ROMPath: "/roms/./a.zip"}
	b := Item{Launch: "/a/launch.sh", ROMPath: "/roms/a.zip"}
	c := Item{Launch: "/a/launch.sh", ROMPath: "/roms/b.zip"}
	if a.identity() != b.identity() {
		t.Fatal("equivalent paths should have the same identity")
	}
	if b.identity() == c.identity() {
		t.Fatal("different ROMs should have different identities")
	}
}

func TestClearPreservesMalformedLines(t *testing.T) {
	lists, root := testLists(t)
	launch := testFile(t, root, "Apps/demo/launch.sh")
	r := Item{Label: "Demo", Launch: launch}.record()
	r.set("unknown", json.RawMessage(`{"x":1}`))
	data, err := encodeMainUIRecords([]mainUIRecordLine{{raw: []byte("broken")}, {record: r}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lists.recent, data, 0o666); err != nil {
		t.Fatal(err)
	}
	if err := lists.Clear(false); err != nil {
		t.Fatal(err)
	}
	written, _ := os.ReadFile(lists.recent)
	if string(written) != "broken\n" {
		t.Fatalf("unexpected clear output: %q", written)
	}
}

func TestStoreROMUsesOnlyNativeFieldsAndLocalPaths(t *testing.T) {
	lists, root := testLists(t)
	rom := Item{
		Label: "游戏 / 日版 / game.zip", SubLabel: "FC", Type: ROMType,
		Launch:  testFile(t, root, "Emus/FC/launch.sh"),
		ROMPath: testFile(t, root, ".fbui/gm/assets/42/game.zip"),
	}
	if added, err := lists.ToggleFavorite(rom); err != nil || !added {
		t.Fatalf("favorite prepared ROM: added=%v err=%v", added, err)
	}
	if err := lists.AddRecent(rom); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(lists.favorites)
	if err != nil {
		t.Fatal(err)
	}
	var native map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(data), &native); err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"label": true, "sublabel": true, "launch": true, "rompath": true, "imgpath": true, "type": true, "order": true}
	for key := range native {
		if !allowed[key] {
			t.Fatalf("unexpected extra field: %s", key)
		}
	}
	renamed := rom
	renamed.Label = "新游戏名 / 日版 / game.zip"
	if err := lists.AddRecent(renamed); err != nil {
		t.Fatal(err)
	}
	if favorite, err := lists.IsFavorite(renamed); err != nil || !favorite {
		t.Fatalf("favorite should match by local paths: %v %v", favorite, err)
	}
	lists = NewLists(root)
	if recent, err := lists.Items(false); err != nil || len(recent) != 1 || recent[0].Label != renamed.Label || recent[0].ROMPath != rom.ROMPath || recent[0].Launch != rom.Launch {
		t.Fatalf("native launch fields should round trip: %#v %v", recent, err)
	}
	if err := os.Remove(rom.ROMPath); err != nil {
		t.Fatal(err)
	}
	if err := rom.ValidateFiles(); err == nil {
		t.Fatal("missing ROM must not launch")
	}
	for _, favorites := range []bool{true, false} {
		if items, err := lists.Items(favorites); err != nil || len(items) != 0 {
			t.Fatalf("missing ROM should be filtered without recovery: %#v %v", items, err)
		}
	}
}
