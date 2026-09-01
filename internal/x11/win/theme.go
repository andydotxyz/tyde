//go:build linux || openbsd || freebsd || netbsd

package win

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"

	wmTheme "fyshos.com/tyde/theme"
)

type transparentTheme struct {
	fyne.Theme

	frame *frame
}

func (t *transparentTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameShadow:
		return color.Transparent
	case theme.ColorNameBackground, theme.ColorNameOverlayBackground:
		if t.frame.client.Focused() {
			n = theme.ColorNameOverlayBackground
		} else {
			n = theme.ColorNameDisabledButton
		}
	}

	return t.Theme.Color(n, v)
}

// Size reports our own frame metrics, used to customse the window borders.
func (t *transparentTheme) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case theme.SizeNameWindowTitleBarHeight:
		return wmTheme.TitleHeight
	case theme.SizeNameWindowButtonHeight:
		return wmTheme.TitleButtonHeight
	case theme.SizeNameWindowButtonIcon:
		return wmTheme.TitleButtonIconSize
	}

	return t.Theme.Size(n)
}
