package sloth

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/FyshOS/saver"

	"fyshos.com/tyde"
	wmtest "fyshos.com/tyde/test"
)

// stackWM is the smallest window manager the perch needs: an ordered window
// list, topmost first (matching the real stack), and listeners to tell about it.
type stackWM struct {
	windows   []tyde.Window
	listeners []tyde.StackListener
}

func (s *stackWM) AddWindow(win tyde.Window) { s.windows = append([]tyde.Window{win}, s.windows...) }
func (s *stackWM) RaiseToTop(tyde.Window)    {}
func (s *stackWM) RemoveWindow(tyde.Window)  {}
func (s *stackWM) TopWindow() tyde.Window {
	if len(s.windows) == 0 {
		return nil
	}
	return s.windows[0]
}
func (s *stackWM) Windows() []tyde.Window { return s.windows }
func (s *stackWM) AddStackListener(l tyde.StackListener) {
	s.listeners = append(s.listeners, l)
}

func (s *stackWM) RemoveStackListener(l tyde.StackListener) {
	for i, cur := range s.listeners {
		if cur == l {
			s.listeners = append(s.listeners[:i], s.listeners[i+1:]...)
			return
		}
	}
}
func (s *stackWM) Blank()                                               {}
func (s *stackWM) Close()                                               {}
func (s *stackWM) Run()                                                 {}
func (s *stackWM) ShowOverlay(fyne.Window, fyne.Size, fyne.Position)    {}
func (s *stackWM) ShowMenuOverlay(*fyne.Menu, fyne.Size, fyne.Position) {}
func (s *stackWM) ShowScreensaver(*saver.ScreenSaver)                   {}

// orderChanged tells the listeners the stack was restacked, as the real window
// manager does when a window is raised or focused.
func (s *stackWM) orderChanged() {
	for _, l := range s.listeners {
		l.WindowOrderChanged()
	}
}

// stateChanged tells the listeners a window was maximized, fullscreened or
// iconified, as the real window manager does.
func (s *stackWM) stateChanged(win tyde.Window) {
	for _, l := range s.listeners {
		l.WindowStateChanged(win)
	}
}

// newTestPerch starts a perch on a desktop with the given windows, stacked in
// the order they are passed - topmost first.
func newTestPerch(t *testing.T, windows ...tyde.Window) (*perch, *stackWM) {
	t.Helper()
	test.NewApp()

	wm := &stackWM{}
	for i := len(windows) - 1; i >= 0; i-- {
		wm.AddWindow(windows[i])
	}
	wmtest.NewDesktopWithWM(wm)

	if slothSprite() == nil {
		t.Skip("sloth artwork unavailable")
	}
	p := newPerch()
	p.start()
	p.xFrac = 0.5 // pin the random spot along the edge so positions are predictable
	p.settle()
	return p, wm
}

// testWindow builds a visible, focusable window big enough to nap on.
func testWindow(title string, x, y int) *wmtest.Window {
	win := wmtest.NewWindow(title)
	win.SetGeometry(x, y, 600, 400)
	return win
}

func TestSpriteIsTrimmedAndTransparent(t *testing.T) {
	img := slothSprite()
	if img == nil {
		t.Fatal("sloth artwork failed to load")
	}
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		t.Fatalf("empty sprite %v", b)
	}
	if b.Dx() <= b.Dy() {
		t.Fatalf("expected a sloth wider than it is tall, got %v", b)
	}
	// The white paper is keyed out, so the corners are see-through...
	if _, _, _, a := img.At(b.Min.X, b.Min.Y).RGBA(); a != 0 {
		t.Fatalf("top left corner is not transparent (alpha %d)", a)
	}
	// ...but the animal in the middle is not.
	if _, _, _, a := img.At(b.Dx()/2, b.Dy()/2).RGBA(); a == 0 {
		t.Fatal("the middle of the sprite is transparent")
	}
}

func TestPerchDrapesOverFocusedWindow(t *testing.T) {
	back := testWindow("back", 40, 300)
	front := testWindow("front", 100, 200)
	front.Focus()
	p, _ := newTestPerch(t, front, back)

	if p.win != front {
		t.Fatalf("expected the sloth on the focused window, got %v", p.win)
	}
	acc := p.WindowAccessories()
	if len(acc) != 1 || acc[0].Window != front {
		t.Fatalf("expected one accessory anchored to the focused window, got %#v", acc)
	}

	// The accessory is placed relative to its window: the body rests on the top
	// edge (y = 0) with the arms hanging over it, and the sloth lies within the
	// window's width.
	pos := acc[0].Object.Position()
	if wantY := -p.size.Height * drapeFrac; pos.Y != wantY {
		t.Fatalf("sloth at y=%v, expected to hang from the frame at %v", pos.Y, wantY)
	}
	if pos.X < 0 || pos.X+p.size.Width > front.Size().Width {
		t.Fatalf("sloth at x=%v hangs off the %v wide window", pos.X, front.Size().Width)
	}
}

