package ui

import (
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/FyshOS/backgrounds"

	"fyshos.com/tyde"
)

type background struct {
	widget.BaseWidget
}

func (b *background) CreateRenderer() fyne.WidgetRenderer {
	c := container.NewStack(b.loadModules()...)
	return widget.NewSimpleRenderer(c)
}

func (b *background) loadModules() []fyne.CanvasObject {
	objects := []fyne.CanvasObject{container.NewStack(loadWallpaper())}

	// Add screen area modules (e.g. desktop files)
	for _, m := range tyde.Instance().Modules() {
		if deskMod, ok := m.(tyde.ScreenAreaModule); ok {
			if wid := deskMod.ScreenAreaWidget(); wid != nil {
				objects = append(objects, wid)
			}
		}
	}

	return objects
}

func (b *background) updateBackground(_ string) {
	b.Refresh()
}

func loadWallpaper() fyne.CanvasObject {
	path := ""
	inst := tyde.Instance()
	if inst != nil {
		path = inst.Settings().Background()
	}

	if path != "" {
		if stat, err := os.Stat(path); err == nil && stat.Mode().IsRegular() {
			img := canvas.NewImageFromFile(path)
			img.ScaleMode = canvas.ImageScaleFastest
			return img
		}
	}

	set := fyne.CurrentApp().Settings()
	src := backgrounds.Default()
	return src.Load(set.Theme(), set.ThemeVariant())
}

func newBackground() *background {
	ret := &background{}
	ret.ExtendBaseWidget(ret)
	return ret
}
