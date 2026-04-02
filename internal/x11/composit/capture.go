//go:build linux || openbsd || freebsd || netbsd
// +build linux openbsd freebsd netbsd

package composit

import (
	"image"
	"image/color"
	"math"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
)

// capturePixmap reads the contents of an X pixmap into a Go NRGBA image.
// If argb is true, the alpha channel from the pixmap is preserved;
// otherwise alpha is set to 0xff (fully opaque).
func capturePixmap(conn *xgb.Conn, drawable xproto.Drawable, w, h uint16, argb bool) *image.NRGBA {
	if w == 0 || h == 0 {
		return nil
	}

	reply, err := xproto.GetImage(conn, xproto.ImageFormatZPixmap, drawable,
		0, 0, w, h, math.MaxUint32).Reply()
	if err != nil || reply == nil {
		return nil
	}

	img := image.NewNRGBA(image.Rect(0, 0, int(w), int(h)))
	data := reply.Data
	pix := img.Pix

	// X11 ZPixmap format: 4 bytes per pixel in BGRx or BGRA order
	expectedLen := int(w) * int(h) * 4
	if len(data) < expectedLen {
		return nil
	}

	for i := 0; i < expectedLen; i += 4 {
		pix[i] = data[i+2]   // R (from X's position 2)
		pix[i+1] = data[i+1] // G
		pix[i+2] = data[i]   // B (from X's position 0)
		if argb {
			pix[i+3] = data[i+3] // A from pixmap
		} else {
			pix[i+3] = 0xff // force opaque
		}
	}

	return img
}

// roundCorners clears pixels outside a circular arc of the given radius
// at each corner of the image, making them fully transparent.
func roundCorners(img *image.NRGBA, radius int) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if radius <= 0 || w < radius*2 || h < radius*2 {
		return
	}

	transparent := color.NRGBA{}
	r2 := float64(radius) * float64(radius)
	for dy := 0; dy < radius; dy++ {
		for dx := 0; dx < radius; dx++ {
			cx := float64(radius-dx) - 0.5
			cy := float64(radius-dy) - 0.5
			if cx*cx+cy*cy > r2 {
				img.SetNRGBA(dx, dy, transparent)         // top-left
				img.SetNRGBA(w-1-dx, dy, transparent)     // top-right
				img.SetNRGBA(dx, h-1-dy, transparent)     // bottom-left
				img.SetNRGBA(w-1-dx, h-1-dy, transparent) // bottom-right
			}
		}
	}
}
