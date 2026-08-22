package keyboard

import (
	"errors"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/stretchr/testify/assert"

	"fyshos.com/tyde"
	wmTest "fyshos.com/tyde/test"
	wmTheme "fyshos.com/tyde/theme"
)

func TestMain(m *testing.M) {
	test.NewApp()

	os.Exit(m.Run())
}

// stroke is one keystroke as the panel asked for it.
type stroke struct {
	sym  xproto.Keysym
	mods modifier
}

// recorder stands in for the X server: it remembers what would have been typed.
type recorder struct {
	sent   []stroke
	err    error
	closed bool
}

func (r *recorder) Send(sym xproto.Keysym, mods modifier) error {
	r.sent = append(r.sent, stroke{sym, mods})
	return r.err
}

func (r *recorder) Close() { r.closed = true }

// screenDesk is the test desktop reporting the content area a real one would:
// the screen less the bar on the left and the widget panel on the right. The
// test desktop on its own reports a fixed 320x240 that has no relation to its
// screen, which the keyboard's sizing is measured against.
// deskBase is the test desktop under another name, so that embedding it does not
// collide with the Desktop() method every desktop has.
type deskBase = wmTest.Desktop

type screenDesk struct {
	*deskBase
}

func (d *screenDesk) ContentBoundsPixels(screen *tyde.Screen) (x, y, w, h uint32) {
	scale := screen.CanvasScale()
	bar := uint32(wmTheme.NarrowBarWidth * scale)
	widgets := uint32(wmTheme.WidgetPanelWidth * scale)
	return bar, 0, uint32(screen.Width) - bar - widgets, uint32(screen.Height)
}

// newTestPanel builds a keyboard that types into a recorder rather than the X
// server, on a desktop that records overlays rather than drawing them.
func newTestPanel(t *testing.T) (*panel, *recorder) {
	t.Helper()

	tyde.SetInstance(&screenDesk{wmTest.NewDesktop()})
	rec := &recorder{}
	newSenderFunc = func() (sender, error) { return rec, nil }

	fyne.CurrentApp().Preferences().SetBool(prefAtTop, false)
	t.Cleanup(func() {
		tyde.SetInstance(nil)
		newSenderFunc = newSender
		fyne.CurrentApp().Preferences().SetBool(prefAtTop, false)
	})

	p := &panel{}
	p.build()
	p.send = rec
	return p, rec
}

// keyFor finds the button showing a given unshifted face, which is how the tests
// press keys.
func keyFor(t *testing.T, p *panel, lower string) *keyButton {
	t.Helper()

	for _, b := range p.keys {
		if b.def.lower == lower {
			return b
		}
	}
	t.Fatalf("no %q key on the keyboard", lower)
	return nil
}

func tap(t *testing.T, p *panel, lower string) {
	t.Helper()
	p.tapped(keyFor(t, p, lower).def)
}

func TestPanel_ShowAndHide(t *testing.T) {
	p, _ := newTestPanel(t)

	p.show()
	assert.True(t, p.shown)

	p.hide()
	assert.False(t, p.shown)
}

func TestPanel_Toggle(t *testing.T) {
	p, _ := newTestPanel(t)

	p.toggle()
	assert.True(t, p.shown)
	p.toggle()
	assert.False(t, p.shown)
}

func TestPanel_ShowConnectsOnce(t *testing.T) {
	tyde.SetInstance(wmTest.NewDesktop())
	calls := 0
	newSenderFunc = func() (sender, error) {
		calls++
		return &recorder{}, nil
	}
	t.Cleanup(func() {
		tyde.SetInstance(nil)
		newSenderFunc = newSender
	})

	p := &panel{}
	p.show()
	p.hide()
	p.show()

	assert.Equal(t, 1, calls, "the connection is reused between openings")
}

// A desktop with no way to fake input still shows the keyboard, so the problem
// is visible rather than the tray icon silently doing nothing.
func TestPanel_ShowWithoutASender(t *testing.T) {
	tyde.SetInstance(wmTest.NewDesktop())
	calls := 0
	newSenderFunc = func() (sender, error) {
		calls++
		return nil, errors.New("no X display")
	}
	t.Cleanup(func() {
		tyde.SetInstance(nil)
		newSenderFunc = newSender
	})

	p := &panel{}
	p.show()
	assert.True(t, p.shown)

	p.press(char("a", "A"))
	p.hide()
	p.show()
	assert.Equal(t, 1, calls, "a display that cannot type is not retried on every keystroke")
}

