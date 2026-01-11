package deskwidgets

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func newBar(max float32, data func() float32) fyne.CanvasObject {
	bg := canvas.NewRectangle(theme.ButtonColor())
	bg.SetMinSize(fyne.NewSize(32, 32))
	bar := canvas.NewRectangle(theme.BackgroundColor())
	label := widget.NewLabel("0.0")
	label.TextStyle.Monospace = true

	go func() {
		for {
			fyne.Do(func() {
				val := data()
				label.SetText(fmt.Sprintf("%0.1f", val))

				height := bg.Size().Height
				barHeight := height * (val / max)
				bar.Resize(fyne.NewSize(bg.Size().Width, barHeight))
				bar.Move(fyne.NewPos(0, height-barHeight))
			})

			time.Sleep(time.Second)
		}
	}()

	outer := container.NewStack(bg, container.NewWithoutLayout(bar), container.NewCenter(label))
	return newDeskWidget(outer)
}
