//go:build (linux || openbsd || freebsd || netbsd) && !no_native_icons

package main

import (
	"log"

	"github.com/FyshOS/appie"

	"fyne.io/fyne/v2"

	"fyshos.com/fynedesk"
	"fyshos.com/fynedesk/internal/ui"
	"fyshos.com/fynedesk/internal/x11/wm"
	_ "fyshos.com/fynedesk/modules/composit"
	_ "fyshos.com/fynedesk/modules/desktops"
	_ "fyshos.com/fynedesk/modules/fyles"
	_ "fyshos.com/fynedesk/modules/launcher"
	_ "fyshos.com/fynedesk/modules/quaketerm"
	_ "fyshos.com/fynedesk/modules/status"
	_ "fyshos.com/fynedesk/modules/systray"
)

func setupDesktop(a fyne.App) fynedesk.Desktop {
	icons := appie.NewFDOProvider()
	mgr, err := wm.NewX11WindowManager(a)
	if err != nil {
		log.Println("Could not create window manager:", err)
		return ui.NewEmbeddedDesktop(a, icons)
	}
	desk := ui.NewDesktop(a, mgr, icons, wm.NewX11ScreensProvider(mgr))
	return desk
}
