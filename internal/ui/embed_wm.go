package ui

import (
	"image/color"

	"fyshos.com/tyde"
	"github.com/FyshOS/saver"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	deskDriver "fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

type embededWM struct {
	windows []tyde.Window
	root    fyne.Window
}

func (e *embededWM) AddWindow(win tyde.Window) {
	e.windows = append(e.windows, win)
}

func (e *embededWM) RaiseToTop(tyde.Window) {
	// no-op
}

func (e *embededWM) RemoveWindow(win tyde.Window) {
	for i, w := range e.windows {
		if w != win {
			continue
		}

		e.windows = append(e.windows[:i], e.windows[i+1:]...)
		return
	}
}

func (e *embededWM) Run() {
}

func (e *embededWM) ShowOverlay(w fyne.Window, s fyne.Size, p fyne.Position) {
	w.Resize(s)
	w.Show()
}

func (e *embededWM) ShowMenuOverlay(*fyne.Menu, fyne.Size, fyne.Position) {
	// no-op, handled by desktop in embed mode
}

func (e *embededWM) TopWindow() tyde.Window {
	if len(e.windows) == 0 {
		return nil
	}

	return e.windows[len(e.windows)-1]
}

func (e *embededWM) Windows() []tyde.Window {
	return e.windows
}

func (e *embededWM) AddStackListener(tyde.StackListener) {
	// no stack
}

func (e *embededWM) RemoveStackListener(tyde.StackListener) {
	// no stack
}

func (e *embededWM) Blank() {
	// no-op, we don't control screen brightness
}

func (e *embededWM) Close() {
	windows := fyne.CurrentApp().Driver().AllWindows()
	if len(windows) > 0 {
		windows[0].Close() // ensure our root is asked to close as well
	}
}

var visible bool

func (e *embededWM) ShowScreensaver(s *saver.ScreenSaver) {
	if visible {
		return
	}

	visible = true
	over := container.NewStack(canvas.NewRectangle(color.Black))

	s.OnUnlocked = func() {
		visible = false
		e.root.Canvas().Overlays().Remove(over)
	}

	over.Add(s.MakeUI(e.root))
	over.Resize(e.root.Canvas().Size())
	e.root.Canvas().Overlays().Add(over)
}

func (e *embededWM) setWindow(win fyne.Window) fyne.CanvasObject {
	e.root = win

	return newSaverMonitor(tyde.Instance().DelayScreenSaver)
}

type saverMonitor struct {
	widget.BaseWidget

	cb func()
}

func newSaverMonitor(cb func()) fyne.CanvasObject {
	s := &saverMonitor{cb: cb}
	s.ExtendBaseWidget(s)
	return s
}

func (s *saverMonitor) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(canvas.NewRectangle(color.Transparent))
}

func (s *saverMonitor) MouseIn(*deskDriver.MouseEvent) {
}

func (s *saverMonitor) MouseMoved(*deskDriver.MouseEvent) {
	s.cb()
}

func (s *saverMonitor) MouseOut() {
}
