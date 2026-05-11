package ui

import (
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatAuthors(t *testing.T) {
	out := formatAuthors("Alice\nBob\n")
	assert.Equal(t, "### Authors\n\n* Alice\n* Bob\n", out)
}

func TestFormatAuthors_SkipsBlankLines(t *testing.T) {
	out := formatAuthors("Alice\n\nBob")
	assert.Equal(t, "### Authors\n\n* Alice\n* Bob\n", out)
}

func TestFormatAuthors_Empty(t *testing.T) {
	out := formatAuthors("")
	assert.Equal(t, "### Authors\n\n", out)
}

func TestWithAlpha(t *testing.T) {
	c := withAlpha(color.NRGBA{R: 10, G: 20, B: 30, A: 255}, 128)
	out := c.(color.NRGBA)
	assert.Equal(t, uint8(10), out.R)
	assert.Equal(t, uint8(20), out.G)
	assert.Equal(t, uint8(30), out.B)
	assert.Equal(t, uint8(128), out.A)
}

func TestVersion(t *testing.T) {
	assert.NotEmpty(t, version())
}
