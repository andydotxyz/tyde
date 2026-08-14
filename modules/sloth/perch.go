package sloth

import (
	"math/rand"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"fyshos.com/tyde"
)

// Perch constants, in canvas units.
const (
	spriteHeight = 62.0 // on-screen height of the sloth; the width follows the artwork

	// drapeFrac is how much of the sprite belongs above the window's top edge.
	drapeFrac = 0.67

	edgeInset = 10.0 // keep the sloth this far from the window's corners
)

// perch owns the sloth's canvas image and the window it is hanging from.
// We only reconsider where the accessory lives when the window stack changes underneath it.
type perch struct {
	img  *canvas.Image
	size fyne.Size // on-screen size of the sprite, derived from the artwork

	win   tyde.Window // the window the sloth is hanging from, nil when it has none
	xFrac float32     // where along that window's top edge it settled, 0-1

	rng *rand.Rand
}

func newPerch() *perch {
	return &perch{rng: rand.New(rand.NewSource(time.Now().UnixNano()))}
}

// start positions the sloth on a window and listens for the stack changes that might call
// for a different one.
func (p *perch) start() {
	img := slothSprite()
	if img == nil {
		return // artwork failed to load; nothing to draw
	}
	b := img.Bounds()
	if b.Dy() <= 0 {
		return
	}
	p.size = fyne.NewSize(spriteHeight*float32(b.Dx())/float32(b.Dy()), spriteHeight)
	p.xFrac = p.rng.Float32()

	p.img = canvas.NewImageFromImage(img)
	p.img.ScaleMode = canvas.ImageScaleFastest
	p.img.FillMode = canvas.ImageFillContain
	p.img.Resize(p.size)

	p.settle()
	if wm := tyde.Instance().WindowManager(); wm != nil {
		wm.AddStackListener(p)
	}
}

// stop leaves the sloth's window and stops watching the stack.
func (p *perch) stop() {
	if wm := tyde.Instance().WindowManager(); wm != nil {
		wm.RemoveStackListener(p)
	}
	p.win = nil
}

// settle finds the window the sloth should be hanging from and places it there,
// reporting whether that changed anything.
func (p *perch) settle() bool {
	if p.img == nil {
		return false // artwork unavailable
	}

	win := p.target()
	changed := win != p.win
	if changed && win != nil {
		// A fresh window is a fresh spot to flop down on.
		p.xFrac = p.rng.Float32()
	}
	p.win = win
	if win == nil {
		return changed
	}

	if pos := p.place(win); pos != p.img.Position() {
		p.img.Move(pos)
		changed = true
	}
	return changed
}

// place works out where the sloth hangs on its window, relative to the top left
// of that window: along the top edge at xFrac, with the body above the frame and
// the arms dangling over it. A window too narrow to lie across gets an evenly
// overhanging sloth.
func (p *perch) place(win tyde.Window) fyne.Position {
	span := win.Size().Width - p.size.Width - edgeInset*2
	x := edgeInset + span*p.xFrac
	if span < 0 {
		x = span / 2 // narrower than the sloth: hang over both sides equally
	}
	return fyne.NewPos(x, -p.size.Height*drapeFrac)
}

// target returns the window the sloth belongs on: the focused one, or the
// topmost when nothing has focus.
func (p *perch) target() tyde.Window {
	wm := tyde.Instance().WindowManager()
	if wm == nil {
		return nil
	}

	win := wm.TopWindow()
	for _, w := range wm.Windows() {
		if w.Focused() {
			win = w
			break
		}
	}
	if !p.suitable(win) {
		return nil
	}
	return win
}

// suitable reports whether a window can hold the sloth: it has to be on screen,
// on the current desktop, and have a frame edge to hang from.
func (p *perch) suitable(win tyde.Window) bool {
	if win == nil || win.Iconic() || win.Fullscreened() || win.Maximized() {
		return false
	}
	return win.Pinned() || win.Desktop() == tyde.Instance().Desktop()
}

func (p *perch) WindowAccessories() []tyde.WindowAccessory {
	if p.win == nil || p.img == nil {
		return nil
	}
	return []tyde.WindowAccessory{{Object: p.img, Window: p.win}}
}

func (p *perch) WindowAdded(tyde.Window)        { p.reconsider() }
func (p *perch) WindowRemoved(tyde.Window)      { p.reconsider() }
func (p *perch) WindowStateChanged(tyde.Window) { p.reconsider() }
func (p *perch) WindowOrderChanged()            { p.reconsider() }
func (p *perch) WindowMoved(tyde.Window)        {}

// reconsider re-settles the sloth. Stack listeners are not called on the main
// goroutine, so the canvas work goes through fyne.Do.
func (p *perch) reconsider() {
	fyne.Do(func() {
		if !p.settle() {
			return
		}
		tyde.Instance().RefreshWindowAccessories()
	})
}
