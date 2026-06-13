package ui

import (
	"fmt"
	"image/color"
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

	wallpaper *fyne.Container // holds the current wallpaper so it can be rebuilt live
}

func (b *background) CreateRenderer() fyne.WidgetRenderer {
	c := container.NewStack(b.loadModules()...)
	return widget.NewSimpleRenderer(c)
}

func (b *background) loadModules() []fyne.CanvasObject {
	b.wallpaper = container.NewStack(loadWallpaper())
	objects := []fyne.CanvasObject{b.wallpaper}

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
	if b.wallpaper != nil {
		b.wallpaper.Objects = []fyne.CanvasObject{loadWallpaper()}
		b.wallpaper.Refresh()
	}
}

func loadWallpaper() fyne.CanvasObject {
	path := ""
	fill := ""
	colorHex := ""
	inst := tyde.Instance()
	if inst != nil {
		path = inst.Settings().Background()
		fill = inst.Settings().BackgroundFill()
		colorHex = inst.Settings().BackgroundColor()
	}

	if path != "" {
		if stat, err := os.Stat(path); err == nil && stat.Mode().IsRegular() {
			img := canvas.NewImageFromFile(path)
			img.ScaleMode = canvas.ImageScaleFastest
			img.FillMode = backgroundFillMode(fill)

			bg := canvas.NewRectangle(ParseHexColor(colorHex))
			return container.NewStack(bg, img)
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

// backgroundFillModes lists the user-facing fill options in display order.
var backgroundFillModes = []string{"Stretch", "Fit", "Fill"}

// backgroundFillMode maps a user-facing fill name to a canvas fill mode.
func backgroundFillMode(name string) canvas.ImageFill {
	switch name {
	case "Fit":
		return canvas.ImageFillContain
	case "Fill":
		return canvas.ImageFillCover
	default: // "Stretch"
		return canvas.ImageFillStretch
	}
}

// ParseHexColor turns a "#rrggbb" or "#rrggbbaa" string into a colour,
// falling back to opaque black for empty or malformed input.
func ParseHexColor(hex string) color.NRGBA {
	c := color.NRGBA{A: 0xff}
	if len(hex) == 0 || hex[0] != '#' {
		return c
	}

	switch len(hex) {
	case 7: // #rrggbb
		_, err := fmt.Sscanf(hex, "#%02x%02x%02x", &c.R, &c.G, &c.B)
		if err != nil {
			return color.NRGBA{A: 0xff}
		}
	case 9: // #rrggbbaa
		_, err := fmt.Sscanf(hex, "#%02x%02x%02x%02x", &c.R, &c.G, &c.B, &c.A)
		if err != nil {
			return color.NRGBA{A: 0xff}
		}
	}
	return c
}

// HexColor formats a colour as a "#rrggbbaa" string for storage.
func HexColor(c color.Color) string {
	r, g, b, a := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x%02x", uint8(r>>8), uint8(g>>8), uint8(b>>8), uint8(a>>8))
}
