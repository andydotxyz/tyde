package systray

import (
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/assert"

	"fyne.io/fyne/v2"
)

func TestParseShortcut(t *testing.T) {
	for name, tt := range map[string]struct {
		press []string
		key   fyne.KeyName
		mod   fyne.KeyModifier
	}{
		"simple":       {[]string{"Control", "Q"}, fyne.KeyQ, fyne.KeyModifierControl},
		"lower case":   {[]string{"control", "q"}, fyne.KeyQ, fyne.KeyModifierControl},
		"multiple mod": {[]string{"Control", "Shift", "N"}, fyne.KeyN, fyne.KeyModifierControl | fyne.KeyModifierShift},
		"super":        {[]string{"Super", "Space"}, fyne.KeySpace, fyne.KeyModifierSuper},
		"no modifier":  {[]string{"F5"}, fyne.KeyF5, 0},
		"function key": {[]string{"Alt", "f12"}, fyne.KeyF12, fyne.KeyModifierAlt},
		"named key":    {[]string{"Control", "page_down"}, fyne.KeyPageDown, fyne.KeyModifierControl},
		"symbol key":   {[]string{"Control", "plus"}, fyne.KeyPlus, fyne.KeyModifierControl},
	} {
		t.Run(name, func(t *testing.T) {
			s := parseShortcut("Item", dbus.MakeVariant([][]string{tt.press}))
			if !assert.NotNil(t, s) {
				return
			}

			assert.Equal(t, "Item", s.ShortcutName())
			assert.Equal(t, tt.key, s.Key())
			assert.Equal(t, tt.mod, s.Mod())
		})
	}
}

func TestParseShortcut_Invalid(t *testing.T) {
	assert.Nil(t, parseShortcut("Item", dbus.MakeVariant([][]string{})))
	assert.Nil(t, parseShortcut("Item", dbus.MakeVariant("Control+Q")))
	assert.Nil(t, parseShortcut("Item", dbus.MakeVariant([][]string{{"Control"}})))
}

// some apps send the shortcut without the signature the specification asks for
func TestParseShortcut_LooselyTyped(t *testing.T) {
	s := parseShortcut("Item", dbus.MakeVariant([]any{[]any{"Alt", "F4"}}))
	if !assert.NotNil(t, s) {
		return
	}

	assert.Equal(t, fyne.KeyF4, s.Key())
	assert.Equal(t, fyne.KeyModifierAlt, s.Mod())
}
