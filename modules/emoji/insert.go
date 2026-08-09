// Emoji insertion: putting the picked character straight into the window you
// came from, the way the Windows and macOS pickers do, using the XTEST extension
// to synthesise key presses.
//
// NOTE: nothing calls this yet. The picker copies to the clipboard instead,
// because insertion did not reliably reach the target window; this is kept as
// the starting point for a second attempt.
//
// X11 has no "type this string" request, so the sequence per rune is the one
// xdotool uses: bind a spare (unused) keycode to the rune's Unicode keysym, ask
// the server to fake a press and release of that keycode, then unbind it. The
// keystroke is delivered to whichever window holds the input focus, and showing
// a Tyde overlay moves the focus to the desktop window (so the picker's search
// box works) - so the focus is handed to the target window for the duration of
// the typing and handed straight back.
package emoji

import (
	"errors"
	"fmt"
	"time"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgb/xtest"
)

// keymapSettleDelay is how long we wait after rebinding a keycode before faking
// the key. The server broadcasts a MappingNotify and toolkits reload their
// keymap asynchronously, so typing immediately can deliver the *old* keysym.
// xdotool uses 12ms by default for the same reason.
const keymapSettleDelay = 12 * time.Millisecond

// unicodeKeysymBase is the X11 convention for keysyms that carry a Unicode code
// point directly: 0x01000000 plus the code point. Latin-1 code points are their
// own keysym and must not be offset.
const unicodeKeysymBase = 0x01000000

// inserter types text into another window. It is an interface so the picker can
// be tested without an X server.
type inserter interface {
	// Focused reports the window that currently holds the X input focus, to be
	// captured before the picker takes focus for itself.
	Focused() (xproto.Window, error)
	// Insert types text into target, restoring the previous focus afterwards.
	Insert(text string, target xproto.Window) error
	// Close releases the X connection.
	Close()
}

// x11Inserter talks to the X server over its own connection. The window
// manager's connection is busy servicing the event loop, and the focus/keymap
// juggling below has to be strictly ordered against itself, so a dedicated
// connection keeps the two from interleaving.
type x11Inserter struct {
	conn *xgb.Conn
	root xproto.Window
}

// newInserter connects to the display and initialises XTEST. It fails when there
// is no X server or the extension is missing, in which case the picker falls
// back to the clipboard alone.
func newInserter() (inserter, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("no X display for emoji insertion: %w", err)
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

	return &x11Inserter{conn: conn, root: setup.DefaultScreen(conn).Root}, nil
}

func (i *x11Inserter) Close() {
	if i.conn != nil {
		i.conn.Close()
		i.conn = nil
	}
}

// Focused returns the window holding the X input focus.
func (i *x11Inserter) Focused() (xproto.Window, error) {
	reply, err := xproto.GetInputFocus(i.conn).Reply()
	if err != nil {
		return 0, err
	}
	return reply.Focus, nil
}

// Insert types text into target and hands the focus back to whoever held it -
// which, while the picker is up, is the desktop window hosting the overlay. Both
// focus changes and every key event travel the same connection, so the server
// processes them in this order.
func (i *x11Inserter) Insert(text string, target xproto.Window) error {
	if target == 0 || target == xproto.WindowNone {
		return errors.New("no target window to insert into")
	}

	restore, err := i.Focused()
	if err != nil {
		return fmt.Errorf("could not read current focus: %w", err)
	}

	if err := xproto.SetInputFocusChecked(i.conn, xproto.InputFocusPointerRoot,
		target, xproto.TimeCurrentTime).Check(); err != nil {
		return fmt.Errorf("could not focus the target window: %w", err)
	}
	// However the typing goes, the focus must come back or the picker goes deaf.
	defer xproto.SetInputFocus(i.conn, xproto.InputFocusPointerRoot, restore, xproto.TimeCurrentTime)

	code, err := i.spareKeycode()
	if err != nil {
		return err
	}
	defer i.unbind(code)

	for _, r := range text {
		if err := i.typeRune(r, code); err != nil {
			return err
		}
	}
	return nil
}

// typeRune binds the rune to the scratch keycode, fakes a press and release, and
// leaves the binding for the caller to clear.
func (i *x11Inserter) typeRune(r rune, code xproto.Keycode) error {
	if err := i.bind(code, keysymFor(r)); err != nil {
		return err
	}
	time.Sleep(keymapSettleDelay)

	if err := xtest.FakeInputChecked(i.conn, xproto.KeyPress, byte(code), 0,
		i.root, 0, 0, 0).Check(); err != nil {
		return fmt.Errorf("could not fake key press: %w", err)
	}
	if err := xtest.FakeInputChecked(i.conn, xproto.KeyRelease, byte(code), 0,
		i.root, 0, 0, 0).Check(); err != nil {
		return fmt.Errorf("could not fake key release: %w", err)
	}
	return nil
}

// spareKeycode finds a keycode with no keysyms bound to it, which we can borrow
// as a scratch slot without disturbing the user's keyboard layout.
func (i *x11Inserter) spareKeycode() (xproto.Keycode, error) {
	setup := xproto.Setup(i.conn)
	first := setup.MinKeycode
	count := int(setup.MaxKeycode) - int(setup.MinKeycode) + 1

	mapping, err := xproto.GetKeyboardMapping(i.conn, first, byte(count)).Reply()
	if err != nil {
		return 0, fmt.Errorf("could not read the keyboard mapping: %w", err)
	}

	per := int(mapping.KeysymsPerKeycode)
	if per == 0 {
		return 0, errors.New("keyboard mapping reported no keysyms per keycode")
	}

	for k := 0; k < count; k++ {
		used := false
		for s := 0; s < per; s++ {
			idx := k*per + s
			if idx < len(mapping.Keysyms) && mapping.Keysyms[idx] != 0 {
				used = true
				break
			}
		}
		if !used {
			return xproto.Keycode(int(first) + k), nil
		}
	}
	return 0, errors.New("no spare keycode to type through")
}

// bind points every shift level of a keycode at one keysym, so the rune arrives
// whatever modifiers happen to be held.
func (i *x11Inserter) bind(code xproto.Keycode, sym xproto.Keysym) error {
	const levels = 2 // unshifted and shifted is enough for a scratch binding

	syms := make([]xproto.Keysym, levels)
	for n := range syms {
		syms[n] = sym
	}
	if err := xproto.ChangeKeyboardMappingChecked(i.conn, 1, code, levels, syms).Check(); err != nil {
		return fmt.Errorf("could not bind a keycode to type through: %w", err)
	}
	return nil
}

// unbind clears the scratch keycode so the borrowed slot is left as we found it.
func (i *x11Inserter) unbind(code xproto.Keycode) {
	xproto.ChangeKeyboardMapping(i.conn, 1, code, 2, []xproto.Keysym{0, 0})
}

// keysymFor maps a rune to its X11 keysym. Latin-1 runes are their own keysym;
// everything else uses the Unicode keysym range.
func keysymFor(r rune) xproto.Keysym {
	if r < 0x100 {
		return xproto.Keysym(r)
	}
	return xproto.Keysym(unicodeKeysymBase + uint32(r))
}