func TestPanel_TypesCharacters(t *testing.T) {
	p, rec := newTestPanel(t)

	tap(t, p, "a")
	tap(t, p, "1")

	assert.Equal(t, []stroke{{xproto.Keysym('a'), 0}, {xproto.Keysym('1'), 0}}, rec.sent)
}

func TestPanel_TypesSpecialKeys(t *testing.T) {
	p, rec := newTestPanel(t)

	tap(t, p, "Enter")
	tap(t, p, "Bksp")

	assert.Equal(t, []stroke{{symReturn, 0}, {symBackSpace, 0}}, rec.sent)
}

func TestPanel_ShiftIsOneShot(t *testing.T) {
	p, rec := newTestPanel(t)

	tap(t, p, "Shift")
	assert.True(t, p.shifted())
	assert.Equal(t, "A", keyFor(t, p, "a").Text, "the keys read as what they will type")

	tap(t, p, "a")
	tap(t, p, "a")

	assert.Equal(t, []stroke{
		{xproto.Keysym('A'), modShift},
		{xproto.Keysym('a'), 0},
	}, rec.sent, "Shift applies to the next key only")
	assert.False(t, p.shifted())
	assert.Equal(t, "a", keyFor(t, p, "a").Text)
}

func TestPanel_ShiftTappedTwiceCancels(t *testing.T) {
	p, rec := newTestPanel(t)

	tap(t, p, "Shift")
	tap(t, p, "Shift")
	tap(t, p, "a")

	assert.Equal(t, []stroke{{xproto.Keysym('a'), 0}}, rec.sent)
}

func TestPanel_CapsHoldsUntilTappedAgain(t *testing.T) {
	p, rec := newTestPanel(t)

	tap(t, p, "Caps")
	tap(t, p, "a")
	tap(t, p, "b")
	tap(t, p, "Caps")
	tap(t, p, "c")

	assert.Equal(t, []stroke{
		{xproto.Keysym('A'), modShift},
		{xproto.Keysym('B'), modShift},
		{xproto.Keysym('c'), 0},
	}, rec.sent)
}

func TestPanel_ModifiersLatchForOneKey(t *testing.T) {
	p, rec := newTestPanel(t)

	tap(t, p, "Ctrl")
	assert.Equal(t, modControl, p.held)
	tap(t, p, "c")
	tap(t, p, "c")

	assert.Equal(t, []stroke{
		{xproto.Keysym('c'), modControl},
		{xproto.Keysym('c'), 0},
	}, rec.sent)
}

func TestPanel_ModifiersCombine(t *testing.T) {
	p, rec := newTestPanel(t)

	tap(t, p, "Ctrl")
	tap(t, p, "Alt")
	tap(t, p, "Bksp")

	assert.Equal(t, []stroke{{symBackSpace, modControl | modAlt}}, rec.sent)
}

// Shift latched alongside another modifier still reaches a named key, which is
// how Shift+Tab (going backwards through a form) is typed.
func TestPanel_ShiftReachesSpecialKeys(t *testing.T) {
	p, rec := newTestPanel(t)

	tap(t, p, "Shift")
	tap(t, p, "Tab")

	assert.Equal(t, []stroke{{symTab, modShift}}, rec.sent)
}

func TestPanel_HideDropsLatchedModifiers(t *testing.T) {
	p, rec := newTestPanel(t)

	p.show()
	tap(t, p, "Ctrl")
	p.hide()

	p.show()
	tap(t, p, "c")
	assert.Equal(t, []stroke{{xproto.Keysym('c'), 0}}, rec.sent,
		"a modifier left latched does not leak into the next opening")
}

func TestPanel_TypingErrorsDoNotStopTheKeyboard(t *testing.T) {
	p, rec := newTestPanel(t)
	rec.err = errors.New("could not type")

	tap(t, p, "a")
	tap(t, p, "b")

	assert.Len(t, rec.sent, 2, "a failed keystroke does not stop the next one")
}

