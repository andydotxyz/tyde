//go:build linux || openbsd || freebsd || netbsd
// +build linux openbsd freebsd netbsd

package wm

import (
	"testing"

	"github.com/BurntSushi/xgb/xproto"
	"github.com/stretchr/testify/assert"

	"fyshos.com/tyde"
)

func TestUnclaimedScreen(t *testing.T) {
	screens := []*tyde.Screen{
		{Name: "Primary", Width: 1920, Height: 1080},
		{Name: "Screen 1", X: 1920, Width: 1280, Height: 1024},
	}

	claimed := map[xproto.Window]string{}
	assert.Equal(t, screens[0], unclaimedScreen(screens, claimed))

	claimed[xproto.Window(1)] = "Primary"
	assert.Equal(t, screens[1], unclaimedScreen(screens, claimed))

	claimed[xproto.Window(2)] = "Screen 1"
	assert.Nil(t, unclaimedScreen(screens, claimed))

	// a screen that went away does not hold a claim on the ones that remain
	assert.Equal(t, screens[1], unclaimedScreen(screens[1:], map[xproto.Window]string{
		xproto.Window(1): "Screen 2",
	}))
}
