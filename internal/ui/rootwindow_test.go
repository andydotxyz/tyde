package ui

import (
	"image"
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"fyshos.com/tyde"
	wmTest "fyshos.com/tyde/test"
)

// shapeRecorder is a window manager stub that records the input shape updates the
// desktop asks for (the inputShaper interface).
type shapeRecorder struct {
	tyde.WindowManager

	active  bool
	regions []image.Rectangle
}

func (s *shapeRecorder) SetOverlayActive(active bool, regions []image.Rectangle) {
	s.active, s.regions = active, regions
}

// TestRootWindowOverlayClaimsInput checks that a plain Fyne overlay - a dialog or a
// pop-up - shown on the window returned by Root() makes the primary screen
// input-active, so its clicks reach it instead of the application window beneath,
// and that its first field takes keyboard focus.
func TestRootWindowOverlayClaimsInput(t *testing.T) {
	test.NewApp()

	rec := &shapeRecorder{}
	l := &desktop{settings: wmTest.NewSettings(), screens: wmTest.NewScreensProvider(), wm: rec}
	win := test.NewWindow(widget.NewLabel("desktop"))
	defer win.Close()
	l.primaryWin = &screenWindow{win: win}

	entry := widget.NewEntry()
	pop := widget.NewModalPopUp(entry, l.Root().Canvas())
	pop.Show()

	if !rec.active {
		t.Fatal("an overlay on the root window should make the desktop input-active")
	}
	want := image.Rect(0, 0, 2000, 1000) // the whole (test) primary screen
	if len(rec.regions) != 1 || rec.regions[0] != want {
		t.Errorf("expected the primary screen region %v, got %v", want, rec.regions)
	}
	if focused := win.Canvas().Focused(); focused != entry {
		t.Errorf("overlay content should take focus, got %v", focused)
	}

	pop.Hide()
	if rec.active {
		t.Error("input shapes should be restored once the overlay is gone")
	}
}

// TestRootWindowFollowsPrimary checks that the wrapper is rebuilt when the primary
// window changes, as it does when the screen layout is reconfigured.
func TestRootWindowFollowsPrimary(t *testing.T) {
	test.NewApp()

	l := &desktop{settings: wmTest.NewSettings(), screens: wmTest.NewScreensProvider()}
	first := test.NewWindow(nil)
	defer first.Close()
	l.primaryWin = &screenWindow{win: first}

	root := l.Root()
	if same := l.Root(); same != root {
		t.Error("Root should reuse the wrapper while the primary window is unchanged")
	}

	second := test.NewWindow(nil)
	defer second.Close()
	l.primaryWin = &screenWindow{win: second}

	if wrapped := l.Root().(*rootWindow).Window; wrapped != second {
		t.Error("Root should re-wrap after the primary window changed")
	}
}
