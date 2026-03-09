package status

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

type scrollButton struct {
	widget.Button

	scroll func(float32)
}

func (s *scrollButton) Scrolled(event *fyne.ScrollEvent) {
	if s.scroll == nil {
		return
	}

	s.scroll(event.Scrolled.DY)
}

func newScrollButton(r fyne.Resource) *scrollButton {
	s := &scrollButton{}
	s.Icon = r

	s.ExtendBaseWidget(s)
	return s
}

type scrollIcon struct {
	widget.Icon

	scroll func(float32)
}

func (s *scrollIcon) Scrolled(event *fyne.ScrollEvent) {
	if s.scroll == nil {
		return
	}

	s.scroll(event.Scrolled.DY)
}

func newScrollIcon(r fyne.Resource) *scrollIcon {
	s := &scrollIcon{}
	s.Resource = r

	s.ExtendBaseWidget(s)
	return s
}
