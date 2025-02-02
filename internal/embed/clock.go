package embed

import (
	_ "embed"
	"image/color"

	clockExample "github.com/fyne-io/examples/clock"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
)

//go:embed "icons/clock.svg"
var clockIcon []byte

type clock struct {
	app
}

func newClock(multi *container.MultipleWindows) *clock {
	c := &clock{}
	c.m = multi
	c.name = "Clock"
	c.categories = []string{"utility"}
	c.icon = fyne.NewStaticResource("clock.svg", clockIcon)
	c.makeContent = c.makeUI
	return c
}

func (c *clock) makeUI() fyne.CanvasObject {
	dummy := test.NewWindow(canvas.NewRectangle(color.Transparent))
	return clockExample.Show(dummy)
}
