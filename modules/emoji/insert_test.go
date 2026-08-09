package emoji

import (
	"testing"

	"github.com/BurntSushi/xgb/xproto"

	"github.com/stretchr/testify/assert"
)

func TestKeysymFor(t *testing.T) {
	// Latin-1 runes are their own keysym.
	assert.Equal(t, xproto.Keysym('a'), keysymFor('a'))
	assert.Equal(t, xproto.Keysym('#'), keysymFor('#'))

	// Everything above Latin-1 uses the Unicode keysym range.
	assert.Equal(t, xproto.Keysym(0x0101F600), keysymFor('😀'))
	assert.Equal(t, xproto.Keysym(0x0100200D), keysymFor('‍')) // zero width joiner
	assert.Equal(t, xproto.Keysym(0x0100FE0F), keysymFor('️')) // variation selector
}

func TestInsert_NoTarget(t *testing.T) {
	i := &x11Inserter{}
	assert.Error(t, i.Insert("😀", 0))
	assert.Error(t, i.Insert("😀", xproto.WindowNone))
}
