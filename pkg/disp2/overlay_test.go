package disp2

import "testing"

type recordingBackend struct{ presented []int }

func (b *recordingBackend) present(index int) error {
	b.presented = append(b.presented, index)
	return nil
}
func (*recordingBackend) hide() error  { return nil }
func (*recordingBackend) close() error { return nil }

func TestRefreshKeepsCurrentFrontBuffer(t *testing.T) {
	backend := &recordingBackend{}
	overlay := &Overlay{
		backend: backend,
		buffers: []*Buffer{{}, {}},
		front:   -1,
	}
	if err := overlay.Refresh(); err != nil {
		t.Fatal(err)
	}
	if len(backend.presented) != 0 {
		t.Fatalf("Refresh before Present submitted %v", backend.presented)
	}
	if err := overlay.Present(); err != nil {
		t.Fatal(err)
	}
	if err := overlay.Refresh(); err != nil {
		t.Fatal(err)
	}
	if err := overlay.Present(); err != nil {
		t.Fatal(err)
	}
	if err := overlay.Refresh(); err != nil {
		t.Fatal(err)
	}
	if got := backend.presented; len(got) != 4 || got[0] != 0 || got[1] != 0 || got[2] != 1 || got[3] != 1 {
		t.Fatalf("unexpected submitted buffers: %v", got)
	}
}
