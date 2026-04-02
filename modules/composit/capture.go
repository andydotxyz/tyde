package composit

import (
	"image"
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
