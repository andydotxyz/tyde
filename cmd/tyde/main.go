package main

import (
	_ "fyshos.com/tyde/modules/desktops"
	_ "fyshos.com/tyde/modules/fyles"
	_ "fyshos.com/tyde/modules/launcher"
	_ "fyshos.com/tyde/modules/quaketerm"
	_ "fyshos.com/tyde/modules/rpc"
	_ "fyshos.com/tyde/modules/status"
	_ "fyshos.com/tyde/modules/systray"
	wmtheme "fyshos.com/tyde/theme"

	"fyne.io/fyne/v2/app"
)

func main() {
	a := app.NewWithID("com.fyshos.fynedesk")
	a.SetIcon(wmtheme.AppIcon)
	desk := setupDesktop(a)

	desk.Run()
}