func TestPanel_FlipMovesToTheOtherEdge(t *testing.T) {
	p, _ := newTestPanel(t)
	p.show()

	size := p.size()
	bottom := p.position(size)

	p.flip()
	top := p.position(size)

	assert.Less(t, top.Y, bottom.Y)
	assert.Equal(t, bottom.X, top.X, "the keyboard stays centred")
	assert.True(t, p.shown, "flipping keeps the keyboard up")
	assert.True(t, fyne.CurrentApp().Preferences().Bool(prefAtTop), "the chosen edge is remembered")
}

// The keyboard fills the space between the bar and the widget panel: a key at
// either end has to be reachable without a gap of desktop beside it.
func TestPanel_SpansTheContentArea(t *testing.T) {
	p, _ := newTestPanel(t)

	screen := tyde.Instance().Screens().Primary()
	x, y, w, h := tyde.Instance().ContentBoundsPixels(screen)
	scale := screen.CanvasScale()

	size := p.size()
	pos := p.position(size)

	assert.Equal(t, float32(w)/scale, size.Width, "as wide as the content area")
	assert.Equal(t, float32(x)/scale, pos.X, "starting where the bar ends")
	assert.Equal(t, float32(y+h)/scale, pos.Y+size.Height, "flush with the bottom edge")

	// Docked the other way up it is flush with the top of the content area.
	p.flip()
	assert.Equal(t, float32(y)/scale, p.position(p.size()).Y)
}

// A narrower screen leaves less between the two panels, and the keyboard takes
// what is there rather than running under them.
func TestPanel_NarrowScreenShrinksTheKeyboard(t *testing.T) {
	p, _ := newTestPanel(t)
	wide := p.size().Width

	screen := tyde.Instance().Screens().Primary()
	screen.Width = 800
	size := p.size()

	assert.Less(t, size.Width, wide)
	assert.Equal(t, float32(800)-wmTheme.NarrowBarWidth-wmTheme.WidgetPanelWidth, size.Width)
	assert.GreaterOrEqual(t, p.position(size).X, float32(0))
}

func TestPanel_FitsOnScreen(t *testing.T) {
	p, _ := newTestPanel(t)

	screen := tyde.Instance().Screens().Primary()
	width := float32(screen.Width) / screen.CanvasScale()
	height := float32(screen.Height) / screen.CanvasScale()

	size := p.size()
	pos := p.position(size)

	assert.LessOrEqual(t, size.Width, width)
	assert.LessOrEqual(t, size.Height, height)
	assert.GreaterOrEqual(t, pos.X, float32(0))
	assert.GreaterOrEqual(t, pos.Y, float32(0))
	assert.LessOrEqual(t, pos.Y+size.Height, height)
}

// With no screen to measure the keyboard still has a size, so a desktop that
// cannot describe itself does not put an empty overlay on screen.
func TestPanel_WithoutAScreen(t *testing.T) {
	p, _ := newTestPanel(t)
	tyde.SetInstance(nil)

	size := p.size()
	assert.Equal(t, fallbackWidth, size.Width)
	assert.Greater(t, size.Height, float32(0))
	assert.Equal(t, fyne.NewPos(0, 0), p.position(size))
}

// The background runs past the edge the keyboard is docked to, so its rounded
// corners fall off the screen there and the keyboard meets the edge square.
func TestPanel_BackgroundOvershootsTheDockedEdge(t *testing.T) {
	p, _ := newTestPanel(t)

	size := p.size()
	content := p.content.(*fyne.Container)
	content.Resize(size)

	bg := content.Objects[0]
	assert.Equal(t, size.Width, bg.Size().Width, "no overshoot sideways, the panels are there")
	assert.Equal(t, size.Height+cornerRadius(), bg.Size().Height)
	assert.Equal(t, float32(0), bg.Position().Y, "docked at the bottom it overshoots downwards")

	p.atTop = true
	content.Refresh()
	assert.Equal(t, -cornerRadius(), bg.Position().Y)
}

func TestPanel_DestroyClosesTheConnection(t *testing.T) {
	p, rec := newTestPanel(t)
	p.show()

	p.destroy()

	assert.False(t, p.shown)
	assert.True(t, rec.closed)
}
