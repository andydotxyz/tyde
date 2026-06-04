package esheep

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/gif"
	"sync"

	"fyne.io/fyne/v2"
)

// StraySheepPoe.gif is the original "Stray Sheep" (eSheep) sprite rip.
// Artwork credit: LiL_Stenly (see assets/ATTRIBUTION.md). The sheet is a
// 16x12 grid of 40x40 pixel tiles on a solid blue (0,0,255) background, with
// the top tile-row reserved for a title banner. We colour-key the blue to
// transparent and slice out the tiles we animate.
//
//go:embed assets/StraySheepPoe.gif
var sheepSheetGIF []byte

const (
	tileSize = 40 // pixels per sprite tile in the source sheet
)

// cell identifies a tile by its column and row in the source grid.
type cell struct{ col, row int }

// poses is the parsed, ready-to-display set of animation frames. The *Right
// slices are produced by horizontally flipping their *Left counterparts so the
// two directions always match.
type poses struct {
	walkLeft   []image.Image
	walkRight  []image.Image
	tumble     []image.Image // a fall
	grazeLeft  []image.Image // side-on eating bob: head up, then head down with mouth open
	grazeRight []image.Image
	splat      []image.Image // "fall death": front-on, plus-sign eyes, limbs splayed
	jumpLeft   image.Image
	jumpRight  image.Image
	daisies    []image.Image // daisy plant, most flowers (full) to none (eaten)
}

// Frame selections, hand-picked from the sheet (see the grid analysis in the
// implementation notes). Row 0 is the banner so all rows are >= 1. All sheep
// poses here face left; right-facing variants are mirrored at load time.
var (
	walkLeftCells = []cell{{0, 1}, {2, 1}, {1, 1}, {3, 1}}
	tumbleCells   = []cell{{0, 8}, {4, 8}, {8, 8}, {12, 8}}
	grazeCells    = []cell{{3, 1}, {5, 3}} // standing mouth-closed, then head-down mouth-open bite; identical feet so only the head/mouth move
	splatCells    = []cell{{5, 6}}         // front-on KO: plus-sign eyes, legs splayed
	jumpCell      = cell{2, 1}
	// daisy plant states ordered full -> empty (4,3,2,1,0 flowers); each chomp
	// advances one frame, and the sheep is done once the plant is bare.
	daisyCells = []cell{{9, 10}, {5, 10}, {6, 10}, {7, 10}, {8, 10}}
)

var (
	loadedPoses *poses
	loadOnce    sync.Once
)

// sheepPoses lazily decodes and slices the embedded sprite sheet, returning a
// shared (read-only) set of animation frames.
func sheepPoses() *poses {
	loadOnce.Do(func() {
		sheet, err := decodeSheet()
		if err != nil {
			fyne.LogError("eSheep: failed to decode sprite sheet", err)
			loadedPoses = &poses{} // empty - module renders nothing
			return
		}

		tile := func(c cell) image.Image {
			return crop(sheet, c.col*tileSize, c.row*tileSize, tileSize, tileSize)
		}

		// tilesLeft slices the given cells; tilesRight mirrors them.
		tilesLeft := func(cells []cell) []image.Image {
			out := make([]image.Image, len(cells))
			for i, c := range cells {
				out[i] = tile(c)
			}
			return out
		}
		mirror := func(frames []image.Image) []image.Image {
			out := make([]image.Image, len(frames))
			for i, f := range frames {
				out[i] = flipH(f)
			}
			return out
		}

		left := tilesLeft(walkLeftCells)
		graze := tilesLeft(grazeCells)
		jump := tile(jumpCell)

		loadedPoses = &poses{
			walkLeft:   left,
			walkRight:  mirror(left),
			tumble:     tilesLeft(tumbleCells),
			grazeLeft:  graze,
			grazeRight: mirror(graze),
			splat:      tilesLeft(splatCells),
			jumpLeft:   jump,
			jumpRight:  flipH(jump),
			daisies:    tilesLeft(daisyCells),
		}
	})
	return loadedPoses
}

// decodeSheet decodes the embedded GIF and colour-keys the blue background to
// transparent, returning an RGBA image.
func decodeSheet() (*image.RGBA, error) {
	src, err := gif.Decode(bytes.NewReader(sheepSheetGIF))
	if err != nil {
		return nil, err
	}

	b := src.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, g, bl, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			// Convert from 16-bit to 8-bit components.
			r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(bl>>8)
			if isBackground(r8, g8, b8) {
				continue // leave fully transparent
			}
			out.SetRGBA(x, y, color.RGBA{R: r8, G: g8, B: b8, A: uint8(a >> 8)})
		}
	}
	return out, nil
}

// isBackground reports whether a colour is the sheet's blue key colour.
func isBackground(r, g, b uint8) bool {
	const tol = 24
	return r < tol && g < tol && b > 255-tol
}

// crop returns the sub-rectangle of img as an independent image.
func crop(img *image.RGBA, x, y, w, h int) image.Image {
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			out.SetRGBA(i, j, img.RGBAAt(x+i, y+j))
		}
	}
	return out
}

// flipH returns a horizontally mirrored copy of img.
func flipH(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			out.Set(w-1-i, j, img.At(b.Min.X+i, b.Min.Y+j))
		}
	}
	return out
}
