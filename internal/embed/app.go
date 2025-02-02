package embed

import (
	"github.com/FyshOS/appie"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

type app struct {
	m *container.MultipleWindows

	name        string
	categories  []string
	icon        fyne.Resource
	makeContent func() fyne.CanvasObject
}

func (a *app) Name() string {
	return a.name
}

func (a *app) Run(_ []string) error {
	w := container.NewInnerWindow(a.name, a.makeContent())

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
