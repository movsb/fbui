package swap_manager

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type fakeRunner struct {
	missing map[string]bool
	fail    map[string]error
	calls   []string
}

func (r *fakeRunner) Run(name string, args ...string) error {
	r.calls = append(r.calls, strings.Join(append([]string{name}, args...), " "))
	return r.fail[name]
}
func (r *fakeRunner) LookPath(name string) error {
	if r.missing[name] {
		return errors.New("missing")
	}
	return nil
}

func TestParseAndFormat(t *testing.T) {
	entries, err := Parse(strings.NewReader("Filename Type Size Used Priority\n/mnt/SDCARD/swapfile file 1048576 262144 -2\n/dev/zram0 partition 512 0 5\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].SizeBytes != 1<<30 || entries[0].UsedBytes != 256<<20 || entries[0].Priority != -2 || !entries[0].Active {
		t.Fatalf("unexpected entries: %#v", entries)
	}
	if got := entries[0].Usage(); got != .25 {
		t.Fatalf("usage=%v", got)
	}
	for _, test := range []struct {
		bytes int64
		want  string
	}{
		{512, "512 B"},
		{1536, "1.5 KiB"},
		{3 << 20, "3.0 MiB"},
		{2 << 30, "2.0 GiB"},
		{1073741824, `1.0 GiB`},
	} {
		if got := FormatBytes(test.bytes); got != test.want {
			t.Errorf("FormatBytes(%d)=%q want %q", test.bytes, got, test.want)
		}
	}
}

func TestParseRejectsMalformedLine(t *testing.T) {
	if _, err := Parse(strings.NewReader("Filename Type Size Used Priority\nbad line\n")); err == nil {
		t.Fatal("expected error")
	}
}

func newTestBackend(t *testing.T, proc string, runner *fakeRunner) *Backend {
	t.Helper()
	root := t.TempDir()
	procPath := filepath.Join(root, "proc_swaps")
	if err := os.WriteFile(procPath, []byte(proc), 0600); err != nil {
		t.Fatal(err)
	}
	return &Backend{
		SDRoot:       root,
		ProcPath:     procPath,
		ManifestPath: filepath.Join(root, manifestName),
		Runner:       runner,
	}
}

func TestListMergesActiveAndManaged(t *testing.T) {
	runner := &fakeRunner{}
	b := newTestBackend(t, "Filename Type Size Used Priority\n/dev/zram0 partition 1024 12 5\n", runner)
	managed := filepath.Join(b.SDRoot, "swapfile")
	if err := os.WriteFile(managed, make([]byte, 4096), 0600); err != nil {
		t.Fatal(err)
	}
	if err := b.saveManifest([]manifestEntry{{Path: managed, Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	entries, err := b.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Path != "/dev/zram0" || !entries[0].Active || entries[0].Regular || entries[1].Path != managed || entries[1].Active || !entries[1].Managed || !entries[1].Regular {
		t.Fatalf("unexpected entries: %#v", entries)
	}
}

func TestCreateAndDelete(t *testing.T) {
	runner := &fakeRunner{fail: map[string]error{}}
	b := newTestBackend(t, "Filename Type Size Used Priority\n", runner)
	oldSizes := Sizes
	Sizes = []int64{1 << 20}
	t.Cleanup(func() { Sizes = oldSizes })
	path, err := b.Create(1<<20, nil)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() != 1<<20 {
		t.Fatalf("created file: info=%v err=%v", info, err)
	}
	managed, err := b.loadManifest()
	if err != nil || !reflect.DeepEqual(managed, []manifestEntry{{Path: path, Enabled: true}}) {
		t.Fatalf("manifest=%v err=%v", managed, err)
	}
	if len(runner.calls) < 2 || !strings.HasPrefix(runner.calls[0], "mkswap ") || runner.calls[1] != "swapon "+path {
		t.Fatalf("calls=%v", runner.calls)
	}
	if err := b.Delete(Entry{Path: path, Active: true, Regular: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file remains: %v", err)
	}
	managed, err = b.loadManifest()
	if err != nil || len(managed) != 0 {
		t.Fatalf("manifest=%v err=%v", managed, err)
	}
}

func TestCreateFailureCleansTemporaryFile(t *testing.T) {
	runner := &fakeRunner{fail: map[string]error{"mkswap": errors.New("boom")}}
	b := newTestBackend(t, "Filename Type Size Used Priority\n", runner)
	oldSizes := Sizes
	Sizes = []int64{4096}
	t.Cleanup(func() { Sizes = oldSizes })
	if _, err := b.Create(4096, nil); err == nil {
		t.Fatal("expected error")
	}
	if matches, _ := filepath.Glob(filepath.Join(b.SDRoot, "swapfile*")); len(matches) != 0 {
		t.Fatalf("leftovers: %v", matches)
	}
}

func TestRestoreDropsMissingAndReportsFailure(t *testing.T) {
	runner := &fakeRunner{fail: map[string]error{"swapon": errors.New("unsupported")}}
	b := newTestBackend(t, "Filename Type Size Used Priority\n", runner)
	existing := filepath.Join(b.SDRoot, "swapfile")
	missing := filepath.Join(b.SDRoot, "gone")
	if err := os.WriteFile(existing, []byte("swap"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := b.saveManifest([]manifestEntry{{Path: existing, Enabled: true}, {Path: missing, Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	errs := b.Restore()
	if len(errs) != 1 {
		t.Fatalf("errors=%v", errs)
	}
	managed, err := b.loadManifest()
	if err != nil || !reflect.DeepEqual(managed, []manifestEntry{{Path: existing, Enabled: true}}) {
		t.Fatalf("manifest=%v err=%v", managed, err)
	}
	entries, err := b.List()
	if err != nil || len(entries) != 1 || !strings.Contains(entries[0].Error, "恢复失败") {
		t.Fatalf("entries=%#v err=%v", entries, err)
	}
}

func TestDeleteMissingManagedEntryOnlyUpdatesManifest(t *testing.T) {
	b := newTestBackend(t, "Filename Type Size Used Priority\n", &fakeRunner{})
	missing := filepath.Join(b.SDRoot, "gone")
	if err := b.saveManifest([]manifestEntry{{Path: missing, Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	if err := b.Delete(Entry{Path: missing, Managed: true}); err != nil {
		t.Fatal(err)
	}
	managed, err := b.loadManifest()
	if err != nil || len(managed) != 0 {
		t.Fatalf("manifest=%v err=%v", managed, err)
	}
}

func TestSetActiveKeepsUnmanagedEntryVisibleWhileDisabled(t *testing.T) {
	runner := &fakeRunner{fail: map[string]error{}}
	b := newTestBackend(t, "Filename Type Size Used Priority\n", runner)
	path := filepath.Join(b.SDRoot, "external-swap")
	if err := os.WriteFile(path, []byte("swap"), 0600); err != nil {
		t.Fatal(err)
	}
	entry := Entry{Path: path, Type: "file", SizeBytes: 4096, UsedBytes: 1024, Active: true, Regular: true}
	if err := b.SetActive(entry, false); err != nil {
		t.Fatal(err)
	}
	entries, err := b.List()
	if err != nil || len(entries) != 1 || entries[0].Active || entries[0].Path != path {
		t.Fatalf("entries=%#v err=%v", entries, err)
	}
	if err := b.SetActive(entries[0], true); err != nil {
		t.Fatal(err)
	}
	entries, err = b.List()
	if err != nil || len(entries) != 0 {
		t.Fatalf("entries=%#v err=%v", entries, err)
	}
	wantCalls := []string{"swapoff " + path, "swapon " + path}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("calls=%v want %v", runner.calls, wantCalls)
	}
}

func TestDisabledManagedEntryStaysDisabledOnRestore(t *testing.T) {
	runner := &fakeRunner{fail: map[string]error{}}
	b := newTestBackend(t, "Filename Type Size Used Priority\n", runner)
	path := filepath.Join(b.SDRoot, "swapfile")
	if err := os.WriteFile(path, []byte("swap"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := b.saveManifest([]manifestEntry{{Path: path, Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	entry := Entry{Path: path, Type: "file", Active: true, Managed: true, Regular: true}
	if err := b.SetActive(entry, false); err != nil {
		t.Fatal(err)
	}
	managed, err := b.loadManifest()
	if err != nil || len(managed) != 1 || managed[0].Enabled {
		t.Fatalf("manifest=%#v err=%v", managed, err)
	}
	if errs := b.Restore(); len(errs) != 0 {
		t.Fatalf("restore errors=%v", errs)
	}
	if !reflect.DeepEqual(runner.calls, []string{"swapoff " + path}) {
		t.Fatalf("calls=%v", runner.calls)
	}
}

func TestLegacyManifestDefaultsToEnabled(t *testing.T) {
	b := newTestBackend(t, "Filename Type Size Used Priority\n", &fakeRunner{})
	if err := os.WriteFile(b.ManifestPath, []byte(`["/old/swapfile"]`), 0600); err != nil {
		t.Fatal(err)
	}
	managed, err := b.loadManifest()
	if err != nil || !reflect.DeepEqual(managed, []manifestEntry{{Path: "/old/swapfile", Enabled: true}}) {
		t.Fatalf("manifest=%#v err=%v", managed, err)
	}
}

func TestCorruptManifestDoesNotHideActiveSwap(t *testing.T) {
	b := newTestBackend(t, "Filename Type Size Used Priority\n/dev/zram0 partition 1024 0 1\n", &fakeRunner{})
	if err := os.MkdirAll(filepath.Dir(b.ManifestPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b.ManifestPath, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}
	entries, err := b.List()
	if err == nil || len(entries) != 1 || entries[0].Path != "/dev/zram0" {
		t.Fatalf("entries=%#v err=%v", entries, err)
	}
}
