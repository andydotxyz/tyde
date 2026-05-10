package desktops

import (
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"fyshos.com/tyde"
)

type pager struct {
	buttons, labels *fyne.Container
	wins            *fyne.Container
	winObjs         map[tyde.Window]fyne.CanvasObject // quick lookup for move updates
}

func newPager(d *desktops) *pager {
	p := &pager{wins: container.NewWithoutLayout()}

	buttons := make([]fyne.CanvasObject, deskCount)
	labels := make([]fyne.CanvasObject, deskCount)
	for i := 0; i < deskCount; i++ {
		id := strconv.Itoa(i + 1)
		deskID := i
		buttons[i] = widget.NewButton("", func() {
			d.setDesktop(deskID)
		})
		labels[i] = widget.NewLabelWithStyle(id, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	}

	if tyde.Instance() != nil && tyde.Instance().Settings().NarrowWidgetPanel() {
		p.buttons = container.NewGridWithColumns(1, buttons...)
		p.labels = container.NewGridWithColumns(1, labels...)
	} else {
		p.buttons = container.NewGridWithColumns(4, buttons...)
		p.labels = container.NewGridWithColumns(4, labels...)
	}
	p.refresh()
	tyde.Instance().WindowManager().AddStackListener(p)

	return p
}

func (p *pager) WindowAdded(_ tyde.Window) {
	p.refresh()
}

func (p *pager) WindowMoved(win tyde.Window) {
	obj, ok := p.winObjs[win]
	if !ok {
		p.refresh()
		return
	}

	desk := tyde.Instance()
	oldID := desk.Desktop()
	pivot := p.buttons.Objects[oldID]
	screen := desk.Screens().ScreenForWindow(win)

	deskID := win.Desktop()
	yPad := theme.Padding() * float32(deskID-oldID)
	if win.Pinned() {
		yPad = 0
	}

	scale := screen.CanvasScale()
	x := (win.Position().X * scale) / float32(screen.Width) * pivot.Size().Width
	y := (win.Position().Y * scale) / float32(screen.Height) * pivot.Size().Height
	w := (win.Size().Width * scale) / float32(screen.Width) * pivot.Size().Width
	h := (win.Size().Height * scale) / float32(screen.Height) * pivot.Size().Height
	fyne.Do(func() {
		obj.Move(pivot.Position().Add(fyne.NewPos(x, y+yPad)))
		obj.Resize(fyne.NewSize(w, h))
		p.wins.Refresh()
	})
}

func (p *pager) WindowOrderChanged() {
	p.refresh()
}

func (p *pager) WindowRemoved(_ tyde.Window) {
	p.refresh()
}

func (p *pager) WindowStateChanged(_ tyde.Window) {
}

func (p *pager) refresh() {
	desk := tyde.Instance()
	fyne.Do(func() {
		p.refreshFrom(desk.Desktop())
	})
}

func (p *pager) refreshButtons() {
	desk := tyde.Instance()
	fyne.Do(func() {
		for i, b := range p.buttons.Objects {
			l := p.labels.Objects[i]
			if i == desk.Desktop() {
				b.(*widget.Button).Importance = widget.HighImportance
				l.(*widget.Label).Importance = widget.LowImportance
			} else {
				b.(*widget.Button).Importance = widget.MediumImportance
				l.(*widget.Label).Importance = widget.MediumImportance
			}

			b.Refresh()
			l.Refresh()
		}
	})
}

func (p *pager) refreshFrom(oldID int) {
	desk := tyde.Instance()
	wins := tyde.Instance().WindowManager().Windows()

	p.refreshButtons()

	var rects []fyne.CanvasObject
	pivot := p.buttons.Objects[oldID]
	winObjs := make(map[tyde.Window]fyne.CanvasObject, len(wins))

	for j := len(wins) - 1; j >= 0; j-- {
		win := wins[j]
		if win.Iconic() || win.Properties().SkipTaskbar() {
			continue
		}

		deskID := win.Desktop()
		yPad := theme.Padding() * float32(deskID-oldID)
		screen := tyde.Instance().Screens().ScreenForWindow(win)
		if win.Pinned() {
			yPad = theme.Padding() * float32(desk.Desktop()-oldID)
			yPad -= float32(oldID-desk.Desktop()) * pivot.Size().Height
		}

		var obj fyne.CanvasObject
		obj = canvas.NewRectangle(theme.Color(theme.ColorNameDisabled))
		if win.Properties().Icon() != nil {
			obj = container.NewStack(obj,
				canvas.NewImageFromResource(win.Properties().Icon()))
		}
		rects = append(rects, obj)
		winObjs[win] = obj

		scale := screen.CanvasScale()
		x := (win.Position().X * scale) / float32(screen.Width) * pivot.Size().Width
		y := (win.Position().Y * scale) / float32(screen.Height) * pivot.Size().Height
		w := (win.Size().Width * scale) / float32(screen.Width) * pivot.Size().Width
		h := (win.Size().Height * scale) / float32(screen.Height) * pivot.Size().Height
		obj.Resize(fyne.NewSize(w, h))
		obj.Move(pivot.Position().Add(fyne.NewPos(x, y+yPad)))
	}

	p.winObjs = winObjs
	p.wins.Objects = rects
	p.wins.Refresh()
}
