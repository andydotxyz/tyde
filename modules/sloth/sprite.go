package sloth

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/png"
	"sync"

	"golang.org/x/image/draw"

	"fyne.io/fyne/v2"
)

// sloth.png is a sleeping sloth drawn on a white studio background. We key that
// background out to transparency and trim the result so the sprite is just the
// animal.
//
//go:embed assets/sloth.png
var slothPNG []byte

const (
	// paperLevel is the darkest channel value that still counts as the white
	// background. The artwork's palest ink (the claws) is well below this.
	paperLevel = 228
	// featherSpan softens the cut: kept pixels this close to paperLevel fade out
	// so the ink does not end in a hard white fringe.
	featherSpan = 34
)

var (
	loadOnce  sync.Once
	loadedImg image.Image
)

// scaled returns a copy of img resampled to w x h. The artwork is far bigger
// than we ever draw it and Fyne resamples an image's source every time it
// uploads the texture - which the compositor makes it do as it repaints - so we
// pay for that once here instead of on every frame.
func scaled(img image.Image, w, h int) image.Image {
	if w <= 0 || h <= 0 {
		return img
	}

	out := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(out, out.Bounds(), img, img.Bounds(), draw.Over, nil)
	return out
}

// slothSprite lazily decodes the embedded artwork into a transparent, trimmed
// image. It returns nil if the asset cannot be read, in which case the module
// simply draws nothing.
func slothSprite() image.Image {
	loadOnce.Do(func() {
		src, err := png.Decode(bytes.NewReader(slothPNG))
		if err != nil {
			fyne.LogError("sloth: failed to decode artwork", err)
			return
		}
		loadedImg = trim(keyBackground(src))
	})
	return loadedImg
}

// keyBackground makes the white paper around the sloth transparent. The paper is
// found by flooding in from the edges of the image so pale colours enclosed by
// the drawing's outline (the belly, the claws) are left alone.
func keyBackground(src image.Image) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewRGBA(image.Rect(0, 0, w, h))

	// mins[i] caches the darkest channel of each pixel: low for ink, high for paper.
	mins := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, _ := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(bl>>8)
			m := r8
			if g8 < m {
				m = g8
			}
			if b8 < m {
				m = b8
			}
			mins[y*w+x] = m
			out.SetRGBA(x, y, color.RGBA{R: r8, G: g8, B: b8, A: 255})
		}
	}

	paper := floodPaper(mins, w, h)

	for i, isPaper := range paper {
		x, y := i%w, i/w
		if isPaper {
			out.SetRGBA(x, y, color.RGBA{})
			continue
		}
		// Fade out the paper-side edge of the ink, otherwise the sprite carries a
		// white halo onto whatever it is sitting in front of.
		if m := mins[i]; m > paperLevel-featherSpan && touchesPaper(paper, w, h, x, y) {
			c := out.RGBAAt(x, y)
			c.A = uint8(255 * int(paperLevel-m) / featherSpan)
			out.SetRGBA(x, y, c)
		}
	}
	return out
}

// floodPaper flood-fills the pale background inwards from the image edges,
// returning a mask of the pixels that belong to it.
func floodPaper(mins []uint8, w, h int) []bool {
	paper := make([]bool, w*h)
	var queue []int

	push := func(x, y int) {
		if x < 0 || y < 0 || x >= w || y >= h {
			return
		}
		i := y*w + x
		if paper[i] || mins[i] < paperLevel {
			return
		}
		paper[i] = true
		queue = append(queue, i)
	}

	for x := 0; x < w; x++ {
		push(x, 0)
		push(x, h-1)
	}
	for y := 0; y < h; y++ {
		push(0, y)
		push(w-1, y)
	}
	for len(queue) > 0 {
		i := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		x, y := i%w, i/w
		push(x-1, y)
		push(x+1, y)
		push(x, y-1)
		push(x, y+1)
	}
	return paper
}

// touchesPaper reports whether a pixel borders the flooded background.
func touchesPaper(paper []bool, w, h, x, y int) bool {
	for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
		nx, ny := x+d[0], y+d[1]
		if nx < 0 || ny < 0 || nx >= w || ny >= h {
			continue
		}
		if paper[ny*w+nx] {
			return true
		}
	}
	return false
}

// trim crops away the fully transparent margin so the sprite's bounds are the
// sloth itself - the perch maths measures from its edges.
func trim(img *image.RGBA) image.Image {
	b := img.Bounds()
	minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X-1, b.Min.Y-1
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.RGBAAt(x, y).A == 0 {
				continue
			}
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	if maxX < minX || maxY < minY {
		return img // nothing survived the key; hand back what we have
	}

	out := image.NewRGBA(image.Rect(0, 0, maxX-minX+1, maxY-minY+1))
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			out.SetRGBA(x-minX, y-minY, img.RGBAAt(x, y))
		}
	}
	return out
}
