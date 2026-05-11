package tyde

import "fyne.io/fyne/v2"

// DeskSettings describes the configuration options available for Fyne desktop
type DeskSettings interface {
	Background() string
	IconTheme() string
	BorderButtonPosition() string
	ClockFormatting() string
	NarrowWidgetPanel() bool

	LauncherIcons() []string
	LauncherDisableTaskbar() bool

	KeyboardModifier() fyne.KeyModifier
	ModuleNames() []string
	ScreenSaverType() string
	ScreenSaverClock() bool
	ScreenSaverLabel() string

	AddChangeListener(listener func(DeskSettings))
}
