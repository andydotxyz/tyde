//go:build linux || openbsd || freebsd || netbsd
// +build linux openbsd freebsd netbsd

package wm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScreensFromOutputs_SideBySide(t *testing.T) {
	screens, primary := screensFromOutputs([]screenOutput{
		{name: "HDMI-1", x: 1920, y: 0, width: 1920, height: 1080, scale: 1},
		{name: "eDP-1", x: 0, y: 0, width: 1920, height: 1080, scale: 2, primary: true},
	})

	assert.Equal(t, 2, len(screens))
	assert.Equal(t, "eDP-1", screens[0].Name)
	assert.Equal(t, "HDMI-1", screens[1].Name)
	assert.Equal(t, screens[0], primary)
}

func TestScreensFromOutputs_Mirrored(t *testing.T) {
	screens, primary := screensFromOutputs([]screenOutput{
		{name: "HDMI-1", x: 0, y: 0, width: 1920, height: 1080, scale: 1},
		{name: "eDP-1", x: 0, y: 0, width: 1920, height: 1080, scale: 2, primary: true},
	})

	// One screen over the pixels both outputs show, named and scaled for the
	// primary output even though it was not the first one seen.
	assert.Equal(t, 1, len(screens))
	assert.Equal(t, "eDP-1", screens[0].Name)
	assert.Equal(t, float32(2), screens[0].Scale)
	assert.Equal(t, 1920, screens[0].Width)
	assert.Equal(t, 1080, screens[0].Height)
	assert.Equal(t, screens[0], primary)
}

func TestScreensFromOutputs_MirroredDifferentModes(t *testing.T) {
	screens, _ := screensFromOutputs([]screenOutput{
		{name: "eDP-1", x: 0, y: 0, width: 1280, height: 1024, scale: 1, primary: true},
		{name: "HDMI-1", x: 0, y: 0, width: 1920, height: 1080, scale: 1},
	})

	// The desktop covers the largest mirrored mode, as the X screen does.
	assert.Equal(t, 1, len(screens))
	assert.Equal(t, "eDP-1", screens[0].Name)
	assert.Equal(t, 1920, screens[0].Width)
	assert.Equal(t, 1080, screens[0].Height)
}

func TestScreensFromOutputs_NoPrimary(t *testing.T) {
	screens, primary := screensFromOutputs([]screenOutput{
		{name: "HDMI-1", x: 1920, y: 0, width: 1920, height: 1080, scale: 1},
		{name: "HDMI-2", x: 0, y: 0, width: 1920, height: 1080, scale: 1},
	})

	assert.Equal(t, 2, len(screens))
	assert.Equal(t, screens[0], primary)
}

func TestScreensFromOutputs_None(t *testing.T) {
	screens, primary := screensFromOutputs(nil)

	assert.Equal(t, 0, len(screens))
	assert.Nil(t, primary)
}
