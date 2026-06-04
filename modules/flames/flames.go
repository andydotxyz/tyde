package flames

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"fyshos.com/tyde"
)

var flamesMeta = tyde.ModuleMetadata{
	Name:        "Flames",
	NewInstance: newFlames,
}

// flames is a screen area module that overlays animated flames across the
// whole screen, drawn over the top of the desktop background and its contents.
type flames struct {
	shader *canvas.Shader
	anim   *fyne.Animation
}

// ScreenAreaWidget returns the full-screen flame overlay. The shader leaves
// everything above the flames transparent, so the desktop shows through.
//
// The shader and its animation are created and started once and then reused:
// this method can be called again whenever the background is rebuilt, and
// starting a fresh animation each time would orphan the previous one - and the
// CPU it burns - still running, since only Stop (called from Destroy) halts it.
func (f *flames) ScreenAreaWidget() fyne.CanvasObject {
	if f.shader == nil {
		f.shader = canvas.NewShader("tydeFlames", flameShader, flameShaderES)
		f.anim = canvas.NewShaderAnimation(f.shader)
		f.anim.Start()
	}
	return f.shader
}

func (f *flames) Metadata() tyde.ModuleMetadata {
	return flamesMeta
}

func (f *flames) Destroy() {
	if f.anim != nil {
		f.anim.Stop()
	}
}

// newFlames creates a new module that overlays animated flames on the screen.
func newFlames() tyde.Module {
	return &flames{}
}
