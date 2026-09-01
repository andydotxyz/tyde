package keyboard

import (
	"testing"

	"github.com/BurntSushi/xgb/xproto"
	"github.com/stretchr/testify/assert"
)

// Every row has to add up to the same number of units or the keyboard comes out
// ragged at whatever width it is drawn.
func TestRows_AreTheSameWidth(t *testing.T) {
	for i, row := range rows {
		var units float32
		for _, k := range row {
			assert.Greater(t, k.width, float32(0), "row %d has a key with no width", i)
			units += k.width
		}
		assert.Equal(t, float32(rowUnits), units, "row %d", i)
	}
}

// A character key that has no shifted face would type nothing when Shift is on.
func TestRows_CharacterKeysHaveBothFaces(t *testing.T) {
	for _, row := range rows {
		for _, k := range row {
			if k.kind != keyChar {
				continue
			}
			assert.NotEmpty(t, k.lower)
			assert.NotEmpty(t, k.upper, "%q has no shifted character", k.lower)
		}
	}
}

func TestRows_HaveTheKeysThatCannotBeTypedAnotherWay(t *testing.T) {
	found := map[xproto.Keysym]bool{}
	for _, row := range rows {
		for _, k := range row {
			if k.kind == keySpecial {
				found[k.sym] = true
			}
		}
	}

	for _, sym := range []xproto.Keysym{
		symSpace, symBackSpace, symTab, symReturn,
		symEscape, symLeft, symRight, symUp, symDown,
	} {
		assert.True(t, found[sym], "keysym %#x is not on the keyboard", sym)
	}
}

func TestKeysymFor(t *testing.T) {
	// Latin-1 runes are their own keysym.
	assert.Equal(t, xproto.Keysym('a'), keysymFor('a'))
	assert.Equal(t, xproto.Keysym('#'), keysymFor('#'))

	// Everything above Latin-1 uses the Unicode keysym range.
	assert.Equal(t, xproto.Keysym(0x01002018), keysymFor('‘'))
}

func TestSymbolFor(t *testing.T) {
	a := char("a", "A")
	assert.Equal(t, xproto.Keysym('a'), symbolFor(a, false))
	assert.Equal(t, xproto.Keysym('A'), symbolFor(a, true))

	one := char("1", "!")
	assert.Equal(t, xproto.Keysym('1'), symbolFor(one, false))
	assert.Equal(t, xproto.Keysym('!'), symbolFor(one, true))

	// A named key sends the same keysym either way - Shift is passed as a
	// modifier instead, so Shift+Tab still reaches the application.
	tab := special("Tab", symTab, 1.5)
	assert.Equal(t, symTab, symbolFor(tab, false))
	assert.Equal(t, symTab, symbolFor(tab, true))
}

func TestFace(t *testing.T) {
	a := char("a", "A")
	assert.Equal(t, "a", face(a, false))
	assert.Equal(t, "A", face(a, true))

	// Keys with only one face keep it whatever Shift is doing.
	shift := modKey("Shift", modShift, 2.25)
	assert.Equal(t, "Shift", face(shift, true))
}
