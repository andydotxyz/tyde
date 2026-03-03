package systray

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

type multiButton struct {
	widget.Button

	rightTap func()
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
