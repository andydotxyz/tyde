package ui

import (
	"fyne.io/fyne/v2"
)

// rootWindow wraps the desktop's primary window so the desktop is told whenever a
// Fyne overlay - a dialog, a pop-up menu, anything that goes through
// Canvas().Overlays() - is added to or removed from it.
type rootWindow struct {
	fyne.Window

	canvas fyne.Canvas
}

func newRootWindow(win fyne.Window, desk *desktop) *rootWindow {
	c := win.Canvas()
	return &rootWindow{
		Window: win,
		canvas: &rootCanvas{
			Canvas:   c,
			overlays: &rootOverlays{OverlayStack: c.Overlays(), desk: desk},
		},
	}
}

func (w *rootWindow) Canvas() fyne.Canvas {
	return w.canvas
}

// rootCanvas is the root window's canvas with a reporting overlay stack.
type rootCanvas struct {
	fyne.Canvas

	overlays fyne.OverlayStack
}

func (c *rootCanvas) Overlays() fyne.OverlayStack {
	return c.overlays
}

// rootOverlays adds and removes overlays on the real stack, then tells the desktop
// so it can update the window manager input shapes and focus.
type rootOverlays struct {
	fyne.OverlayStack

	desk *desktop
}

func (o *rootOverlays) Add(overlay fyne.CanvasObject) {
	o.OverlayStack.Add(overlay)
	o.desk.canvasOverlaysChanged(true)
}

func (o *rootOverlays) Remove(overlay fyne.CanvasObject) {
	o.OverlayStack.Remove(overlay)
	o.desk.canvasOverlaysChanged(false)
}
