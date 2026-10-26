package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbiw/input/sticks"
	"github.com/movsb/fbui/pkg/config"
	"github.com/movsb/fbui/pkg/mainui"
	"golang.org/x/image/font/gofont/goregular"
)

// These navigation tests construct documents without opening a display.
type savedItemsTestRenderer struct{ fbiw.Renderer }

func (savedItemsTestRenderer) Size() (int, int) { return 640, 480 }
func (savedItemsTestRenderer) Close() error     { return nil }

func TestCommonColumnsNavigation(t *testing.T) {
	root := t.TempDir()
	oldRoot, oldOverlay := config.SDCARDRoot, overlayWindow
	config.SDCARDRoot = root
	t.Cleanup(func() { config.SDCARDRoot, overlayWindow = oldRoot, oldOverlay })
	app := fbiw.NewApp(fbiw.WithRenderer(savedItemsTestRenderer{}), fbiw.WithSystemFontData(goregular.TTF))
	defer app.Close()
	overlayDoc := app.NewOverlay(_embed, "overlay.html")
	overlayWindow = &OverlayWindow{doc: overlayDoc}
	overlayDoc.Bind(overlayWindow)
	win := &MainWindow{app: app, doc: app.NewDesktop(_embed, "window.html"), lists: mainui.NewLists(root)}
	win.statusBarNav = NewStatusBarNavigator(win)
	win.commonNav = NewSavedItemsNavigator(win)
	n := win.commonNav
	win.statusBarNav.setIndex(0)
	n.activate()
	n.container.Calc(640, 440, fbiw.Constraints{ParentContentWidth: 640, ParentContentHeight: 440, PrefersMaxWidth: true, PrefersMaxHeight: true})
	for _, column := range n.columns {
		empty := column.empty.Base().GetLayoutBox()
		text := column.empty.Base().Children()[0].Base().GetLayoutBox()
		dx := text.X*2 + text.Width - empty.Width
		dy := text.Y*2 + text.Height - empty.Height
		if empty.Height < 400 || dx < -1 || dx > 1 || dy < -1 || dy > 1 {
			t.Fatalf("empty message should fill and center in the column: empty=%+v, text=%+v", empty, text)
		}
	}
	if win.doc.ActiveBox().Base() != n.container.Base() || !n.favorites {
		t.Fatal("empty favorites should focus the common page")
	}
	// An empty category must still allow switching and returning to the title.
	win.doc.ActiveBox().Base().Dispatch(fbiw.InputDownEvent, fbiw.InputEventArgs{Name: sticks.Left})
	if n.favorites || loadCommonFavorites(root) {
		t.Fatal("left should select and remember recent")
	}
	win.doc.ActiveBox().Base().Dispatch(fbiw.InputDownEvent, fbiw.InputEventArgs{Name: sticks.B})
	if win.doc.ActiveBox().Base() != win.statusBarNav.catBar.Base() {
		t.Fatal("B should return to the title bar")
	}
	launch := filepath.Join(root, "launch.sh")
	if err := os.WriteFile(launch, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for i := range 12 {
		rom := filepath.Join(root, fmt.Sprintf("%d.zip", i))
		if err := os.WriteFile(rom, []byte("rom"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := win.lists.AddRecent(mainui.Item{Label: fmt.Sprint(i), SubLabel: "测试", Launch: launch, ROMPath: rom}); err != nil {
			t.Fatal(err)
		}
	}
	n.activate()
	if len(n.columns[false].items) != 12 || n.columns[true].list.DataIndex() != -1 {
		t.Fatal("recent items should be loaded while the other column stays unselected")
	}
	n.current().list.SetIndex(3, 0, 8)
	// Right must switch columns before the list consumes it as page navigation.
	n.current().list.Dispatch(fbiw.InputDownEvent, fbiw.InputEventArgs{Name: sticks.Right})
	if !n.favorites {
		t.Fatal("right from a populated list should switch to favorites")
	}
	if len(n.columns[false].items) != 12 || n.columns[false].list.DataIndex() != 11 {
		t.Fatal("switching focus must keep recent items and their scroll position")
	}
	win.doc.ActiveBox().Base().Dispatch(fbiw.InputDownEvent, fbiw.InputEventArgs{Name: sticks.Left})
	if n.current().list.DataIndex() != 11 {
		t.Fatalf("recent selection lost: got %d", n.current().list.DataIndex())
	}
	if _, err := win.lists.ToggleFavorite(n.current().items[0]); err != nil {
		t.Fatal(err)
	}
	n.indices[false] = n.current().list.DataIndex()
	n.reload()
	if len(n.columns[true].items) != 1 || len(n.columns[false].items) != 12 {
		t.Fatal("both columns must contain their items at the same time")
	}
	if n.columns[true].list.ClassContains("active-column") || !n.columns[false].list.ClassContains("active-column") {
		t.Fatal("only the focused column should highlight its selection")
	}
	if err := win.lists.Clear(false); err != nil {
		t.Fatal(err)
	}
	n.switchColumn(true)
	n.switchColumn(false)
	if n.current().list.DataIndex() != -1 || win.doc.ActiveBox().Base() != n.container.Base() {
		t.Fatal("cleared category should have no selection and remain navigable")
	}
}
