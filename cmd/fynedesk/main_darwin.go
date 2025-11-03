package main

import (
	"log"
	"runtime"

	"github.com/FyshOS/appie"

	"fyne.io/fyne/v2"

	"fyshos.com/fynedesk"
	"fyshos.com/fynedesk/internal/ui"
	_ "fyshos.com/fynedesk/modules/quaketerm"
)

func setupDesktop(a fyne.App) fynedesk.Desktop {
	log.Println("Full desktop not possible on", runtime.GOOS)
	return ui.NewEmbeddedDesktop(a, appie.NewMacOSProvider())
}
