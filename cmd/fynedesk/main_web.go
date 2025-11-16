//go:build web || wasm || js || no_native_icons

package main

import (
	"fyne.io/fyne/v2"

	"fyshos.com/fynedesk"
	"fyshos.com/fynedesk/internal/ui"
	_ "fyshos.com/fynedesk/modules/desktops"
	_ "fyshos.com/fynedesk/modules/launcher"
)

func setupDesktop(a fyne.App) fynedesk.Desktop {
	return ui.NewEmbeddedDesktop(a, nil)
}
