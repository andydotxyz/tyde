package embed

import (
	"fmt"

	"fyshos.com/fynedesk"
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

func (a *app) Actions() []appie.Action {
	return nil
}

func (a *app) Name() string {
	return a.name
}

func (a *app) Run(_ []string) error {
	w := container.NewInnerWindow(a.name, a.makeContent())
	embed := &embedWindow{inner: w}
	w.Icon = a.icon

	setupInnerWindow(w, embed)
	fynedesk.Instance().WindowManager().AddWindow(embed)
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

// TODO de-duplicate from border once menu overlay bug fixed
func showMenu(w fynedesk.Window, _ fyne.CanvasObject) {
	name := w.Properties().Title()
	if len(name) > 25 {
		name = name[:25] + "..."
	}
	title := fyne.NewMenuItem(name, func() {})
	title.Disabled = true
	max := fyne.NewMenuItem("Maximize", func() {
		if w.Maximized() {
			w.Unmaximize()
		} else {
			w.Maximize()
		}
	})
	if w.Maximized() {
		max.Checked = true
	}

	pos := w.Position()
	menuPos := pos.AddXY(w.Size().Width-32, 0)
	menu := fyne.NewMenu("",
		title,
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Minimize", func() {
			w.Iconify()
		}),
		max,
		fyne.NewMenuItemSeparator(),
		makeDesktopMenu(w),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Close", func() {
			w.Close()
		}))

	fynedesk.Instance().ShowMenuAt(menu, menuPos)
}

func makeDesktopMenu(w fynedesk.Window) *fyne.MenuItem {
	desks := make([]*fyne.MenuItem, 4)
	for i := 0; i < 4; i++ {
		deskID := i
		desks[i] = fyne.NewMenuItem(fmt.Sprintf("Desktop %d", i+1), func() {
			if w.Pinned() {
				w.Unpin()
			}

			w.SetDesktop(deskID)
		})
	}
	pin := fyne.NewMenuItem("All Desktops", func() {
		if w.Pinned() {
			return
		}

		w.Pin()
	})
	if w.Pinned() {
		pin.Checked = true
	}
	desks = append(desks, pin)

	m := fyne.NewMenuItem("Move to Desktop...", nil)
	m.ChildMenu = fyne.NewMenu("", desks...)
	return m
}
