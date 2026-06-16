package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"fyshos.com/tyde"
	wmTest "fyshos.com/tyde/test"
)

// fakeOverlayModule is a minimal OverlayAreaModule used to exercise the
// above-windows overlay layer's enable/disable handling.
type fakeOverlayModule struct {
	widget    fyne.CanvasObject
	destroyed bool
}

func (f *fakeOverlayModule) Metadata() tyde.ModuleMetadata {
	return tyde.ModuleMetadata{Name: "fakeOverlay"}
}
func (f *fakeOverlayModule) Destroy()                             { f.destroyed = true }
func (f *fakeOverlayModule) OverlayAreaWidget() fyne.CanvasObject { return f.widget }

// TestOverlayLayerReflectsModuleToggle verifies that rebuilding the overlay
// layer adds the widget of an enabled OverlayAreaModule and removes it when the
// module is gone - the mechanism that makes enabling/disabling a desktop pet
// take effect immediately (driven from fireSettingsChangeListener).
func TestOverlayLayerReflectsModuleToggle(t *testing.T) {
	test.NewApp()
	l := &desktop{settings: wmTest.NewSettings(), screens: wmTest.NewScreensProvider()}
	o := newOverlayLayer(l)

	if got := len(o.content.Objects); got != 0 {
		t.Fatalf("expected no overlay widgets initially, got %d", got)
	}

	// Enable: inject a fake overlay module and rebuild.
	rect := canvas.NewRectangle(color.Black)
	l.moduleCache = []tyde.Module{&fakeOverlayModule{widget: rect}}
	o.rebuild()
	if got := len(o.content.Objects); got != 1 || o.content.Objects[0] != rect {
		t.Fatalf("enabled overlay module not shown (objects=%d)", got)
	}

	// Disable: the module is gone, rebuild clears it.
	l.moduleCache = []tyde.Module{}
	o.rebuild()
	if got := len(o.content.Objects); got != 0 {
		t.Fatalf("disabled overlay module still shown (objects=%d)", got)
	}
}

// TestClearModuleCacheDestroys verifies disabling tears the module down (which,
// for a desktop pet, stops its animation loop).
func TestClearModuleCacheDestroys(t *testing.T) {
	test.NewApp()
	l := &desktop{settings: wmTest.NewSettings()}
	fake := &fakeOverlayModule{widget: canvas.NewRectangle(color.Black)}
	l.moduleCache = []tyde.Module{fake}

	l.clearModuleCache()

	if !fake.destroyed {
		t.Fatal("clearModuleCache should Destroy module instances")
	}
	if l.moduleCache != nil {
		t.Fatal("module cache should be cleared")
	}
}
