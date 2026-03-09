package systray

import (
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

type multiButton struct {
	widget.Button

	rightTap func()
	scroll   func(float32, bool)
}

func (m *multiButton) Scrolled(event *fyne.ScrollEvent) {
	if m.scroll == nil {
		return
	}

	if math.Abs(float64(event.Scrolled.DY)) >= math.Abs(float64(event.Scrolled.DX)) {
		m.scroll(event.Scrolled.DY, false)
	} else {
		m.scroll(event.Scrolled.DX, true)
	}
}

func (m *multiButton) TappedSecondary(*fyne.PointEvent) {
	m.rightTap()
}

func newMultiButton(leftTap, rightTap func()) *multiButton {
	m := &multiButton{rightTap: rightTap}
	m.OnTapped = leftTap

	m.ExtendBaseWidget(m)
	return m
}
