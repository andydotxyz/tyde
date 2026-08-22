package keyboard

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// minKeySize is the floor for a key size.
const minKeySize = float32(24)

// keyButton is one key. It is a button with its own minimum size, so that the
// long labels ("Shift", "Enter") do not force the keyboard wider than the
// screen - a key is as wide as its row gives it, and the label truncates.
type keyButton struct {
	widget.Button

	def key
}

func newKeyButton(def key, tapped func()) *keyButton {
	b := &keyButton{def: def}
	b.Text = def.lower
	b.Importance = widget.MediumImportance
	b.OnTapped = tapped
	b.ExtendBaseWidget(b)
	return b
}

func (b *keyButton) MinSize() fyne.Size {
	return fyne.NewSize(minKeySize, minKeySize)
}

// setFace updates what the key shows and how it looks: character keys follow
// Shift so the keyboard reads as what it will type, and a latched modifier is
// highlighted so it is clear it is waiting for the next keystroke.
func (b *keyButton) setFace(shifted, latched bool) {
	text := face(b.def, shifted)
	importance := widget.MediumImportance
	if latched {
		importance = widget.HighImportance
	}

	if b.Text == text && b.Importance == importance {
		return
	}
	b.Text = text
	b.Importance = importance
	b.Refresh()
}

// rowLayout shares a row's width between its keys in proportion to their widths
// in key units, so every row lines up however wide the keyboard is drawn.
type rowLayout struct {
	widths []float32
}

func (r *rowLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) == 0 {
		return
	}

	pad := theme.Padding()
	gaps := pad * float32(len(objects)-1)
	unit := (size.Width - gaps) / r.units()

	x := float32(0)
	for i, o := range objects {
		width := unit * r.widthOf(i)
		o.Resize(fyne.NewSize(width, size.Height))
		o.Move(fyne.NewPos(x, 0))
		x += width + pad
	}
}

func (r *rowLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) == 0 {
		return fyne.Size{}
	}

	pad := theme.Padding()
	return fyne.NewSize(minKeySize*r.units()+pad*float32(len(objects)-1), minKeySize)
}

// units is the row's total width in key units.
func (r *rowLayout) units() float32 {
	var total float32
	for _, w := range r.widths {
		total += w
	}
	if total == 0 {
		return 1 // nothing to share out, but never divide by zero
	}
	return total
}

func (r *rowLayout) widthOf(i int) float32 {
	if i >= len(r.widths) || r.widths[i] <= 0 {
		return 1
	}
	return r.widths[i]
}
