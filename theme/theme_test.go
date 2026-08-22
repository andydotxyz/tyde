package theme

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	_ "fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"

	"github.com/stretchr/testify/assert"
)

func TestIconResources(t *testing.T) {
	assert.NotNil(t, BatteryIcon.Name())
	assert.NotNil(t, BrightnessIcon.Name())
	assert.NotNil(t, SoundHighIcon.Name())
	assert.NotNil(t, MuteIcon.Name())
}

func TestIconTheme(t *testing.T) {
	th := &testTheme{fg: color.White}
	fyne.CurrentApp().Settings().SetTheme(th)
	battDark := BatteryIcon.Content()

	th.fg = color.Black
	assert.NotEqual(t, battDark, BatteryIcon.Content())
}

func TestIconTheme_BrokenImage(t *testing.T) {
	assert.NotNil(t, BrokenImageIcon) // must not be nil as we fall back
}

type testTheme struct {
	fyne.Theme

	fg color.Color
}

func (t *testTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	if n == theme.ColorNameForeground {
		return t.fg
	}

	return t.Theme.Color(n, v)
}

func TestSetTouchScreen(t *testing.T) {
	defer SetTouchScreen(false)

	SetTouchScreen(true)
	// big enough to tap - the buttons grow with the bar they sit in
	assert.Greater(t, TitleHeight, titleHeight)
	assert.Greater(t, TitleButtonHeight, titleButtonHeight)
	assert.Greater(t, TitleButtonIconSize, titleButtonIconSize)
	assert.Greater(t, ButtonWidth, buttonWidth)
	assert.Less(t, TitleButtonHeight, TitleHeight) // still fits in the bar

	SetTouchScreen(false)
	assert.Equal(t, titleHeight, TitleHeight)
	assert.Equal(t, titleButtonHeight, TitleButtonHeight)
	assert.Equal(t, titleButtonIconSize, TitleButtonIconSize)
	assert.Equal(t, buttonWidth, ButtonWidth)
}
