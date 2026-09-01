package keyboard

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/assert"
)

func TestRowLayout_SharesWidthByUnits(t *testing.T) {
	row := &rowLayout{widths: []float32{1, 1, 2}}
	keys := []fyne.CanvasObject{
		newKeyButton(char("a", "A"), nil),
		newKeyButton(char("b", "B"), nil),
		newKeyButton(special("Bksp", symBackSpace, 2), nil),
	}

	pad := theme.Padding()
	width := 400 + pad*2 // room for 4 units and the two gaps between three keys
	row.Layout(keys, fyne.NewSize(width, keyHeight))

	assert.Equal(t, float32(100), keys[0].Size().Width)
	assert.Equal(t, float32(100), keys[1].Size().Width)
	assert.Equal(t, float32(200), keys[2].Size().Width, "a double-width key gets twice the room")
	assert.Equal(t, keyHeight, keys[0].Size().Height)

	// The keys sit side by side, in order, with one padding between them.
	assert.Equal(t, float32(0), keys[0].Position().X)
	assert.Equal(t, 100+pad, keys[1].Position().X)
	assert.Equal(t, 200+pad*2, keys[2].Position().X)

	// The last key ends where the row does, so the keyboard has a straight edge.
	assert.Equal(t, width, keys[2].Position().X+keys[2].Size().Width)
}

func TestRowLayout_Empty(t *testing.T) {
	row := &rowLayout{}

	assert.NotPanics(t, func() { row.Layout(nil, fyne.NewSize(100, 10)) })
	assert.Equal(t, fyne.Size{}, row.MinSize(nil))
}

func TestKeyButton_SetFace(t *testing.T) {
	b := newKeyButton(char("a", "A"), nil)
	assert.Equal(t, "a", b.Text)

	b.setFace(true, false)
	assert.Equal(t, "A", b.Text)

	b.setFace(false, false)
	assert.Equal(t, "a", b.Text)
}

// A latched modifier is highlighted, so it is clear the keyboard is waiting to
// apply it to the next key.
func TestKeyButton_LatchedModifierStandsOut(t *testing.T) {
	b := newKeyButton(modKey("Ctrl", modControl, 1.5), nil)
	assert.Equal(t, widget.MediumImportance, b.Importance)

	b.setFace(false, true)
	assert.Equal(t, widget.HighImportance, b.Importance)

	b.setFace(false, false)
	assert.Equal(t, widget.MediumImportance, b.Importance)
}
