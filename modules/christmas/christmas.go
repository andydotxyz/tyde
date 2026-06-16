package christmas

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"fyshos.com/tyde"
	wmtheme "fyshos.com/tyde/theme"
)

var christmasMeta = tyde.ModuleMetadata{
	Name:        "Christmas",
	NewInstance: newLights,
}

// christmas is a screen area module that hangs a string of animated fairy christmas
// around the edge of the desktop content area, drawn over the background and its
// contents.
type christmas struct {
	shader *canvas.Shader
	anim   *fyne.Animation
}

// ScreenAreaWidget returns the fairy christmas overlay. It spans the full screen
// height but is inset horizontally by the bar and widget panel, so the string
// runs along the screen's top and bottom edges and right up to the inner edges
// of the bars - touching the content area, not hidden under the bars. The shader
// leaves everything away from the string transparent, so the desktop shows
// through.
//
// The shader and its animation are created and started once and then reused:
// this method can be called again whenever the background is rebuilt, and
// starting a fresh animation each time would orphan the previous one - and the
// CPU it burns - still running, since only Stop (called from Destroy) halts it.
func (c *christmas) ScreenAreaWidget() fyne.CanvasObject {
	if c.shader == nil {
		c.shader = canvas.NewShader("tydeLights", lightsShader, lightsShaderES)
		c.anim = canvas.NewShaderAnimation(c.shader)
		c.anim.Start()
	}

	desk := tyde.Instance()
	barPad := canvas.NewRectangle(color.Transparent)
	barPad.SetMinSize(fyne.NewSize(wmtheme.NarrowBarWidth, 1))

	rightIndent := wmtheme.WidgetPanelWidth
	if desk.Settings().NarrowWidgetPanel() {
		rightIndent = wmtheme.NarrowBarWidth
	}
	widgetPad := canvas.NewRectangle(color.Transparent)
	widgetPad.SetMinSize(fyne.NewSize(rightIndent, 1))

	return container.NewBorder(nil, nil, barPad, widgetPad, c.shader)
}

func (c *christmas) Metadata() tyde.ModuleMetadata {
	return christmasMeta
}

func (c *christmas) Destroy() {
	if c.anim != nil {
		c.anim.Stop()
	}
}

// newLights creates a new module that overlays animated fairy christmas on the
// desktop content area.
func newLights() tyde.Module {
	return &christmas{}
}
