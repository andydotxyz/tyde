package main

import (
	wmtheme "fyshos.com/fynedesk/theme"

	"fyne.io/fyne/v2/app"
)

func main() {
	a := app.NewWithID("com.fyshos.fynedesk")
	a.SetIcon(wmtheme.AppIcon)
	desk := setupDesktop(a)

	desk.Run()
}
