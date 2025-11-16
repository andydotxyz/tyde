//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !no_native_icons && !web && !wasm && !js

package main

import (
	"log"
	"runtime"

	"fyne.io/fyne/v2"

	"fyshos.com/fynedesk"
	"fyshos.com/fynedesk/internal"
	"fyshos.com/fynedesk/internal/ui"
	_ "fyshos.com/fynedesk/modules/desktops"
	_ "fyshos.com/fynedesk/modules/fyles"
	_ "fyshos.com/fynedesk/modules/launcher"
	_ "fyshos.com/fynedesk/modules/status"
)

func setupDesktop(a fyne.App) fynedesk.Desktop {
	log.Println("Full desktop not possible on", runtime.GOOS)
	return ui.NewEmbeddedDesktop(a, internal.NewFDOIconProvider())
}
