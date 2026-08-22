// Package keyboard adds an on-screen keyboard to Tyde, for touchscreen machines
// where there may be no hardware keyboard within reach. It stays out of the way
// until it is asked for: a keyboard icon sits in the status area alongside the
// system tray, and tapping it raises the keyboard over whatever is on screen,
// tapping it again puts it away. Super+K does the same for the times a hardware
// keyboard is to hand after all.
//
// Keys are typed into the focused application as real key events through the
// XTEST extension (see typer.go), so applications cannot tell them from a
// physical keyboard and nothing needs to support the keyboard specially.
package keyboard

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"fyshos.com/tyde"
	wmTheme "fyshos.com/tyde/theme"
)

// ModuleName is the registered name of the on-screen keyboard module.
const ModuleName = "Virtual Keyboard"

var meta = tyde.ModuleMetadata{
	Name:        ModuleName,
	NewInstance: newKeyboard,
}

// module is the module instance. It owns the single keyboard overlay.
type module struct {
	panel *panel
}

func newKeyboard() tyde.Module {
	return &module{panel: &panel{}}
}

func (m *module) Metadata() tyde.ModuleMetadata {
	return meta
}

func (m *module) Destroy() {
	if m.panel != nil {
		m.panel.destroy()
	}
}

// StatusAreaWidget puts the keyboard icon in the status area, next to the system
// tray icons - the one place a touchscreen user can reach without a keyboard.
func (m *module) StatusAreaWidget() fyne.CanvasObject {
	return &widget.Button{Icon: wmTheme.KeyboardIcon, Importance: widget.LowImportance,
		OnTapped: func() { m.panel.toggle() }}
}

// Shortcuts binds the keyboard to Super+K, which is unclaimed elsewhere in Tyde.
func (m *module) Shortcuts() map[*tyde.Shortcut]func() {
	return map[*tyde.Shortcut]func(){
		tyde.NewShortcut("Show Virtual Keyboard", fyne.KeyK, tyde.UserModifier): func() {
			m.panel.toggle()
		},
	}
}

// ShowKeyboard opens the keyboard. Must be called on the UI thread.
func (m *module) ShowKeyboard() {
	m.panel.show()
}
