package systray

import (
	"strconv"
	"strings"

	"github.com/godbus/dbus/v5"

	"fyne.io/fyne/v2"

	"fyshos.com/tyde"
)

// dbusKeyNames maps the X11 style key names that appear in a dbusmenu
// "shortcut" property to the matching Fyne key.
var dbusKeyNames = map[string]fyne.KeyName{
	"escape":     fyne.KeyEscape,
	"esc":        fyne.KeyEscape,
	"return":     fyne.KeyReturn,
	"enter":      fyne.KeyReturn,
	"kp_enter":   fyne.KeyEnter,
	"tab":        fyne.KeyTab,
	"backspace":  fyne.KeyBackspace,
	"insert":     fyne.KeyInsert,
	"ins":        fyne.KeyInsert,
	"delete":     fyne.KeyDelete,
	"del":        fyne.KeyDelete,
	"space":      fyne.KeySpace,
	"home":       fyne.KeyHome,
	"end":        fyne.KeyEnd,
	"prior":      fyne.KeyPageUp,
	"page_up":    fyne.KeyPageUp,
	"pageup":     fyne.KeyPageUp,
	"next":       fyne.KeyPageDown,
	"page_down":  fyne.KeyPageDown,
	"pagedown":   fyne.KeyPageDown,
	"up":         fyne.KeyUp,
	"down":       fyne.KeyDown,
	"left":       fyne.KeyLeft,
	"right":      fyne.KeyRight,
	"plus":       fyne.KeyPlus,
	"minus":      fyne.KeyMinus,
	"equal":      fyne.KeyEqual,
	"asterisk":   fyne.KeyAsterisk,
	"period":     fyne.KeyPeriod,
	"comma":      fyne.KeyComma,
	"slash":      fyne.KeySlash,
	"backslash":  fyne.KeyBackslash,
	"semicolon":  fyne.KeySemicolon,
	"apostrophe": fyne.KeyApostrophe,
	"grave":      fyne.KeyBackTick,
}

// parseShortcut reads the "shortcut" property of a dbusmenu item.
func parseShortcut(name string, in dbus.Variant) *tyde.Shortcut {
	presses := shortcutPresses(in.Value())
	if len(presses) == 0 {
		return nil
	}

	var mod fyne.KeyModifier
	key := fyne.KeyUnknown
	for _, press := range presses[0] {
		switch strings.ToLower(press) {
		case "control", "ctrl":
			mod |= fyne.KeyModifierControl
		case "alt":
			mod |= fyne.KeyModifierAlt
		case "shift":
			mod |= fyne.KeyModifierShift
		case "super", "meta":
			mod |= fyne.KeyModifierSuper
		default:
			key = keyForName(press)
		}
	}

	if key == fyne.KeyUnknown {
		return nil
	}
	return tyde.NewShortcut(name, key, mod)
}

// shortcutPresses normalises the value of a "shortcut" property to a list of
// key presses. We have a loose matching to be more supportive of non-compliant desktops.
func shortcutPresses(in any) [][]string {
	switch v := in.(type) {
	case [][]string:
		return v
	case []any:
		var presses [][]string
		for _, item := range v {
			switch press := item.(type) {
			case []string:
				presses = append(presses, press)
			case []any:
				var keys []string
				for _, k := range press {
					if s, ok := k.(string); ok {
						keys = append(keys, s)
					}
				}
				presses = append(presses, keys)
			}
		}
		return presses
	}
	return nil
}

// keyForName matches a key name from a dbusmenu shortcut to a Fyne key name.
func keyForName(in string) fyne.KeyName {
	lower := strings.ToLower(in)
	if key, ok := dbusKeyNames[lower]; ok {
		return key
	}

	if len(in) == 1 { // letters and symbols are named by the character itself
		return fyne.KeyName(strings.ToUpper(in))
	}
	if lower[0] == 'f' { // function keys, F1 through F12
		if _, err := strconv.Atoi(lower[1:]); err == nil {
			return fyne.KeyName(strings.ToUpper(in))
		}
	}

	return fyne.KeyName(in)
}
