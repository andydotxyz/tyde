//go:build web || wasm || js || no_native_icons

package main

import (
	"fyne.io/fyne/v2"

	"fyshos.com/fynedesk"
	"fyshos.com/fynedesk/internal/ui"
)

func setupDesktop(a fyne.App) fynedesk.Desktop {
	return ui.NewEmbeddedDesktop(a, nil)
}
