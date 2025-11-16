package embed

import (
	"image"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	deskDriver "fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	wmtheme "fyshos.com/fynedesk/theme"

	"fyshos.com/fynedesk"
	"github.com/FyshOS/saver"
)

type EmbedWindowManager interface {
	fynedesk.WindowManager

	SetRootWindow(fyne.Window) fyne.CanvasObject
}

type embededWM struct {
	windows   []fynedesk.Window
	listeners []fynedesk.StackListener

	multi *container.MultipleWindows
	over  *fyne.Container
	root  fyne.Window
}

func NewWM(m *container.MultipleWindows, o *fyne.Container) EmbedWindowManager {
	return &embededWM{multi: m, over: o}
}

func (e *embededWM) AddWindow(win fynedesk.Window) {
	e.windows = append(e.windows, win)

	for _, l := range e.listeners {
		l.WindowAdded(win)
	}
}

func (e *embededWM) RaiseToTop(w fynedesk.Window) {
	e.multi.RaiseToTop(w.(*embedWindow).inner)
}

func (e *embededWM) RemoveWindow(win fynedesk.Window) {
	for i, w := range e.windows {
		if w != win {
			continue
		}

		e.windows = append(e.windows[:i], e.windows[i+1:]...)

		for _, l := range e.listeners {
			l.WindowRemoved(win)
		}

		return
	}
}

func (e *embededWM) Run() {
}

func (e *embededWM) ShowOverlay(content fyne.CanvasObject, closed func(), s fyne.Size, p fyne.Position) (fyne.Canvas, func()) {
	return e.doShowOverlay(content, closed, s, p, false)
}

func (e *embededWM) doShowOverlay(content fyne.CanvasObject, closed func(), s fyne.Size, p fyne.Position, modal bool) (fyne.Canvas, func()) {
	if p.IsZero() {
		x, y := e.root.Content().Size().Components()
		p = fyne.NewPos((x-s.Width)/2, (y-s.Height)/2)
	}

	var bg fyne.CanvasObject
	doClose := func() {
		if fn := closed; fn != nil {
			fn()
		}
		e.over.Remove(bg)
	}

	isIn := false
	win := container.NewStack(
		newHoverer(theme.BackgroundColor(), func() {
			isIn = true
		}),
		content)

	bg = container.NewStack(
		newHoverer(color.Transparent, func() {
			if !isIn || modal {
				return
			}

			doClose()
		}),
		container.NewWithoutLayout(win))

	e.root.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		if ev.Name == fyne.KeyEscape {
			doClose()
			return
		}
	})

	e.over.Add(bg)
	bg.Resize(e.over.Size())
	win.Resize(s)
	win.Move(p)
	e.over.Refresh()
	return e.root.Canvas(), doClose
}

func (e *embededWM) ShowMenuOverlay(menu *fyne.Menu, s fyne.Size, p fyne.Position) {
	wid := widget.NewPopUpMenu(menu, e.root.Canvas())
	if s.Width > 0 {
		wid.Resize(s)
	} else {
		wid.Resize(fyne.NewSize(wmtheme.WidgetPanelWidth, wid.MinSize().Height))
	}
	wid.ShowAtPosition(p)
}

func (e *embededWM) ShowModal(content fyne.CanvasObject, onClose func(), s fyne.Size) (fyne.Canvas, func()) {
	_, over := e.doShowOverlay(content, onClose, s, fyne.Position{}, true)
	return e.root.Canvas(), over
}

func (e *embededWM) ShowWindow(content fyne.CanvasObject, title string, closed func(), s fyne.Size) (fyne.Canvas, func()) {
	w := container.NewInnerWindow(title, content)
	w.Resize(s)
	if closed != nil {
		w.CloseIntercept = func() {
			closed()
			w.Hide()
		}
	}

	buttonAlign := widget.ButtonAlignLeading
	if fynedesk.Instance().Settings().BorderButtonPosition() == "Right" {
		buttonAlign = widget.ButtonAlignTrailing
	}
	w.Alignment = buttonAlign

	w.Icon = fyne.CurrentApp().Icon()
	embed := &embedWindow{inner: w}
	setupInnerWindow(w, embed)

	fynedesk.Instance().WindowManager().AddWindow(embed)
	e.multi.Add(w)
	return e.root.Canvas(), w.Close
}

func (e *embededWM) TopWindow() fynedesk.Window {
	if len(e.windows) == 0 {
		return nil
	}

	var top *container.InnerWindow
	for i := len(e.multi.Windows) - 1; i >= 0; i-- {
		w := e.multi.Windows[i]
		if w.Visible() {
			top = w
			break
		}
	}
	if top == nil {
		return e.windows[len(e.windows)-1]
	}

	for _, w := range e.windows {
		if w.(*embedWindow).inner == top {
			return w
		}
	}

	return e.windows[len(e.windows)-1]
}

func (e *embededWM) Windows() []fynedesk.Window {
	return e.windows
}

func (e *embededWM) AddStackListener(l fynedesk.StackListener) {
	e.listeners = append(e.listeners, l)
}

func (e *embededWM) Blank() {
	// no-op, we don't control screen brightness
}

func (e *embededWM) Capture() image.Image {
	return nil // would mean accessing the underling OS screen functions...
}

func (e *embededWM) Close() {
	windows := fyne.CurrentApp().Driver().AllWindows()
	if len(windows) > 0 {
		windows[0].Close() // ensure our root is asked to close as well
	}
}

var visible bool

func (e *embededWM) NotifyWindowMoved(win fynedesk.Window) {
	for _, l := range e.listeners {
		fyne.Do(func() {
			l.WindowMoved(win)
		})
	}
}

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

func (e *embededWM) publishWindowChange(win fynedesk.Window) {
	for _, l := range e.listeners {
		l.WindowStateChanged(win)
	}
}

func (e *embededWM) SetRootWindow(win fyne.Window) fyne.CanvasObject {
	e.root = win

	return newSaverMonitor(fynedesk.Instance().DelayScreenSaver)
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

type hoverer struct {
	widget.BaseWidget

	color   color.Color
	hovered func()
}

func newHoverer(bg color.Color, hover func()) fyne.CanvasObject {
	h := &hoverer{hovered: hover, color: bg}
	h.ExtendBaseWidget(h)

	return h
}

func (h *hoverer) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(canvas.NewRectangle(h.color))
}

func (h *hoverer) MouseIn(*deskDriver.MouseEvent) {
}

func (h *hoverer) MouseMoved(*deskDriver.MouseEvent) {
	h.hovered()
}

func (h *hoverer) MouseOut() {
}
