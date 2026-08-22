package keyboard

import (
	"errors"
	"fmt"
	"time"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgb/xtest"

	"fyshos.com/tyde"
)

// keymapSettleDelay is how long to wait after rebinding a keycode before faking the key.
const keymapSettleDelay = 12 * time.Millisecond

// sender types a key into the focused application. It is an interface so the
// panel can be exercised without an X server.
type sender interface {
	// Send types one keysym with the given modifiers held around it.
	Send(sym xproto.Keysym, mods modifier) error
	// Close releases the X connection.
	Close()
}

// newSenderFunc builds the sender the panel types through. It is a variable so
// tests can swap in a recorder.
var newSenderFunc = newSender

// x11Sender talks to the X server over its own connection.
// A dedicated connection keeps these events from interleaving with Tyde's X handler.
type x11Sender struct {
	conn *xgb.Conn
	root xproto.Window

	// The keyboard mapping, read once and re-read only when a lookup misses.
	first   xproto.Keycode
	per     int
	keysyms []xproto.Keysym
}

// newSender connects to the display and initialises XTEST.
func newSender() (sender, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("no X display to type into: %w", err)
	}
	if err := xtest.Init(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("XTEST extension unavailable: %w", err)
	}

	setup := xproto.Setup(conn)
	if setup == nil || len(setup.Roots) == 0 {
		conn.Close()
		return nil, errors.New("X server reported no screens")
	}

	s := &x11Sender{conn: conn, root: setup.DefaultScreen(conn).Root}
	if err := s.readMapping(); err != nil {
		conn.Close()
		return nil, err
	}
	return s, nil
}

func (s *x11Sender) Close() {
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
}

// Send types one keysym into the application the user is working in.
func (s *x11Sender) Send(sym xproto.Keysym, mods modifier) error {
	if sym == 0 {
		return errors.New("no character to type")
	}
	s.focusTarget()

	code, shifted, found := s.lookup(sym)
	if !found {
		// A layout we have not seen since it changed, or a character it simply
		// does not have a key for.
		if err := s.readMapping(); err == nil {
			code, shifted, found = s.lookup(sym)
		}
	}
	if !found {
		return s.sendBorrowed(sym, mods)
	}

	if shifted {
		mods |= modShift // the character lives on the shifted level of that key
	}
	return s.tap(code, mods)
}

// focusTarget hands the X input focus to the window the typing is meant for.
func (s *x11Sender) focusTarget() {
	win, ok := typingTarget()
	if !ok {
		return
	}
	xproto.SetInputFocus(s.conn, xproto.InputFocusPointerRoot, win, xproto.TimeCurrentTime)
}

// tap fakes a press and release of a keycode with modifiers held around it.
func (s *x11Sender) tap(code xproto.Keycode, mods modifier) error {
	held := s.holdModifiers(mods)
	defer s.releaseModifiers(held)

	if err := s.fake(xproto.KeyPress, code); err != nil {
		return err
	}
	return s.fake(xproto.KeyRelease, code)
}

// holdModifiers presses the keys for the requested modifiers, returning those it
// managed to press so they can be released again.
func (s *x11Sender) holdModifiers(mods modifier) []xproto.Keycode {
	var held []xproto.Keycode
	for _, m := range modifierSyms {
		if mods&m.mod == 0 {
			continue
		}
		code, _, found := s.lookup(m.sym)
		if !found {
			continue
		}
		if err := s.fake(xproto.KeyPress, code); err != nil {
			continue
		}
		held = append(held, code)
	}
	return held
}

// releaseModifiers lets the held keys go, innermost first.
func (s *x11Sender) releaseModifiers(held []xproto.Keycode) {
	for i := len(held) - 1; i >= 0; i-- {
		s.fake(xproto.KeyRelease, held[i])
	}
}

func (s *x11Sender) fake(kind byte, code xproto.Keycode) error {
	err := xtest.FakeInputChecked(s.conn, kind, byte(code), 0, s.root, 0, 0, 0).Check()
	if err != nil {
		return fmt.Errorf("could not fake a key event: %w", err)
	}
	return nil
}

