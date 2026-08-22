package ui

import (
	"encoding/json"
	"image/color"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	deskDriver "fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// The names Fyne stores in its settings file for the theme variant.
const (
	themeModeLight = "light"
	themeModeDark  = "dark"

	themeTileRadius = 10
)

// currentThemeMode reports whether the desktop is currently light or dark.
func currentThemeMode() string {
	if fyne.CurrentApp().Settings().ThemeVariant() == theme.VariantLight {
		return themeModeLight
	}
	return themeModeDark
}

// applyThemeMode switches the desktop between light and dark. The variant lives
// in the Fyne settings file, which the running app watches, so saving it
// restyles the desktop and every Fyne app on it. The save is disk IO so it is
// pushed off the render thread.
func applyThemeMode(mode string) {
	runAsync(func() {
		if err := writeThemeMode(mode); err != nil {
			fyne.LogError("Unable to save theme mode", err)
		}
	})
}

// writeThemeMode records the light/dark choice in the Fyne settings file,
// leaving the other settings (scale, primary colour...) as they were.
// It is a var so tests can capture the choice without touching real settings.
var writeThemeMode = func(mode string) error {
	schema := &app.SettingsSchema{}
	path := schema.StoragePath()
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, schema); err != nil {
			fyne.LogError("Unreadable Fyne settings, replacing", err)
			schema = &app.SettingsSchema{}
		}
	}
	schema.ThemeName = mode

	data, err := json.Marshal(schema)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// newThemeModeChoice builds the light/dark switcher: two tiles previewing the
// desktop in each mode, the active one ringed. Choosing a mode applies it to the
// whole desktop straight away, so there is nothing to confirm.
func newThemeModeChoice() fyne.CanvasObject {
	var tiles []*themeModeTile
	choose := func(mode string) {
		for _, t := range tiles {
			t.setSelected(t.mode == mode)
		}
		applyThemeMode(mode)
	}
	tiles = []*themeModeTile{
		newThemeModeTile(themeModeLight, "Light", theme.VariantLight, choose),
		newThemeModeTile(themeModeDark, "Dark", theme.VariantDark, choose),
	}

	current := currentThemeMode()
	objects := make([]fyne.CanvasObject, len(tiles))
	for i, t := range tiles {
		t.selected = t.mode == current
		objects[i] = t
	}
	return container.NewGridWithColumns(len(objects), objects...)
}

// variantTheme is the theme the desktop is using, from which each mode tile
// samples its own variant's colours.
func variantTheme() fyne.Theme {
	if th := fyne.CurrentApp().Settings().Theme(); th != nil {
		return th
	}
	return theme.DefaultTheme()
}

// themeModeTile is one of the light/dark options in the appearance settings.
// It paints a miniature desktop in that variant's colours - sampled from the
// user's current theme, so it previews what they will actually get.
type themeModeTile struct {
	widget.BaseWidget

	mode    string
	title   string
	variant fyne.ThemeVariant
	onTap   func(string)

	selected bool
	hovered  bool
	ring     *canvas.Rectangle
}

func newThemeModeTile(mode, title string, variant fyne.ThemeVariant, onTap func(string)) *themeModeTile {
	t := &themeModeTile{mode: mode, title: title, variant: variant, onTap: onTap}
	t.ExtendBaseWidget(t)
	return t
}

func (t *themeModeTile) CreateRenderer() fyne.WidgetRenderer {
	th := variantTheme()

	t.ring = canvas.NewRectangle(th.Color(theme.ColorNameBackground, t.variant))
	t.ring.CornerRadius = themeTileRadius

	// A mock window on the sample desktop: an accent title bar over two lines of
	// content, enough to read as "this is what your screen will look like".
	win := canvas.NewRectangle(th.Color(theme.ColorNameInputBackground, t.variant))
	win.CornerRadius = theme.Size(theme.SizeNameInputRadius)
	win.SetMinSize(fyne.NewSize(96, 48))
	bar := canvas.NewRectangle(th.Color(theme.ColorNamePrimary, t.variant))
	bar.CornerRadius = theme.Size(theme.SizeNameSelectionRadius)
	bar.SetMinSize(fyne.NewSize(0, 8))
	mock := container.NewStack(win, container.NewPadded(container.NewVBox(
		bar, t.mockLine(th, 60), t.mockLine(th, 38),
	)))

	label := canvas.NewText(t.title, th.Color(theme.ColorNameForeground, t.variant))
	label.TextStyle = fyne.TextStyle{Bold: true}
	label.Alignment = fyne.TextAlignCenter

	content := container.NewVBox(container.NewPadded(mock), label)
	t.applyState()
	return widget.NewSimpleRenderer(container.NewStack(t.ring, container.NewPadded(content)))
}

// mockLine draws one line of pretend window content, width wide.
func (t *themeModeTile) mockLine(th fyne.Theme, width float32) fyne.CanvasObject {
	c := color.NRGBAModel.Convert(th.Color(theme.ColorNameForeground, t.variant)).(color.NRGBA)
	c.A = 0x55
	line := canvas.NewRectangle(c)
	line.CornerRadius = 2
	line.SetMinSize(fyne.NewSize(width, 5))
	return container.NewHBox(line, layout.NewSpacer())
}

// setSelected marks this tile as the chosen mode, or not.
func (t *themeModeTile) setSelected(selected bool) {
	t.selected = selected
	t.applyState()
}

// applyState paints the ring and tick for the current selection and hover.
func (t *themeModeTile) applyState() {
	if t.ring == nil {
		return
	}

	switch {
	case t.selected:
		t.ring.StrokeColor = theme.Color(theme.ColorNamePrimary)
		t.ring.StrokeWidth = 3
	case t.hovered:
		t.ring.StrokeColor = variantTheme().Color(theme.ColorNameForeground, t.variant)
		t.ring.StrokeWidth = 1
	default:
		t.ring.StrokeColor = variantTheme().Color(theme.ColorNameInputBorder, t.variant)
		t.ring.StrokeWidth = 1
	}
	t.ring.Refresh()
}

func (t *themeModeTile) Tapped(*fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap(t.mode)
	}
}

func (t *themeModeTile) MouseIn(*deskDriver.MouseEvent) {
	t.hovered = true
	t.applyState()
}

func (t *themeModeTile) MouseMoved(*deskDriver.MouseEvent) {}

func (t *themeModeTile) MouseOut() {
	t.hovered = false
	t.applyState()
}
