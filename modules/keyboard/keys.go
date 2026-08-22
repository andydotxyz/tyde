package keyboard

import "github.com/BurntSushi/xgb/xproto"

// Keysyms for the keys that do not simply type the character on their face.
// X11 names these XK_*; the values come from keysymdef.h and never change.
const (
	symSpace     xproto.Keysym = 0x0020
	symBackSpace xproto.Keysym = 0xff08
	symTab       xproto.Keysym = 0xff09
	symReturn    xproto.Keysym = 0xff0d
	symEscape    xproto.Keysym = 0xff1b
	symLeft      xproto.Keysym = 0xff51
	symUp        xproto.Keysym = 0xff52
	symRight     xproto.Keysym = 0xff53
	symDown      xproto.Keysym = 0xff54

	// The modifier keys we hold down around another keystroke. The left-hand
	// one of each pair is used; every layout has it.
	symShiftL   xproto.Keysym = 0xffe1
	symControlL xproto.Keysym = 0xffe3
	symAltL     xproto.Keysym = 0xffe9
	symSuperL   xproto.Keysym = 0xffeb
)

// unicodeKeysymBase is the X11 convention for keysyms carrying a Unicode code
// point directly: 0x01000000 plus the code point. Latin-1 code points are their
// own keysym and must not be offset.
const unicodeKeysymBase = 0x01000000

// keysymFor maps a rune to its X11 keysym.
func keysymFor(r rune) xproto.Keysym {
	if r < 0x100 {
		return xproto.Keysym(r)
	}
	return xproto.Keysym(unicodeKeysymBase + uint32(r))
}

// modifier is a modifier held down around the next keystroke. Tapping a
// modifier key latches it for one keystroke - the on-screen equivalent of
// holding it - because a touchscreen cannot press two keys at once.
type modifier uint8

const (
	modShift modifier = 1 << iota
	modControl
	modAlt
	modSuper
)

// modifierSyms lists the keysym standing for each modifier, in the order they
// are pressed. Shift goes last so it is innermost, matching the way a hand
// reaches for Ctrl+Shift+key.
var modifierSyms = []struct {
	mod modifier
	sym xproto.Keysym
}{
	{modControl, symControlL},
	{modAlt, symAltL},
	{modSuper, symSuperL},
	{modShift, symShiftL},
}

// keyKind says what tapping a key does.
type keyKind uint8

const (
	keyChar    keyKind = iota // types the character on its face
	keySpecial                // types a named key such as Backspace or Return
	keyMod                    // latches a modifier for the next keystroke
	keyLock                   // latches Shift until tapped again (Caps Lock)
)

// key describes one button on the keyboard.
type key struct {
	kind  keyKind
	lower string        // face, and character typed, when unshifted
	upper string        // face, and character typed, when shifted; "" means as lower
	sym   xproto.Keysym // keySpecial: the keysym to send
	mod   modifier      // keyMod: the modifier to latch
	width float32       // width in key units, a letter key being 1
}

// symbolFor is the keysym a key sends, which for a character key depends on
// whether Shift is on.
func symbolFor(k key, shifted bool) xproto.Keysym {
	if k.kind == keySpecial {
		return k.sym
	}

	face := k.lower
	if shifted && k.upper != "" {
		face = k.upper
	}
	for _, r := range face { // the first rune of the face is the character
		return keysymFor(r)
	}
	return 0
}

// face is the label a key shows, which for a character key follows Shift so the
// keyboard reads as what it will actually type.
func face(k key, shifted bool) string {
	if shifted && k.upper != "" {
		return k.upper
	}
	return k.lower
}

// rowUnits is the width of every row in key units. The rows below each add up
// to it, so the keyboard has straight edges whatever size it is drawn at.
const rowUnits = 15

func char(lower, upper string) key {
	return key{kind: keyChar, lower: lower, upper: upper, width: 1}
}

func special(label string, sym xproto.Keysym, width float32) key {
	return key{kind: keySpecial, lower: label, sym: sym, width: width}
}

func modKey(label string, mod modifier, width float32) key {
	return key{kind: keyMod, lower: label, mod: mod, width: width}
}

// wider returns a copy of a key at a different width, for the character keys
// that are not one unit wide.
func wider(k key, width float32) key {
	k.width = width
	return k
}

// rows is the key layout: a compact US QWERTY, with the cursor keys and Escape
// sharing the bottom row rather than earning blocks of their own. There is no
// number pad and no function row - this is a keyboard to fill in a text field
// with, not to replace the one on the desk.
var rows = [][]key{{
	char("`", "~"), char("1", "!"), char("2", "@"), char("3", "#"), char("4", "$"),
	char("5", "%"), char("6", "^"), char("7", "&"), char("8", "*"), char("9", "("),
	char("0", ")"), char("-", "_"), char("=", "+"),
	special("Bksp", symBackSpace, 2),
}, {
	special("Tab", symTab, 1.5),
	char("q", "Q"), char("w", "W"), char("e", "E"), char("r", "R"), char("t", "T"),
	char("y", "Y"), char("u", "U"), char("i", "I"), char("o", "O"), char("p", "P"),
	char("[", "{"), char("]", "}"),
	wider(char("\\", "|"), 1.5),
}, {
	key{kind: keyLock, lower: "Caps", width: 1.75},
	char("a", "A"), char("s", "S"), char("d", "D"), char("f", "F"), char("g", "G"),
	char("h", "H"), char("j", "J"), char("k", "K"), char("l", "L"),
	char(";", ":"), char("'", "\""),
	special("Enter", symReturn, 2.25),
}, {
	modKey("Shift", modShift, 2.25),
	char("z", "Z"), char("x", "X"), char("c", "C"), char("v", "V"), char("b", "B"),
	char("n", "N"), char("m", "M"), char(",", "<"), char(".", ">"), char("/", "?"),
	modKey("Shift", modShift, 2.75),
}, {
	modKey("Ctrl", modControl, 1.5),
	modKey("Super", modSuper, 1.5),
	modKey("Alt", modAlt, 1.5),
	special("", symSpace, 5.5), // the space bar reads as one by being blank and long
	special("←", symLeft, 1),
	special("↓", symDown, 1),
	special("↑", symUp, 1),
	special("→", symRight, 1),
	special("Esc", symEscape, 1),
}}