// sendBorrowed types a keysym the user's layout has no key for, by binding it to
// an unused keycode for the length of one press. This is the slow path: the
// keymap change is broadcast to every client on the display, so it is worth
// avoiding for anything the layout can already reach.
func (s *x11Sender) sendBorrowed(sym xproto.Keysym, mods modifier) error {
	code, err := s.spareKeycode()
	if err != nil {
		return err
	}
	defer s.unbind(code)

	if err := s.bind(code, sym); err != nil {
		return err
	}
	time.Sleep(keymapSettleDelay)
	return s.tap(code, mods)
}

// readMapping loads the user's keyboard layout: which keysyms sit on which key,
// at which shift level.
func (s *x11Sender) readMapping() error {
	setup := xproto.Setup(s.conn)
	first := setup.MinKeycode
	count := int(setup.MaxKeycode) - int(setup.MinKeycode) + 1

	reply, err := xproto.GetKeyboardMapping(s.conn, first, byte(count)).Reply()
	if err != nil {
		return fmt.Errorf("could not read the keyboard mapping: %w", err)
	}
	if reply.KeysymsPerKeycode == 0 {
		return errors.New("keyboard mapping reported no keysyms per keycode")
	}

	s.first = first
	s.per = int(reply.KeysymsPerKeycode)
	s.keysyms = reply.Keysyms
	return nil
}

// lookup finds the key that already carries a keysym in the user's layout, and
// whether Shift has to be held to reach it.
func (s *x11Sender) lookup(sym xproto.Keysym) (code xproto.Keycode, shifted, found bool) {
	if s.per == 0 {
		return 0, false, false
	}

	levels := min(s.per, 2)
	for k := 0; (k+1)*s.per <= len(s.keysyms); k++ {
		for level := 0; level < levels; level++ {
			if s.keysyms[k*s.per+level] != sym {
				continue
			}
			return xproto.Keycode(int(s.first) + k), level == 1, true
		}
	}
	return 0, false, false
}

// spareKeycode finds a keycode with no keysyms bound to it, which can be
// borrowed as a scratch slot without disturbing the user's layout.
func (s *x11Sender) spareKeycode() (xproto.Keycode, error) {
	if s.per == 0 {
		return 0, errors.New("no keyboard mapping to borrow a keycode from")
	}

	for k := 0; (k+1)*s.per <= len(s.keysyms); k++ {
		used := false
		for level := 0; level < s.per; level++ {
			if s.keysyms[k*s.per+level] != 0 {
				used = true
				break
			}
		}
		if !used {
			return xproto.Keycode(int(s.first) + k), nil
		}
	}
	return 0, errors.New("no spare keycode to type through")
}

// bind points every shift level of a keycode at one keysym, so the character
// arrives whatever modifiers happen to be held.
func (s *x11Sender) bind(code xproto.Keycode, sym xproto.Keysym) error {
	const levels = 2 // unshifted and shifted is enough for a scratch binding

	syms := make([]xproto.Keysym, levels)
	for n := range syms {
		syms[n] = sym
	}
	if err := xproto.ChangeKeyboardMappingChecked(s.conn, 1, code, levels, syms).Check(); err != nil {
		return fmt.Errorf("could not bind a keycode to type through: %w", err)
	}
	return nil
}

// unbind clears the scratch keycode, leaving the borrowed slot as we found it.
func (s *x11Sender) unbind(code xproto.Keycode) {
	xproto.ChangeKeyboardMapping(s.conn, 1, code, 2, []xproto.Keysym{0, 0})
}

// xWindow is the part of a managed X11 window this module needs: the id of the
// client window that key events are delivered to. Mirroring the X11 specifics to avoid import.
type xWindow interface {
	ChildID() xproto.Window
}

// typingTarget picks the window typed keys should go to: the topmost one that is
// neither iconified nor on another desktop.
func typingTarget() (xproto.Window, bool) {
	desk := tyde.Instance()
	if desk == nil {
		return 0, false
	}
	manager := desk.WindowManager()
	if manager == nil {
		return 0, false
	}

	current := desk.Desktop()
	for _, win := range manager.Windows() {
		if win.Iconic() || (win.Desktop() != current && !win.Pinned()) {
			continue
		}

		x, ok := win.(xWindow)
		if !ok {
			return 0, false // not an X11 desktop, so nothing to fake input to
		}
		return x.ChildID(), true
	}
	return 0, false
}
