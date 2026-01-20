package deskwidgets

import (
	"image/color"
	"math/rand"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"fyshos.com/fynedesk"
	wmTheme "fyshos.com/fynedesk/theme"
)

type DeskWidgets interface {
	EditAll(bool)
	Add()
}

type widgets struct {
	widgets []*deskWidget
}

func (w *widgets) Add() {
	dialog := fyne.CurrentApp().NewWindow("Add Widget")
	dialog.SetContent(widget.NewLabel("Here will be a selection of supported widgets"))
	dialog.Show()
}

func (w *widgets) Destroy() {
}

func (w *widgets) EditAll(edit bool) {
	for _, i := range w.widgets {
		i.edit(edit)
	}
}

func (w *widgets) Metadata() fynedesk.ModuleMetadata {
	return widgetsMeta
}

func (w *widgets) ScreenAreaWidget() fyne.CanvasObject {
	items := []fyne.CanvasObject{
		newDeskWidget(widget.NewLabel("This is a test widget")),
		newBar(100, func() float32 {
			i := rand.Intn(100)
			return float32(i)
		}),
	}

	w.widgets = nil
	for i, item := range items {
		item.Move(fyne.NewPos(50+10*float32(i), 120+10*float32(i)))
		item.Resize(item.MinSize())

		w.widgets = append(w.widgets, item.(*deskWidget))
	}

	return container.NewWithoutLayout(items...)
}

// newDesktops creates a new module that will manage virtual desktops and display a pager widget.
func newDesktopWidgets() fynedesk.Module {
	w := &widgets{}
	return w
}

type deskWidget struct {
	widget.BaseWidget

	content, frame            fyne.CanvasObject
	dragging, editing, resize bool
}

func newDeskWidget(content fyne.CanvasObject) *deskWidget {
	d := &deskWidget{content: content}
	d.ExtendBaseWidget(d)
	return d
}

func (d *deskWidget) CreateRenderer() fyne.WidgetRenderer {
	frame := canvas.NewRectangle(color.Transparent)
	frame.StrokeWidth = 3
	frame.StrokeColor = theme.ForegroundColor()

	resize := canvas.NewImageFromResource(wmTheme.BorderResizeIcon)
	resize.SetMinSize(fyne.NewSquareSize(18))

	closer := container.NewStack(
		canvas.NewCircle(theme.BackgroundColor()),
		widget.NewIcon(theme.WindowCloseIcon()),
	)

	pad := theme.InnerPadding()
	d.frame = container.NewStack(frame,
		container.NewHBox(layout.NewSpacer(),
			container.NewVBox(
				container.New(layout.NewCustomPaddedLayout(-pad, 0, 0, -pad), closer),
				layout.NewSpacer(), container.NewPadded(resize))))
	d.frame.Hide()
	return widget.NewSimpleRenderer(container.NewStack(d.content, d.frame))
}

func (d *deskWidget) Dragged(ev *fyne.DragEvent) {
	if !d.editing {
		return
	}

	if !d.dragging {
		d.resize = ev.Position.X > d.Size().Width-16 && ev.Position.Y > d.Size().Height-16
	}

	d.dragging = true
	if d.resize {
		d.Resize(d.Size().Add(ev.Dragged))
	} else {
		d.Move(d.Position().Add(ev.Dragged))
	}
}

func (d *deskWidget) DragEnd() {
	d.dragging = false
}

func (d *deskWidget) TappedSecondary(_ *fyne.PointEvent) {
	d.edit(!d.editing)
}

func (d *deskWidget) edit(edit bool) {
	d.editing = edit
	if d.editing {
		d.frame.Show()
	} else {
		d.frame.Hide()
	}
	d.Refresh()
}