// TestSlothIgnoresItsWindowMoving is the point of a window accessory: the
// compositor keeps it with its window, so the module has nothing to do when that
// window is dragged around.
func TestSlothIgnoresItsWindowMoving(t *testing.T) {
	win := testWindow("win", 100, 200)
	win.Focus()
	p, _ := newTestPerch(t, win)
	settled := p.img.Position()

	win.SetGeometry(460, 380, 600, 400) // the user drags the window across the screen
	p.WindowMoved(win)                  // ...which fires for every step of the drag
	if p.img.Position() != settled {
		t.Fatalf("expected the sloth to stay at %v, found it at %v", settled, p.img.Position())
	}
	// Even asked to, there is nothing for it to re-place: the position is
	// relative to the window, which is what the compositor moved.
	if p.settle() {
		t.Fatalf("the sloth was re-placed for a move it should not care about: %v then %v",
			settled, p.img.Position())
	}

	// A resize does move it, since it holds its place along the top edge.
	win.SetGeometry(460, 380, 900, 400)
	if !p.settle() {
		t.Fatal("expected the sloth to be re-placed when its window was widened")
	}
	if x := p.img.Position().X; x <= settled.X {
		t.Fatalf("sloth at x=%v did not keep its place along the wider top edge", x)
	}
}

func TestPerchFollowsFocus(t *testing.T) {
	first := testWindow("first", 100, 200)
	second := testWindow("second", 700, 250)
	first.Focus()
	p, wm := newTestPerch(t, first, second)

	if p.win != first {
		t.Fatalf("expected the sloth on the first window, got %v", p.win)
	}

	first.Unfocus()
	second.Focus()
	wm.orderChanged() // the window manager restacks on focus
	if p.win != second {
		t.Fatalf("expected the sloth to follow focus, got %v", p.win)
	}
	if acc := p.WindowAccessories(); len(acc) != 1 || acc[0].Window != second {
		t.Fatalf("expected the accessory to be anchored to the new window, got %#v", acc)
	}

	// Clicking the desktop takes focus off every window; the sloth stays on the
	// topmost one rather than blinking out.
	second.Unfocus()
	wm.orderChanged()
	if p.win != wm.TopWindow() {
		t.Fatalf("expected the sloth on the topmost window when nothing has focus, got %v", p.win)
	}
}

// TestMaximizedAndFullscreenHideTheSloth verifies that a window with no frame
// edge to hang from hides the sloth outright: it does not go and find some other
// window to sit on. The state change alone has to be enough to trigger this -
// the window manager reports one for maximize and fullscreen (see
// x11WM.handleStateActionRequest), and moves are not reacted to at all.
func TestMaximizedAndFullscreenHideTheSloth(t *testing.T) {
	busy := testWindow("busy", 100, 200)
	busy.Focus()
	spare := testWindow("spare", 300, 300)
	p, wm := newTestPerch(t, busy, spare)

	if p.win != busy {
		t.Fatalf("expected the sloth on the window in use, got %v", p.win)
	}

	busy.Maximize()
	wm.stateChanged(busy)
	if p.win != nil {
		t.Fatalf("expected the sloth to be gone, found it on %v", p.win)
	}
	if acc := p.WindowAccessories(); acc != nil {
		t.Fatalf("expected the sloth to be dropped, got %#v", acc)
	}

	busy.Unmaximize()
	busy.Fullscreen()
	wm.stateChanged(busy)
	if p.win != nil {
		t.Fatalf("expected the sloth to be gone from a fullscreen window, got %v", p.win)
	}

	// Back to a normal window and it returns - to that window, not the spare.
	busy.Unfullscreen()
	wm.stateChanged(busy)
	if p.win != busy {
		t.Fatalf("expected the sloth back on the window in use, got %v", p.win)
	}
}

// TestTightWindowsStillTakeTheSloth covers the windows we used to give up on: it
// hangs off the top of the screen or over the sides rather than disappearing.
func TestTightWindowsStillTakeTheSloth(t *testing.T) {
	high := testWindow("high", 100, 4) // barely any room above the top edge
	high.Focus()
	p, _ := newTestPerch(t, high)

	if p.win != high {
		t.Fatalf("a window near the top of the screen should still take the sloth, got %v", p.win)
	}
	// It hangs above the frame as always; the screen edge crops the rest.
	if y := p.img.Position().Y; y != -p.size.Height*drapeFrac {
		t.Fatalf("sloth at y=%v, expected it to hang from the frame at %v", y, -p.size.Height*drapeFrac)
	}

	narrow := wmtest.NewWindow("narrow")
	narrow.SetGeometry(100, 200, uint(p.size.Width)/2, 400)
	narrow.Focus()
	high.Unfocus()
	if !p.suitable(narrow) {
		t.Fatal("a window narrower than the sloth should still take it")
	}
	p.settle()
	// Too narrow to lie across, so it overhangs evenly on both sides.
	if x := p.place(narrow).X; x >= 0 || x != (narrow.Size().Width-p.size.Width-edgeInset*2)/2 {
		t.Fatalf("expected the sloth to overhang a narrow window evenly, got x=%v", x)
	}
}

func TestOffDesktopWindowsAreSkipped(t *testing.T) {
	p, _ := newTestPerch(t)

	other := testWindow("other-desk", 100, 200)
	other.SetDesktop(3)
	if p.suitable(other) {
		t.Error("a window on another desktop should be skipped")
	}
	other.Pin()
	if !p.suitable(other) {
		t.Error("a pinned window is on every desktop")
	}

	gone := testWindow("iconified", 100, 200)
	gone.Iconify()
	if p.suitable(gone) {
		t.Error("an iconified window is not on screen to hang from")
	}
}

func TestStoppedPerchLetsGo(t *testing.T) {
	win := testWindow("win", 100, 200)
	win.Focus()
	p, wm := newTestPerch(t, win)

	p.stop()
	if len(wm.listeners) != 0 {
		t.Fatalf("expected the stack listener to be removed, %d left", len(wm.listeners))
	}
	if acc := p.WindowAccessories(); acc != nil {
		t.Fatalf("a stopped sloth should draw nothing, got %#v", acc)
	}
}
