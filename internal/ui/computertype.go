package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"fyshos.com/tyde"
	wmtheme "fyshos.com/tyde/theme"
)

// The hardware Tyde can be set up for, in the order they are offered. Each is a
// setting value paired with the icon that hints at the machine it describes.
var computerTypes = []struct {
	kind string
	icon fyne.Resource
}{
	{tyde.ComputerDesktop, wmtheme.DisplayIcon},
	{tyde.ComputerLaptop, wmtheme.LaptopIcon},
	{tyde.ComputerTablet, wmtheme.TabletIcon},
}

// newComputerTypeChoice builds the hardware type picker.
func newComputerTypeChoice(current string, choose func(string)) fyne.CanvasObject {
	buttons := make([]fyne.CanvasObject, len(computerTypes))

	showChoice := func(kind string) {
		for i, computer := range computerTypes {
			button := buttons[i].(*widget.Button)
			if computer.kind == kind {
				button.Importance = widget.HighImportance
			} else {
				button.Importance = widget.LowImportance
			}
			button.Refresh()
		}
	}

	for i, computer := range computerTypes {
		kind := computer.kind
		buttons[i] = widget.NewButtonWithIcon(kind, computer.icon, func() {
			showChoice(kind)
			choose(kind)
		})
	}
	showChoice(current)

	return container.NewGridWithColumns(len(buttons), buttons...)
}
