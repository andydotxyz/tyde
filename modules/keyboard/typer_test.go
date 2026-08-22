package keyboard

import (
	"testing"

	"fyne.io/fyne/v2"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/FyshOS/saver"
	"github.com/stretchr/testify/assert"

	"fyshos.com/tyde"
	wmTest "fyshos.com/tyde/test"
)

// newTestSender builds a sender with a hand-written keyboard mapping instead of
// one read from an X server: keycode 10 is "a"/"A", keycode 11 is Shift, and
// keycode 12 is unbound so it can be borrowed.
func newTestSender() *x11Sender {
	return &x11Sender{
		first: 10,
		per:   2,
		keysyms: []xproto.Keysym{
			'a', 'A',
			symShiftL, 0,
			0, 0,
		},
	}
}

func TestSender_Lookup(t *testing.T) {
	s := newTestSender()

	code, shifted, found := s.lookup('a')
	assert.True(t, found)
	assert.Equal(t, xproto.Keycode(10), code)
	assert.False(t, shifted)

	// The capital sits on the shifted level of the same key, so typing it means
	// holding Shift as well.
	code, shifted, found = s.lookup('A')
	assert.True(t, found)
	assert.Equal(t, xproto.Keycode(10), code)
	assert.True(t, shifted)

	_, _, found = s.lookup('z')
	assert.False(t, found, "a character the layout has no key for")
}

func TestSender_LookupWithoutAMapping(t *testing.T) {
	s := &x11Sender{}

	_, _, found := s.lookup('a')
	assert.False(t, found)
}

func TestSender_SpareKeycode(t *testing.T) {
	s := newTestSender()

	code, err := s.spareKeycode()
	assert.NoError(t, err)
	assert.Equal(t, xproto.Keycode(12), code, "the only keycode with nothing bound to it")
}

func TestSender_SpareKeycodeWhenTheKeyboardIsFull(t *testing.T) {
	s := &x11Sender{first: 10, per: 1, keysyms: []xproto.Keysym{'a', 'b'}}

	_, err := s.spareKeycode()
	assert.Error(t, err)
}

// stackWM is the smallest window manager typingTarget needs: an ordered window
// list, topmost first, matching the real stack.
type stackWM struct {
	windows []tyde.Window
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
func (s *stackWM) Windows() []tyde.Window                               { return s.windows }
func (s *stackWM) AddStackListener(tyde.StackListener)                  {}
func (s *stackWM) RemoveStackListener(tyde.StackListener)               {}
func (s *stackWM) Blank()                                               {}
func (s *stackWM) Close()                                               {}
func (s *stackWM) Run()                                                 {}
func (s *stackWM) ShowOverlay(fyne.Window, fyne.Size, fyne.Position)    {}
func (s *stackWM) ShowMenuOverlay(*fyne.Menu, fyne.Size, fyne.Position) {}
func (s *stackWM) ShowScreensaver(*saver.ScreenSaver)                   {}

// testWindow is a virtual window that knows its X id, which the test Window on
// its own always reports as 0.
type testWindow struct {
	*wmTest.Window

	id xproto.Window
}

func newTestWindow(id xproto.Window) *testWindow {
	return &testWindow{Window: wmTest.NewWindow("test"), id: id}
}

func (w *testWindow) ChildID() xproto.Window { return w.id }

func TestTypingTarget(t *testing.T) {
	wm := &stackWM{}
	wmTest.NewDesktopWithWM(wm)
	t.Cleanup(func() { tyde.SetInstance(nil) })

	// No windows at all: there is nowhere for the typing to go.
	_, ok := typingTarget()
	assert.False(t, ok)

	wm.AddWindow(newTestWindow(42))
	win, ok := typingTarget()
	assert.True(t, ok)
	assert.Equal(t, xproto.Window(42), win)

	// A newer window on top takes the typing.
	wm.AddWindow(newTestWindow(43))
	win, _ = typingTarget()
	assert.Equal(t, xproto.Window(43), win)
}

func TestTypingTarget_SkipsIconifiedWindows(t *testing.T) {
	wm := &stackWM{}
	wmTest.NewDesktopWithWM(wm)
	t.Cleanup(func() { tyde.SetInstance(nil) })

	wm.AddWindow(newTestWindow(42))
	top := newTestWindow(43)
	top.Iconify()
	wm.AddWindow(top)

	win, ok := typingTarget()
	assert.True(t, ok)
	assert.Equal(t, xproto.Window(42), win, "a minimised window is not typed into")
}

func TestTypingTarget_SkipsOtherDesktops(t *testing.T) {
	wm := &stackWM{}
	wmTest.NewDesktopWithWM(wm)
	t.Cleanup(func() { tyde.SetInstance(nil) })

	wm.AddWindow(newTestWindow(42))
	top := newTestWindow(43)
	top.SetDesktop(2) // the test desktop is always desktop 0
	wm.AddWindow(top)

	win, ok := typingTarget()
	assert.True(t, ok)
	assert.Equal(t, xproto.Window(42), win)
}

func TestTypingTarget_WithoutAWindowManager(t *testing.T) {
	tyde.SetInstance(wmTest.NewDesktop())
	t.Cleanup(func() { tyde.SetInstance(nil) })

	_, ok := typingTarget()
	assert.False(t, ok, "an embedded desktop has no X windows to type into")
}
