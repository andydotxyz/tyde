package ui

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"  // register decoders so renderWallpaper can read any wallpaper
	_ "image/jpeg" // ...
	_ "image/png"  // ...
	"log"
	"math"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/FyshOS/backgrounds"
	xdraw "golang.org/x/image/draw"

	"fyshos.com/tyde"
)

type background struct {
	widget.BaseWidget

	wallpaper *fyne.Container // holds the current wallpaper so it can be rebuilt live
}

func (b *background) CreateRenderer() fyne.WidgetRenderer {
	b.wallpaper = container.NewStack(b.loadModules()...)
	return widget.NewSimpleRenderer(b.wallpaper)
}

func (b *background) loadModules() []fyne.CanvasObject {
	objects := []fyne.CanvasObject{loadWallpaper()}

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

// updateBackground rebuilds the background content - the wallpaper and the
// screen area module overlays.
func (b *background) updateBackground(_ string) {
	if b.wallpaper != nil {
		b.wallpaper.Objects = b.loadModules()
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

// renderWallpaper rasterises the current wallpaper into an opaque RGBA image of
// the given pixel size, honouring the configured background colour and fill
// mode. It mirrors loadWallpaper's settings so a synthesised desktop face (used
// by the cube transition before a desktop has been captured live) matches the
// real background. The colour fill always backs the image so "Fit" letterboxing
// reads correctly; a missing or unreadable wallpaper just leaves that fill.
func renderWallpaper(w, h int) *image.RGBA {
	if w <= 0 || h <= 0 {
		return nil
	}
	inst := tyde.Instance()
	if inst == nil {
		return nil
	}

	out := image.NewRGBA(image.Rect(0, 0, w, h))
	bg := ParseHexColor(inst.Settings().BackgroundColor())
	draw.Draw(out, out.Bounds(), &image.Uniform{C: bg}, image.Point{}, draw.Src)

	path := inst.Settings().Background()
	if path == "" {
		return out
	}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return out
	}

	xdraw.CatmullRom.Scale(out, wallpaperRect(src.Bounds(), w, h, inst.Settings().BackgroundFill()),
		src, src.Bounds(), draw.Over, nil)
	return out
}

// wallpaperRect returns the destination rectangle for the wallpaper within a
// w×h image for the given fill mode, matching backgroundFillMode: Fit scales to
// fit inside (letterboxed), Fill scales to cover (overflow clipped by Scale),
// and the default stretches to the full size.
func wallpaperRect(src image.Rectangle, w, h int, fill string) image.Rectangle {
	sw, sh := src.Dx(), src.Dy()
	if sw <= 0 || sh <= 0 {
		return image.Rect(0, 0, w, h)
	}

	var scale float64
	switch fill {
	case "Fit":
		scale = math.Min(float64(w)/float64(sw), float64(h)/float64(sh))
	case "Fill":
		scale = math.Max(float64(w)/float64(sw), float64(h)/float64(sh))
	default: // Stretch
		return image.Rect(0, 0, w, h)
	}

	dw, dh := int(float64(sw)*scale), int(float64(sh)*scale)
	ox, oy := (w-dw)/2, (h-dh)/2
	return image.Rect(ox, oy, ox+dw, oy+dh)
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
