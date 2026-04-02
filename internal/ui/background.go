package ui

import (
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/FyshOS/backgrounds"

	"fyshos.com/fynedesk"
)

type background struct {
	widget.BaseWidget
	compositor *CompositorWidget
}

func (b *background) CreateRenderer() fyne.WidgetRenderer {
	c := container.NewStack(b.loadModules()...)
	return widget.NewSimpleRenderer(c)
}

func (b *background) loadModules() []fyne.CanvasObject {
	objects := b.screenWallpapers()

	// Add screen area modules (e.g. desktop files)
	for _, m := range fynedesk.Instance().Modules() {
		if deskMod, ok := m.(fynedesk.ScreenAreaModule); ok {
			if wid := deskMod.ScreenAreaWidget(); wid != nil {
				objects = append(objects, wid)
			}
		}
	}

	// Compositor on top so windows are drawn over desktop content
	if b.compositor != nil {
		objects = append(objects, b.compositor)
	}

	return objects
}

// screenWallpapers creates one wallpaper image per screen, positioned correctly
// within the multi-screen window, mirroring the X root background layout.
func (b *background) screenWallpapers() []fyne.CanvasObject {
	inst := fynedesk.Instance()
	if inst == nil {
		return nil
	}

	screens := inst.Screens()
	if screens == nil {
		return nil
	}

	// Find the window origin (top-left of all screens bounding box)
	originX, originY := 0, 0
	for _, screen := range screens.Screens() {
		if screen.X < originX {
			originX = screen.X
		}
		if screen.Y < originY {
			originY = screen.Y
		}
	}

	scale := screens.Primary().CanvasScale()
	wallpapers := container.NewWithoutLayout()

	for _, screen := range screens.Screens() {
		img := loadWallpaper()
		img.Move(fyne.NewPos(
			float32(screen.X-originX)/scale,
			float32(screen.Y-originY)/scale,
		))
		img.Resize(fyne.NewSize(
			float32(screen.Width)/scale,
			float32(screen.Height)/scale,
		))
		wallpapers.Add(img)
	}

	return []fyne.CanvasObject{wallpapers}
}

func (b *background) updateBackground(_ string) {
	b.Refresh()
}

func loadWallpaper() fyne.CanvasObject {
	path := ""
	inst := fynedesk.Instance()
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

func newBackground(compositor *CompositorWidget) *background {
	ret := &background{compositor: compositor}
	ret.ExtendBaseWidget(ret)
	return ret
}
