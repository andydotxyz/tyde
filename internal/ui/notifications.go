package ui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	deskDriver "fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"fyshos.com/fynedesk"
	wmtheme "fyshos.com/fynedesk/theme"
	"fyshos.com/fynedesk/wm"
)

type notification struct {
	message *wm.Notification

	renderer fyne.CanvasObject
	popup    fyne.Window
}

func (n *notification) show(list *fyne.Container) {
	title := widget.NewLabel(n.message.Title)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Truncation = fyne.TextTruncateEllipsis
	text := widget.NewLabel(n.message.Body)
	text.Wrapping = fyne.TextWrapWord

	closer := widget.NewButtonWithIcon("", theme.WindowCloseIcon(), func() {
		n.hide(list)
	})
	closer.Importance = widget.LowImportance

	var icon fyne.CanvasObject
	if n.message.Icon != nil {
		img := canvas.NewImageFromResource(n.message.Icon)
		pad := theme.SizeForWidget(theme.SizeNamePadding, title)
		img.SetMinSize(fyne.NewSquareSize(closer.MinSize().Height - pad*2))
		icon = container.New(layout.NewCustomPaddedLayout(pad, pad, pad, 0), img)
	}
	n.renderer = container.NewVBox(
		container.NewBorder(nil, nil, icon, closer, title), text)

	if fynedesk.Instance().Settings().NarrowWidgetPanel() {
		// TODO move away from window when we have overlay proper as this takes focus...
		n.popup = fyne.CurrentApp().Driver().(deskDriver.Driver).CreateSplashWindow()
		n.popup.SetContent(n.renderer)

		width := float32(270)
		offset := float32(10)
		n.renderer.Resize(fyne.NewSize(width, 0))

		winSize := fynedesk.Instance().Root().Canvas().Size()
		pos := fyne.NewPos(winSize.Width-width-offset-wmtheme.NarrowBarWidth, offset)
		fynedesk.Instance().WindowManager().ShowOverlay(n.popup, fyne.NewSize(width, n.renderer.MinSize().Height), pos)
	} else {
		list.Objects = append(list.Objects, n.renderer)
		list.Refresh()
	}

	go func() {
		time.Sleep(time.Second * 10)

		n.hide(list)
	}()
}

func (n *notification) hide(list *fyne.Container) {
	if fynedesk.Instance().Settings().NarrowWidgetPanel() {
		n.popup.Hide()
		return
	}

	var items []fyne.CanvasObject
	for _, item := range list.Objects {
		if item == n.renderer {
			continue
		}

		items = append(items, item)
	}

	list.Objects = items
	list.Refresh()
}

type notifications struct {
	list *fyne.Container
}

func (n *notifications) newMessage(message *wm.Notification) {
	item := &notification{message: message}
	go item.show(n.list)
}

func startNotifications() fyne.CanvasObject {
	box := container.NewVBox()

	n := &notifications{list: box}
	wm.SetNotificationListener(n.newMessage)

	return box
}
