// Package esheep adds a herd of "Stray Sheep" desktop pets to Tyde. The little
// sheep fall in from above, land on the top borders of windows (or the screen
// floor), walk along them, occasionally sit to eat a daisy and hop over one
// another - a tribute to the 1990s eSheep desktop mate.
package esheep

import (
	"fyne.io/fyne/v2"

	"fyshos.com/tyde"
)

var esheepMeta = tyde.ModuleMetadata{
	Name:        "eSheep",
	NewInstance: newESheep,
}

// esheep is the module instance. It owns a herd that is started lazily when the
// overlay widget is first requested by the desktop.
type esheep struct {
	herd *herd
}

func newESheep() tyde.Module {
	return &esheep{herd: newHerd()}
}

func (e *esheep) Metadata() tyde.ModuleMetadata { return esheepMeta }

// OverlayAreaWidget returns the full-screen, visual-only layer the sheep are
// drawn into and starts the simulation.
func (e *esheep) OverlayAreaWidget() fyne.CanvasObject {
	e.herd.start()
	return e.herd.container
}

func (e *esheep) Destroy() {
	e.herd.stop()
}
