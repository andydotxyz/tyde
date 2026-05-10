package ui

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"fyshos.com/tyde"
	wmtheme "fyshos.com/tyde/theme"
	"fyshos.com/tyde/wm"
)

type notification struct {
	message *wm.Notification

	renderer fyne.CanvasObject
	overlay  fyne.CanvasObject // used for narrow panel mode (overlay)
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

	var ico fyne.CanvasObject
	if n.message.Icon != nil {
		img := canvas.NewImageFromResource(n.message.Icon)
		pad := theme.SizeForWidget(theme.SizeNamePadding, title)
		img.SetMinSize(fyne.NewSquareSize(closer.MinSize().Height - pad*2))
		ico = container.New(layout.NewCustomPaddedLayout(pad, pad, pad, 0), img)
	}
	n.renderer = container.NewVBox(
		container.NewBorder(nil, nil, ico, closer, title), text,
	)

	if tyde.Instance().Settings().NarrowWidgetPanel() {
		r, g, b, _ := theme.Color(theme.ColorNameOverlayBackground).RGBA()
		bgCol := &color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 230}
		bg := canvas.NewRectangle(bgCol)
		n.overlay = container.NewStack(bg, container.NewPadded(n.renderer))

		width := float32(270)
		offset := float32(10)
		n.renderer.Resize(fyne.NewSize(width, 0))
		height := n.renderer.MinSize().Height + theme.Padding()*2

		inst := tyde.Instance()
		primary := inst.Screens().Primary()
		scale := primary.CanvasScale()
		pRight := float32(primary.Width) / scale
		pos := fyne.NewPos(pRight-width-offset-wmtheme.NarrowBarWidth, offset)
		inst.ShowOverlay(n.overlay, fyne.NewSize(width, height), pos)
	} else {
		fyne.Do(func() {
			list.Objects = append(list.Objects, n.renderer)
			list.Refresh()
		})
	}

	go func() {
		time.Sleep(time.Second * 10)

		n.hide(list)
	}()
}

func (n *notification) hide(list *fyne.Container) {
	if tyde.Instance().Settings().NarrowWidgetPanel() {
		if n.overlay != nil {
			tyde.Instance().HideOverlay(n.overlay)
		}
		return
	}

	fyne.Do(func() {
		var items []fyne.CanvasObject
		for _, item := range list.Objects {
			if item == n.renderer {
				continue
			}
			items = append(items, item)
		}
		list.Objects = items
		list.Refresh()
	})
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
