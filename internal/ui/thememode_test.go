package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"github.com/stretchr/testify/assert"
)

// TestThemeModeChoice checks that the tiles show which mode is in use and that
// picking the other one moves the selection and saves the new mode.
func TestThemeModeChoice(t *testing.T) {
	test.NewApp()

	saved := ""
	old := writeThemeMode
	writeThemeMode = func(mode string) error {
		saved = mode
		return nil
	}
	defer func() { writeThemeMode = old }()

	tiles := modeTiles(t, newThemeModeChoice())
	assert.Len(t, tiles, 2)

	current, other := tiles[0], tiles[1]
	if current.mode != currentThemeMode() {
		current, other = other, current
	}
	assert.True(t, current.selected)
	assert.False(t, other.selected)

	other.Tapped(nil)
	assert.Equal(t, other.mode, saved)
	assert.True(t, other.selected)
	assert.False(t, current.selected)
}

// TestThemeModeTileSamplesItsVariant confirms each tile paints itself in its own
// variant's colours rather than the desktop's current ones, so that both tiles
// preview what they offer.
func TestThemeModeTileSamplesItsVariant(t *testing.T) {
	test.NewApp()
	fyne.CurrentApp().Settings().SetTheme(theme.DefaultTheme()) // the test theme has no variants

	light := newThemeModeTile(themeModeLight, "Light", theme.VariantLight, nil)
	test.WidgetRenderer(light) // builds the tile, so the ring exists

	th := variantTheme()
	assert.Equal(t, th.Color(theme.ColorNameBackground, theme.VariantLight), light.ring.FillColor)
	assert.NotEqual(t, th.Color(theme.ColorNameBackground, theme.VariantDark), light.ring.FillColor)
}

// modeTiles pulls the tiles out of the container built by newThemeModeChoice.
func modeTiles(t *testing.T, o fyne.CanvasObject) []*themeModeTile {
	t.Helper()

	group, ok := o.(*fyne.Container)
	if !ok {
		t.Fatalf("expected a container of tiles, got %T", o)
	}

	var tiles []*themeModeTile
	for _, child := range group.Objects {
		tile, ok := child.(*themeModeTile)
		if !ok {
			t.Fatalf("expected a theme mode tile, got %T", child)
		}
		tiles = append(tiles, tile)
	}
	return tiles
}
