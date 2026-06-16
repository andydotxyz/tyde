// Package esheep adds a herd of "Stray Sheep" desktop pets to Tyde. The little
// sheep fall in from above, land on the top borders of windows (or the screen
// floor), walk along them, occasionally sit to eat a daisy and hop over one
// another - a tribute to the 1990s eSheep desktop mate.
package esheep

import "fyshos.com/tyde"

var esheepMeta = tyde.ModuleMetadata{
	Name:        "eSheep",
	NewInstance: newESheep,
}

// esheep is the module instance. It owns a herd and exposes the sheep to the
// compositor as window accessories so they stack at the z-level of the window
// each one walks on.
type esheep struct {
	herd *herd
}

func newESheep() tyde.Module {
	e := &esheep{herd: newHerd()}
	e.herd.start()
	return e
}

func (e *esheep) Metadata() tyde.ModuleMetadata { return esheepMeta }

// WindowAccessories returns the live sheep as decorations anchored to the
// windows they stand on (see herd.WindowAccessories).
func (e *esheep) WindowAccessories() []tyde.WindowAccessory {
	return e.herd.WindowAccessories()
}

func (e *esheep) Destroy() {
	e.herd.stop()
}
