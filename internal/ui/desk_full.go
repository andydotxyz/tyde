package ui

import (
	"image/color"
	"log"
	"runtime/debug"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	wmtheme "fyshos.com/fynedesk/theme"
)

func (l *desktop) newDesktopWindowFull() fyne.Window {
	desk := l.app.NewWindow(RootWindowName)
	desk.SetPadded(false)

	desk.SetMaster()
	desk.SetOnClosed(func() {
		l.wm.Close()
	})

	return desk
}

func (l *desktop) runFull() {
	debug.SetPanicOnFault(true)

	defer func() {
		if r := recover(); r != nil {
			log.Println("FyneDesk panic cause", r)
			debug.PrintStack()
			l.wm.Close() // attempt to close cleanly to leave X server running
		}
	}()

	l.root.ShowAndRun()
}

func (l *desktop) showMenuFull(menu *fyne.Menu, pos fyne.Position) {
	menuSize := widget.NewMenu(menu).MinSize()
	size := fyne.NewSize(wmtheme.WidgetPanelWidth, menuSize.Height)

	// Measure child menus to calculate the total hover-catch area.
	// The submenu appears to the right, aligned with its parent item,
	// so it can extend below the parent menu.
	catchSize := size
	itemY := float32(0)
	for _, item := range menu.Items {
		itemH := widget.NewLabel(item.Label).MinSize().Height
		if item.ChildMenu != nil {
			childSize := widget.NewMenu(item.ChildMenu).MinSize()
			catchSize.Width = size.Width + childSize.Width
			totalH := itemY + childSize.Height
			if totalH > catchSize.Height {
				catchSize.Height = totalH
			}
		}
		if item.IsSeparator {
			itemY += theme.SeparatorThicknessSize()
		} else {
			itemY += itemH
		}
	}

	var combined fyne.CanvasObject

	// Wrap each menu item action (and child menu item actions) to dismiss the overlay
	var wrapItems func([]*fyne.MenuItem)
	wrapItems = func(items []*fyne.MenuItem) {
		for _, item := range items {
			if item.Action != nil {
				origAction := item.Action
				item.Action = func() {
					l.HideOverlay(combined)
					origAction()
				}
			}
			if item.ChildMenu != nil {
				wrapItems(item.ChildMenu.Items)
			}
		}
	}
	wrapItems(menu.Items)

	r, g, b, _ := theme.Color(theme.ColorNameOverlayBackground).RGBA()
	bgCol := &color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 230}
	bg := canvas.NewRectangle(bgCol)

	menuWidget := widget.NewMenu(menu)
	content := container.NewStack(bg, menuWidget)

	combined = l.ShowOverlayWithBackdrop(content, size, catchSize, pos)
}
