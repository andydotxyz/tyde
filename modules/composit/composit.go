package composit

import (
	"fyne.io/fyne/v2"
	"fyshos.com/fynedesk"
)

var compMeta = fynedesk.ModuleMetadata{
	Name:        "Compositor",
	NewInstance: newCompositor,
}

type comp struct {
	done    chan struct{}
	widget  *compositorWidget
	overlay *compositorWidget // fullscreen windows drawn above desktop chrome
}

func (c *comp) Destroy() {
	c.disable()
}

func (c *comp) Metadata() fynedesk.ModuleMetadata {
	return compMeta
}

func (c *comp) ScreenAreaWidget() fyne.CanvasObject {
	return c.widget
}

func (c *comp) OverlayWidget() fyne.CanvasObject {
	return c.overlay
}

func (c *comp) disable() {
	close(c.done)
}

func (c *comp) enable() {
	go func() {
		err := run(c.done, c.widget, c.overlay)
		if err != nil {
			fyne.LogError("Compositor failed", err)
		}
	}()
}

// newCompositor creates a new module that will manage composition of the windows.
func newCompositor() fynedesk.Module {
	c := &comp{
		done:    make(chan struct{}),
		widget:  newCompositorWidget(),
		overlay: newCompositorWidget(),
	}
	c.enable()
	return c
}
