package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriteUserFace_ReencodesAsPNG(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// A tiny JPEG that should be decoded and re-saved as PNG at ~/.face.
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	src.Set(1, 1, color.RGBA{R: 200, G: 100, B: 50, A: 255})
	var buf bytes.Buffer
	assert.NoError(t, jpeg.Encode(&buf, src, nil))

	assert.NoError(t, writeUserFace(&buf))

	data, err := os.ReadFile(filepath.Join(home, ".face"))
	assert.NoError(t, err)
	// PNG signature.
	assert.Equal(t, []byte{0x89, 'P', 'N', 'G'}, data[:4])
}

func TestPasswdError(t *testing.T) {
	assert.Equal(t, "Authentication token manipulation error",
		passwdError("passwd: Authentication token manipulation error\n"))
	assert.Equal(t, "BAD PASSWORD: it is too short",
		passwdError("Retype new password: \nBAD PASSWORD: it is too short\n"))
	// Nothing recognisable falls back to the generic hint.
	assert.Contains(t, passwdError("Changing password for bob.\n"), "not changed")
}
