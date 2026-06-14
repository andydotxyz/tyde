//go:build linux || openbsd || freebsd || netbsd
// +build linux openbsd freebsd netbsd

package composit

import (
	"testing"
)

// convertNaive is a straightforward, independent reference implementation of
// the BGRA->premultiplied-RGBA conversion, used as a correctness oracle and as
// a benchmark baseline for the optimized convertZPixmap.
func convertNaive(pix, data []byte, argb bool) {
	n := len(pix)
	if len(data) < n {
		n = len(data)
	}
	n -= n % 4
	for i := 0; i < n; i += 4 {
		if argb {
			a := uint32(data[i+3])
			pix[i] = uint8((uint32(data[i+2])*a + 127) / 255)
			pix[i+1] = uint8((uint32(data[i+1])*a + 127) / 255)
			pix[i+2] = uint8((uint32(data[i])*a + 127) / 255)
			pix[i+3] = data[i+3]
		} else {
			pix[i] = data[i+2]
			pix[i+1] = data[i+1]
			pix[i+2] = data[i]
			pix[i+3] = 0xff
		}
	}
}

func sampleZPixmap(w, h int) []byte {
	data := make([]byte, w*h*4)
	for i := range data {
		data[i] = byte(i*7 + 3) // deterministic, varies across all 4 bytes
	}
	return data
}

func TestConvertZPixmapMatchesNaive(t *testing.T) {
	for _, argb := range []bool{true, false} {
		src := sampleZPixmap(64, 48)
		want := make([]byte, len(src))
		got := make([]byte, len(src))

		convertNaive(want, src, argb)
		convertZPixmap(got, src, argb)

		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("argb=%v: byte %d = %d, want %d", argb, i, got[i], want[i])
			}
		}
	}
}

func TestConvertZPixmapShortBuffers(t *testing.T) {
	// dst smaller than src: only dst-many whole pixels are converted, no panic.
	src := sampleZPixmap(10, 1)
	dst := make([]byte, 5*4)
	convertZPixmap(dst, src, false)
	for i := 0; i < len(dst); i += 4 {
		if dst[i] != src[i+2] || dst[i+3] != 0xff {
			t.Fatalf("pixel %d not converted correctly", i/4)
		}
	}

	// Non-pixel-multiple tail is ignored rather than read out of range.
	convertZPixmap(make([]byte, 7), sampleZPixmap(2, 1), true) // must not panic
}

func benchConvert(b *testing.B, fn func(dst, src []byte, argb bool)) {
	const w, h = 1920, 1080 // a typical maximised Chrome window
	src := sampleZPixmap(w, h)
	dst := make([]byte, len(src))
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fn(dst, src, false) // opaque path — the common case (e.g. Chrome)
	}
}

func BenchmarkConvertNaive(b *testing.B)   { benchConvert(b, convertNaive) }
func BenchmarkConvertZPixmap(b *testing.B) { benchConvert(b, convertZPixmap) }
