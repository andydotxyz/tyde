// Package sloth adds a sleepy sloth to Tyde. It drapes itself over the top edge
// of the window you are working in - body resting on the frame, arms dangling
// over the title bar - and then does what sloths do: nothing. It hangs on as the
// window is dragged or resized, and moves only when you work in another window.
// Maximized and fullscreen windows have no frame edge to hang from, so there the
// sloth simply is not there.
package sloth

import "fyshos.com/tyde"

var slothMeta = tyde.ModuleMetadata{
	Name:        "Sloth",
	NewInstance: newSloth,
}

// sloth is the module instance. It owns a perch and exposes the animal to the
// compositor as a window accessory, so it stacks at the z-level of the window it
// hangs from - windows raised above that one cover it, and it travels with the
// window as that is moved.
type sloth struct {
	perch *perch
}

func newSloth() tyde.Module {
	s := &sloth{perch: newPerch()}
	s.perch.start()
	return s
}

func (s *sloth) Metadata() tyde.ModuleMetadata { return slothMeta }

// WindowAccessories returns the sloth anchored to the window it is hanging from
// (see perch.WindowAccessories).
func (s *sloth) WindowAccessories() []tyde.WindowAccessory {
	return s.perch.WindowAccessories()
}

func (s *sloth) Destroy() {
	s.perch.stop()
}
