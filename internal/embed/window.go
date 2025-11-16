package embed

import (
	"image"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/widget"

	"fyshos.com/fynedesk"
)

type embedWindow struct {
	inner *container.InnerWindow

	iconic, maximized, pinned bool
	desk                      int
	prevPos                   fyne.Position
	prevSize                  fyne.Size
}

func (e *embedWindow) Focused() bool {
	return false // TODO implement
}

func (e *embedWindow) Fullscreened() bool {
	return false // TODO implement
}

func (e *embedWindow) Iconic() bool {
	return e.iconic
}

func (e *embedWindow) Maximized() bool {
	return e.maximized
}

func (e *embedWindow) TopWindow() bool {
	return fynedesk.Instance().WindowManager().TopWindow() == e
}

func (e *embedWindow) Capture() image.Image {
	return software.Render(e.inner.Content, fyne.CurrentApp().Settings().Theme())
}

func (e *embedWindow) Close() {
	e.inner.Close()
}

func (e *embedWindow) Focus() {
	e.RaiseToTop()
}

func (e *embedWindow) Fullscreen() {
	// TODO implement me
}

func (e *embedWindow) Iconify() {
	e.iconic = true
	e.inner.Hide()
	fynedesk.Instance().WindowManager().(*embededWM).publishWindowChange(e)
}

func (e *embedWindow) Maximize() {
	e.inner.SetMaximized(true)
}

func (e *embedWindow) RaiseToTop() {
	fynedesk.Instance().WindowManager().RaiseToTop(e)
}

func (e *embedWindow) Unfullscreen() {
	// TODO implement me
}

func (e *embedWindow) Uniconify() {
	e.iconic = false
	e.inner.Show()
	fynedesk.Instance().WindowManager().(*embededWM).publishWindowChange(e)
}

func (e *embedWindow) Unmaximize() {
	e.inner.SetMaximized(false)
}

func (e *embedWindow) Parent() fynedesk.Window {
	return nil
}

func (e *embedWindow) Properties() fynedesk.WindowProperties {
	return &embedProps{win: e.inner}
}

func (e *embedWindow) Position() fyne.Position {
	return e.inner.Position()
}

func (e *embedWindow) Size() fyne.Size {
	return e.inner.Size()
}

func (e *embedWindow) Move(p fyne.Position) {
	e.inner.Move(p)
}

func (e *embedWindow) Resize(s fyne.Size) {
	e.inner.Resize(s)
}

func (e *embedWindow) Desktop() int {
	return e.desk
}

func (e *embedWindow) SetDesktop(id int) {
	if e.desk == id {
		return
	}

	diff := id - e.desk
	e.desk = id
	if e.pinned {
		return
	}

	d := fynedesk.Instance()
	_, height := d.RootSizePixels()
	offPix := float32(diff * -int(height))
	display := d.Screens().ScreenForWindow(e)
	off := offPix / display.Scale

	start := e.Position()
	fyne.NewAnimation(canvas.DurationStandard, func(f float32) {
		newY := start.Y - off*f

		e.Move(fyne.NewPos(start.X, newY))
		notifyMove(e)
	}).Start()
}

func (e *embedWindow) Pin() {
	e.pinned = true
	d := fynedesk.Instance()
	e.SetDesktop(d.Desktop())
}

func (e *embedWindow) Pinned() bool {
	return e.pinned
}

func (e *embedWindow) Unpin() {
	e.pinned = false
	d := fynedesk.Instance()
	id := d.Desktop()
	e.desk = id

	e.SetDesktop(id)
}

func setupInnerWindow(w *container.InnerWindow, embed *embedWindow) {
	w.OnTappedIcon = func() {
		showMenu(embed, nil)
	}

	w.OnMaximized = func() {
		if embed.maximized {
			embed.maximized = false
			w.Resize(embed.prevSize)
			w.Move(embed.prevPos)
			return
		}

		embed.prevSize = w.Size()
		embed.prevPos = w.Position()
		embed.maximized = true

		head := fynedesk.Instance().Screens().Primary()
		maxX, maxY, maxWidth, maxHeight := fynedesk.Instance().ContentBoundsPixels(head)
		w.Move(fyne.NewPos(float32(maxX), float32(maxY)))
		w.Resize(fyne.NewSize(float32(maxWidth), float32(maxHeight)))
	}
	w.OnMinimized = func() {
		if embed.iconic {
			embed.Uniconify()
		} else {
			embed.Iconify()
		}
	}

	buttonAlign := widget.ButtonAlignLeading
	if fynedesk.Instance().Settings().BorderButtonPosition() == "Right" {
		buttonAlign = widget.ButtonAlignTrailing
	}
	w.Alignment = buttonAlign

	w.CloseIntercept = func() {
		fynedesk.Instance().WindowManager().RemoveWindow(embed)
		w.Hide()
	}
	w.OnDragged = func(_ *fyne.DragEvent) {
		notifyMove(embed)
	}
}

type embedProps struct {
	win *container.InnerWindow
}

func (e *embedProps) Class() []string {
	return nil
}

func (e *embedProps) Command() string {
	return ""
}

func (e *embedProps) Decorated() bool {
	return true
}

func (e *embedProps) Icon() fyne.Resource {
	return e.win.Icon
}

func (e *embedProps) IconName() string {
	return e.win.Title
}

func (e *embedProps) SkipTaskbar() bool {
	return false
}

func (e *embedProps) Title() string {
	return e.win.Title
}

func notifyMove(embed *embedWindow) {
	type moveNotifier interface {
		NotifyWindowMoved(win fynedesk.Window)
	}
	if mover, ok := fynedesk.Instance().WindowManager().(moveNotifier); ok {
		mover.NotifyWindowMoved(embed)
	}
}
