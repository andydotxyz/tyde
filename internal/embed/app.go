package embed

import (
	"fyshos.com/fynedesk"
	"github.com/FyshOS/appie"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type app struct {
	m *container.MultipleWindows

	name        string
	categories  []string
	icon        fyne.Resource
	makeContent func() fyne.CanvasObject
}

func (a *app) Actions() []appie.Action {
	return nil
}

func (a *app) Name() string {
	return a.name
}

func (a *app) Run(_ []string) error {
	w := container.NewInnerWindow(a.name, a.makeContent())
	w.OnMaximized = func() {
		//head := fynedesk.Instance().Screens().ScreenForWindow(w)
		head := fynedesk.Instance().Screens().Primary()
		maxX, maxY, maxWidth, maxHeight := fynedesk.Instance().ContentBoundsPixels(head)
		w.Move(fyne.NewPos(float32(maxX), float32(maxY)))
		w.Resize(fyne.NewSize(float32(maxWidth), float32(maxHeight)))
	}

	buttonAlign := widget.ButtonAlignLeading
	if fynedesk.Instance().Settings().BorderButtonPosition() == "Right" {
		buttonAlign = widget.ButtonAlignTrailing
	}
	w.Alignment = buttonAlign

	a.m.Add(w)
	return nil
}

func (a *app) RunWithParameters(_ []string, env []string) error {
	return a.Run(env)
}

func (a *app) Categories() []string {
	return a.Categories()
}

func (a *app) Hidden() bool {
	return false
}

func (a *app) Icon(_ string, _ int) fyne.Resource {
	return a.icon
}

func (a *app) MimeTypes() []string {
	return nil
}

func (a *app) Source() *appie.AppSource {
	return nil
}
