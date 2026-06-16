package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

// indexOf returns the position of obj in objs, or -1.
func indexOf(objs []fyne.CanvasObject, obj fyne.CanvasObject) int {
	for i, o := range objs {
		if o == obj {
			return i
		}
	}
	return -1
}

// TestCompositorInterleavesAccessories verifies that a window accessory is drawn
// directly above its window (so a window stacked higher occludes it), and that
// top accessories are drawn above everything.
func TestCompositorInterleavesAccessories(t *testing.T) {
	cw := NewCompositorWidget(nil)
	lower := cw.EnsureWindow(1) // appended first => bottom
	upper := cw.EnsureWindow(2) // appended next => on top

	onLower := canvas.NewRectangle(color.White)
	onTop := canvas.NewRectangle(color.Black)
	cw.SetAccessories(map[uint32][]fyne.CanvasObject{1: {onLower}}, []fyne.CanvasObject{onTop})

	r := cw.CreateRenderer().(*compositorRenderer)
	r.Refresh()
	objs := r.cont.Objects

	iLowerWin := indexOf(objs, lower.Img)
	iUpperWin := indexOf(objs, upper.Img)
	iOnLower := indexOf(objs, onLower)
	iOnTop := indexOf(objs, onTop)

	for name, idx := range map[string]int{"lowerWin": iLowerWin, "upperWin": iUpperWin, "onLower": iOnLower, "onTop": iOnTop} {
		if idx < 0 {
			t.Fatalf("%s not present in draw order", name)
		}
	}
	// Accessory on the lower window sits above it but below the upper window.
	if !(iLowerWin < iOnLower && iOnLower < iUpperWin) {
		t.Fatalf("accessory not occluded by higher window: lowerWin=%d onLower=%d upperWin=%d", iLowerWin, iOnLower, iUpperWin)
	}
	// Top accessory is above all windows.
	if iOnTop < iUpperWin {
		t.Fatalf("top accessory should be above all windows: onTop=%d upperWin=%d", iOnTop, iUpperWin)
	}
}

// TestCompositorAccessoryFallsBackToTop verifies that an accessory anchored to a
// window that no longer exists is still drawn (above everything) rather than lost.
func TestCompositorAccessoryFallsBackToTop(t *testing.T) {
	cw := NewCompositorWidget(nil)
	win := cw.EnsureWindow(1)
	orphan := canvas.NewRectangle(color.White)
	cw.SetAccessories(map[uint32][]fyne.CanvasObject{99: {orphan}}, nil)

	r := cw.CreateRenderer().(*compositorRenderer)
	r.Refresh()
	objs := r.cont.Objects

	if indexOf(objs, orphan) <= indexOf(objs, win.Img) {
		t.Fatal("orphaned accessory should fall back above the windows")
	}
}
