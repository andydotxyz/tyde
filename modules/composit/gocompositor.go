package composit

// A Fyne-based compositor that captures X11 window content and displays it
// as canvas.Image objects in the Fyne desktop window.
// Based on the original X RENDER compositor, which was based on
// https://github.com/bvkgo/gcompositor and https://github.com/jmanc3/xcompmgr-simple/.

import (
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"github.com/FyshOS/saver"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/composite"
	"github.com/BurntSushi/xgb/damage"
	"github.com/BurntSushi/xgb/shape"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgbutil"

	"fyshos.com/fynedesk"
)

type opaqueType string

type client struct {
	title      string
	win        xproto.Window
	opacity    uint32
	opaqueType opaqueType
	damaged    bool
	skipped    bool // Fyne Desktop window or other skipped windows
	fullscreen bool // window has _NET_WM_STATE_FULLSCREEN

	geom       xproto.GetGeometryReply
	attributes xproto.GetWindowAttributesReply
	damage     damage.Damage
	pixmap     xproto.Pixmap // cached NameWindowPixmap
}

var (
	defaultScreen int
	rootWindow    xproto.Window
	rootWidth     uint16
	rootHeight    uint16
	allDamage     bool
	clients       []*client

	opacityAtom    xproto.Atom
	netWmNameAtom  xproto.Atom
	netWmStateAtom xproto.Atom
	utf8StringAtom xproto.Atom
	atomAtom       xproto.Atom
	wmNameAtom     xproto.Atom
	stringAtom     xproto.Atom
)

const (
	solid       opaqueType = ""
	transparent opaqueType = "TRANSPARENT"
	argb        opaqueType = "ARGB"

	opaque = math.MaxUint32
)

type cookieReply[R any] interface {
	Reply() (R, error)
}

func initExtension[S any, T cookieReply[S]](conn *xgb.Conn, initFunc func(conn *xgb.Conn) error,
	verFunc func(*xgb.Conn, uint32, uint32) T, major, minor uint32,
) error {
	if err := initFunc(conn); err != nil {
		return err
	}
	_, err := verFunc(conn, major, minor).Reply()
	if err != nil {
		return err
	}

	return nil
}

func setup(conn *xgb.Conn) error {
	if err := initExtension[*composite.QueryVersionReply, composite.QueryVersionCookie](conn, composite.Init, composite.QueryVersion, 0, 2); err != nil {
		return err
	}
	if err := initExtension[*damage.QueryVersionReply, damage.QueryVersionCookie](conn, damage.Init, damage.QueryVersion, 1, 1); err != nil {
		return err
	}
	if err := shape.Init(conn); err != nil {
		return err
	}
	if _, err := shape.QueryVersion(conn).Reply(); err != nil {
		return err
	}

	err := setupRoot(conn)
	if err != nil {
		return err
	}
	if err = registerManager(conn, defaultScreen); err != nil {
		return err
	}

	err = func() error {
		if err = xproto.GrabServerChecked(conn).Check(); err != nil {
			return err
		}
		defer xproto.UngrabServer(conn)

		err = composite.RedirectSubwindowsChecked(conn, rootWindow, composite.RedirectManual).Check()
		if err != nil {
			return err
		}

		mask := []uint32{
			xproto.EventMaskSubstructureNotify |
				xproto.EventMaskExposure |
				xproto.EventMaskStructureNotify |
				xproto.EventMaskPropertyChange,
		}
		if err = xproto.ChangeWindowAttributesChecked(conn, rootWindow, xproto.CwEventMask, mask).Check(); err != nil {
			return err
		}

		name := "_NET_WM_WINDOW_OPACITY"
		opacityAtomReply, err := xproto.InternAtom(conn, false, uint16(len(name)), name).Reply()
		if err != nil {
			return err
		}
		opacityAtom = opacityAtomReply.Atom

		stateAtomName := "_NET_WM_STATE"
		stateAtomReply, err := xproto.InternAtom(conn, false, uint16(len(stateAtomName)), stateAtomName).Reply()
		if err != nil {
			return err
		}
		netWmStateAtom = stateAtomReply.Atom

		tree, err := xproto.QueryTree(conn, rootWindow).Reply()
		if err != nil {
			return err
		}
		for _, child := range tree.Children {
			if err := addClient(conn, child); err != nil {
				return err
			}
		}
		return nil
	}()
	if err != nil {
		return err
	}

	return nil
}

//gocyclo:ignore
func run(done chan struct{}, w *compositorWidget, overlay *compositorWidget) error {
	c, err := xgbutil.NewConn()
	if err != nil {
		return err
	}

	conn := c.Conn()
	defer conn.Close()

	ws := &widgets{normal: w, overlay: overlay}

	// Set up the screen scale function for the widget
	screenScaleFunc = func() float32 {
		inst := fynedesk.Instance()
		if inst == nil {
			return 1
		}
		return inst.Screens().Primary().CanvasScale()
	}

	err = setup(conn)
	if err != nil {
		return err
	}

	// Add all initially mapped windows to the appropriate widget.
	for _, c := range clients {
		if c.skipped || c.attributes.MapState != xproto.MapStateViewable {
			continue
		}
		c.fullscreen = checkFullscreen(c)
		ws.targetFor(c).ensureWindow(c.win)
	}
	syncOrder(ws)
	ws.refreshBoth()

	// Initial capture of all visible windows
	refreshWindows(conn, ws)

	for {
		select {
		case <-done:
			return nil
		default:
			ev, err := conn.WaitForEvent()
			var repaint bool

			if err != nil {
				var badDamageError *damage.BadDamageError
				if errors.As(err, &badDamageError) {
					repaint = true
				}

				if err != nil {
					fyne.LogError("error waiting for event", err)
					continue
				}
			}

			switch e := ev.(type) {
			case xproto.CreateNotifyEvent:
				if err := addClient(conn, e.Window); err != nil {
					fyne.LogError("failed to add client", err)
					repaint = true
				}
			case xproto.ConfigureNotifyEvent:
				if err := configureClient(conn, ws, e); err != nil {
					fyne.LogError("failed to configure client", err)
					repaint = true
				}
			case xproto.DestroyNotifyEvent:
				destroyWin(conn, ws, e.Window)
			case xproto.MapNotifyEvent:
				if err := mapWin(conn, ws, e.Window); err != nil {
					fyne.LogError("failed to map window", err)
					repaint = true
				}
			case xproto.UnmapNotifyEvent:
				unmapWin(conn, ws, e.Window)
			case xproto.ReparentNotifyEvent:
				if e.Parent == rootWindow {
					if err := addClient(conn, e.Window); err != nil {
						fyne.LogError("failed to add client", err)
						repaint = true
					}
				} else {
					destroyWin(conn, ws, e.Window)
				}
			case xproto.CirculateNotifyEvent:
				circulateClient(ws, e)
			case xproto.PropertyNotifyEvent:
				if e.Atom == opacityAtom {
					if c := getClientFromWindow(e.Window); c != nil {
						updateOpacity(conn, 1, c)
						allDamage = true
					}
				}
				if e.Atom == netWmStateAtom {
					if cl := getClientFromWindow(e.Window); cl != nil {
						updateFullscreen(ws, cl)
					}
				}
			case damage.NotifyEvent:
				if err := damageClient(conn, &e); err != nil {
					fyne.LogError("failed to send damage notify", err)
					repaint = true
				}
			}

			if allDamage || repaint {
				refreshTranslucency(conn, ws)
				refreshWindows(conn, ws)
				allDamage = false
			}

			conn.Sync()
		}
	}
}

func registerManager(conn *xgb.Conn, screen int) error {
	atomName := fmt.Sprintf("_NET_WM_CM_S%d", screen)
	atom, err := xproto.InternAtom(conn, false, uint16(len(atomName)), atomName).Reply()
	if err != nil {
		return err
	}
	owner, err := xproto.GetSelectionOwner(conn, atom.Atom).Reply()
	if err != nil {
		return err
	}
	if owner.Owner != 0 {
		log.Printf("Another composite manager is running")
		return fmt.Errorf("another composite manager is running: %w", os.ErrExist)
	}
	win, err := xproto.NewWindowId(conn)
	if err != nil {
		return err
	}
	err = xproto.CreateWindowChecked(conn, xproto.WindowClassCopyFromParent, win, rootWindow,
		0, 0, 1, 1, 0, xproto.WindowClassInputOutput, 0, 0, nil).Check()
	if err != nil {
		return err
	}
	err = xproto.SetSelectionOwnerChecked(conn, win, atom.Atom, xproto.TimeCurrentTime).Check()
	if err != nil {
		return err
	}
	return nil
}

func setupRoot(conn *xgb.Conn) error {
	screen := xproto.Setup(conn).DefaultScreen(conn)
	defaultScreen = conn.DefaultScreen
	rootWindow = screen.Root
	rootWidth = screen.WidthInPixels
	rootHeight = screen.HeightInPixels
	return nil
}

// widgets pairs the normal and overlay compositor widgets.
type widgets struct {
	normal  *compositorWidget
	overlay *compositorWidget
}

// targetFor returns the appropriate widget for a client based on fullscreen state.
func (ws *widgets) targetFor(c *client) *compositorWidget {
	if c.fullscreen {
		return ws.overlay
	}
	return ws.normal
}

// refreshBoth refreshes both widgets via fyne.Do.
func (ws *widgets) refreshBoth() {
	fyne.Do(func() {
		ws.normal.Refresh()
		ws.overlay.Refresh()
	})
}

// checkFullscreen detects fullscreen by checking if the window geometry
// exactly matches any screen. This works because fullscreen windows cover the
// entire screen, while maximized windows are smaller (content area only).
func checkFullscreen(c *client) bool {
	inst := fynedesk.Instance()
	if inst == nil {
		return false
	}
	for _, screen := range inst.Screens().Screens() {
		if int(c.geom.X) == screen.X && int(c.geom.Y) == screen.Y &&
			int(c.geom.Width) == screen.Width && int(c.geom.Height) == screen.Height {
			return true
		}
	}
	return false
}

// updateFullscreen checks whether a client's fullscreen state changed and
// moves it between the normal and overlay widgets if needed.
func updateFullscreen(ws *widgets, c *client) {
	wasFull := c.fullscreen
	c.fullscreen = checkFullscreen(c)
	if wasFull == c.fullscreen {
		return
	}

	var from, to *compositorWidget
	if c.fullscreen {
		from, to = ws.normal, ws.overlay
	} else {
		from, to = ws.overlay, ws.normal
	}

	from.removeWindow(c.win)
	to.ensureWindow(c.win)
	c.damaged = true
	allDamage = true
	syncOrder(ws)
	ws.refreshBoth()
}

// syncOrder rebuilds the image ordering in both widgets to match the clients list.
func syncOrder(ws *widgets) {
	order := make([]xproto.Window, len(clients))
	for i, c := range clients {
		order[i] = c.win
	}
	ws.normal.reorder(order)
	ws.overlay.reorder(order)
}

// refreshWindows captures all damaged windows and updates the Fyne widgets.
func refreshWindows(conn *xgb.Conn, ws *widgets) {
	for _, c := range clients {
		if !c.damaged || c.skipped {
			continue
		}
		if c.attributes.MapState != xproto.MapStateViewable {
			continue
		}
		if c.geom.X+int16(c.geom.Width) < 1 || c.geom.Y+int16(c.geom.Height) < 1 ||
			c.geom.X >= int16(rootWidth) || c.geom.Y >= int16(rootHeight) {
			continue
		}

		captureAndUpdateClient(conn, ws.targetFor(c), c)
	}
}

// refreshTranslucency updates the translucency of all visible windows
// without recapturing their content. Called after stacking order changes.
func refreshTranslucency(conn *xgb.Conn, ws *widgets) {
	type update struct {
		wi           *windowImage
		translucency float64
	}
	var updates []update

	for _, c := range clients {
		if c.skipped || c.attributes.MapState != xproto.MapStateViewable {
			continue
		}
		w := ws.targetFor(c)
		wi := w.getWindow(c.win)
		if wi == nil {
			continue
		}
		translucency := computeTranslucency(conn, c)
		if wi.img.Translucency != translucency {
			updates = append(updates, update{wi, translucency})
		}
	}

	if len(updates) > 0 {
		fyne.Do(func() {
			for _, u := range updates {
				u.wi.img.Translucency = u.translucency
			}
			ws.normal.Refresh()
			ws.overlay.Refresh()
		})
	}
}

func captureAndUpdateClient(conn *xgb.Conn, w *compositorWidget, c *client) {
	// Ensure we have a named pixmap
	if c.pixmap == 0 {
		pixmap, err := xproto.NewPixmapId(conn)
		if err != nil {
			return
		}
		if err = composite.NameWindowPixmapChecked(conn, c.win, pixmap).Check(); err != nil {
			return
		}
		c.pixmap = pixmap
	}

	totalW := c.geom.Width + c.geom.BorderWidth*2
	totalH := c.geom.Height + c.geom.BorderWidth*2

	isARGB := c.opaqueType == argb
	img := capturePixmap(conn, xproto.Drawable(c.pixmap), totalW, totalH, isARGB)
	if img == nil {
		return
	}

	// Calculate translucency
	translucency := computeTranslucency(conn, c)

	wi := w.getWindow(c.win)
	if wi == nil {
		return
	}

	fyne.Do(func() {
		wi.img.Image = img
		wi.img.Translucency = translucency
		wi.x = c.geom.X
		wi.y = c.geom.Y
		wi.w = totalW
		wi.h = totalH
		canvas.Refresh(wi.img)
		w.Refresh()
	})
}

func computeTranslucency(conn *xgb.Conn, c *client) float64 {
	if strings.Contains(c.title, "Terminal Overlay") {
		return 0.2
	}

	// Check if this is the top visible window
	isTop := true
	idx := indexFunc(clients, func(cl *client) bool { return cl.win == c.win })
	if idx > 0 {
		for j := idx - 1; j >= 0; j-- {
			if clients[j].skipped {
				continue
			}
			if strings.Contains(clients[j].title, "FyneDesk:skip") {
				continue
			}
			if ok, err := windowSkipped(conn, clients[j].win); err == nil && ok {
				continue
			}
			if clients[j].attributes.MapState == xproto.MapStateViewable {
				isTop = false
				break
			}
		}
	}

	if !isTop {
		return 0.2
	}

	// Check custom opacity
	if c.opacity != opaque {
		return 1.0 - float64(c.opacity)/float64(opaque)
	}

	return 0.0
}

// isScreensaver detects screensaver windows by checking for override-redirect
// fullscreen windows or known screensaver title patterns.
func isScreensaver(title string, attr *xproto.GetWindowAttributesReply, geom *xproto.GetGeometryReply) bool {
	lower := strings.ToLower(title)
	if strings.Contains(lower, "screensaver") || strings.Contains(lower, "screen saver") ||
		strings.Contains(lower, saver.WindowTitle) || title == "FyneDesk Screensaver" {
		return true
	}

	// Override-redirect windows covering a full screen are likely screensavers
	if attr.OverrideRedirect && geom.Width >= rootWidth && geom.Height >= rootHeight {
		return true
	}

	return false
}

func getClientFromWindow(window xproto.Window) *client {
	i := indexFunc(clients, func(c *client) bool {
		return c.win == window
	})
	if i == -1 {
		return nil
	}
	return clients[i]
}

func addClient(conn *xgb.Conn, window xproto.Window) error {
	attr, err := xproto.GetWindowAttributes(conn, window).Reply()
	if err != nil {
		return err
	}
	name, err := windowTitle(conn, window)
	if err != nil {
		return err
	}
	geom, err := xproto.GetGeometry(conn, xproto.Drawable(window)).Reply()
	if err != nil {
		return err
	}
	c := &client{
		title:      name,
		win:        window,
		attributes: *attr,
		geom:       *geom,
		opacity:    opaque,
		damaged:    false,
	}

	// Skip the Fyne Desktop root window, skip-hinted windows, and screensavers.
	// Skipped windows are unredirected so they render directly above the compositor.
	if strings.Contains(name, "Fyne Desktop") || strings.Contains(name, "FyneDesk:skip") ||
		isScreensaver(name, attr, geom) {
		c.skipped = true
		_ = composite.UnredirectWindowChecked(conn, window, composite.RedirectManual).Check()
	}

	if !c.skipped && attr.Class != xproto.WindowClassInputOnly {
		c.damage, err = damage.NewDamageId(conn)
		if err != nil {
			return err
		}
		if err = damage.CreateChecked(conn, c.damage, xproto.Drawable(window), damage.ReportLevelNonEmpty).Check(); err != nil {
			return err
		}
	}

	clients = append([]*client{c}, clients...)
	if c.attributes.MapState == xproto.MapStateViewable {
		return mapWin(conn, nil, window)
	}
	return nil
}

func mapWin(conn *xgb.Conn, ws *widgets, window xproto.Window) error {
	c := getClientFromWindow(window)
	if c == nil {
		return fmt.Errorf("could not get client for window %x", window)
	}

	mask := []uint32{xproto.EventMaskPropertyChange}
	_ = xproto.ChangeWindowAttributes(conn, window, xproto.CwEventMask, mask)

	c.attributes.MapState = xproto.MapStateViewable
	c.damaged = true
	updateOpacity(conn, 1, c)

	// Invalidate cached pixmap on map
	freeClientPixmap(conn, c)

	if ws != nil && !c.skipped {
		c.fullscreen = checkFullscreen(c)
		ws.targetFor(c).ensureWindow(c.win)
		syncOrder(ws)
		ws.refreshBoth()
	}

	allDamage = true
	return nil
}

func unmapWin(conn *xgb.Conn, ws *widgets, window xproto.Window) {
	c := getClientFromWindow(window)
	if c == nil {
		return
	}
	c.attributes.MapState = xproto.MapStateUnmapped
	c.damaged = false
	freeClientPixmap(conn, c)

	if ws != nil && !c.skipped {
		ws.targetFor(c).removeWindow(c.win)
		ws.refreshBoth()
	}

	allDamage = true
}

func destroyWin(conn *xgb.Conn, ws *widgets, window xproto.Window) {
	i := indexFunc(clients, func(c *client) bool {
		return c.win == window
	})
	if i == -1 {
		return
	}

	c := clients[i]
	freeClientPixmap(conn, c)
	if c.damage != 0 {
		_ = damage.Destroy(conn, c.damage).Check()
		c.damage = 0
	}

	if ws != nil && !c.skipped {
		ws.targetFor(c).removeWindow(c.win)
		ws.refreshBoth()
	}

	clients = delete(clients, i, i+1)
	allDamage = true
}

func freeClientPixmap(conn *xgb.Conn, c *client) {
	if c.pixmap != 0 {
		xproto.FreePixmap(conn, c.pixmap)
		c.pixmap = 0
	}
}

func configureClient(conn *xgb.Conn, ws *widgets, e xproto.ConfigureNotifyEvent) error {
	client := getClientFromWindow(e.Window)
	if client == nil {
		if e.Window == rootWindow {
			rootWidth = e.Width
			rootHeight = e.Height
		}
		return nil
	}

	if client.geom.Width != e.Width || client.geom.Height != e.Height {
		freeClientPixmap(conn, client)
	}

	client.geom.X = e.X
	client.geom.Y = e.Y
	client.geom.Width = e.Width
	client.geom.Height = e.Height
	client.geom.BorderWidth = e.BorderWidth
	client.attributes.OverrideRedirect = e.OverrideRedirect

	restackWin(ws, e.Window, e.AboveSibling)

	// Check if fullscreen state changed due to geometry change
	updateFullscreen(ws, client)

	if !client.skipped {
		w := ws.targetFor(client)
		wi := w.getWindow(client.win)
		if wi != nil {
			totalW := client.geom.Width + client.geom.BorderWidth*2
			totalH := client.geom.Height + client.geom.BorderWidth*2
			fyne.Do(func() {
				wi.x = client.geom.X
				wi.y = client.geom.Y
				wi.w = totalW
				wi.h = totalH
				w.Refresh()
			})
		}
	}

	client.damaged = true
	allDamage = true
	return nil
}

func restackWin(ws *widgets, window, target xproto.Window) {
	i := indexFunc(clients, func(c *client) bool { return c.win == window })
	if i == -1 {
		return
	}
	c := clients[i]
	clients = delete(clients, i, i+1)

	if target == 0 {
		clients = append(clients, c)
	} else {
		j := indexFunc(clients, func(c *client) bool { return c.win == target })
		if j == -1 {
			clients = append(clients, c)
		} else {
			clients = insert(clients, j, c)
		}
	}

	if ws != nil {
		syncOrder(ws)
		ws.refreshBoth()
	}
}

func circulateClient(ws *widgets, e xproto.CirculateNotifyEvent) {
	client := getClientFromWindow(e.Window)
	if client == nil {
		return
	}
	var target xproto.Window
	if e.Place == xproto.PlaceOnTop {
		target = clients[0].win
	} else if e.Place == xproto.PlaceOnBottom {
		target = 0
	}
	restackWin(ws, client.win, target)
	allDamage = true
}

func damageClient(conn *xgb.Conn, e *damage.NotifyEvent) error {
	client := getClientFromWindow(xproto.Window(e.Drawable))
	if client == nil {
		return nil
	}

	if err := damage.SubtractChecked(conn, client.damage, 0, 0).Check(); err != nil {
		return err
	}

	client.damaged = true
	allDamage = true
	return nil
}

func updateOpacity(conn *xgb.Conn, fallback float32, c *client) {
	opacity, err := getOpacity(conn, c.win)
	if err != nil {
		if fallback < 1.0 {
			opacity = uint32(fallback * float32(opaque))
		} else {
			opacity = opaque
		}
	}
	c.opacity = opacity

	c.opaqueType = solid
	// Detect ARGB visuals by checking if the window has 32-bit depth
	if c.geom.Depth == 32 {
		c.opaqueType = argb
	} else if opacity != opaque {
		c.opaqueType = transparent
	}
}

func windowTitle(conn *xgb.Conn, window xproto.Window) (string, error) {
	if netWmNameAtom == 0 {
		a := "_NET_WM_NAME"
		atom, err := xproto.InternAtom(conn, false, uint16(len(a)), a).Reply()
		if err != nil {
			return "", err
		}
		netWmNameAtom = atom.Atom
	}

	if utf8StringAtom == 0 {
		b := "UTF8_STRING"
		atom, err := xproto.InternAtom(conn, false, uint16(len(b)), b).Reply()
		if err != nil {
			return "", err
		}
		utf8StringAtom = atom.Atom
	}

	prop, err := xproto.GetProperty(conn, false, window, netWmNameAtom, utf8StringAtom, 0, 1024).Reply()
	if err == nil && prop.Type == utf8StringAtom && len(prop.Value) > 0 {
		return string(prop.Value), nil
	}

	if wmNameAtom == 0 {
		c := "WM_NAME"
		atom, err := xproto.InternAtom(conn, false, uint16(len(c)), c).Reply()
		if err != nil {
			return "", err
		}
		wmNameAtom = atom.Atom
	}

	if stringAtom == 0 {
		d := "STRING"
		atom, err := xproto.InternAtom(conn, false, uint16(len(d)), d).Reply()
		if err != nil {
			return "", fmt.Errorf("failed to intern STRING atom: %v", err)
		}
		stringAtom = atom.Atom
	}

	prop, err = xproto.GetProperty(conn, false, window, wmNameAtom, stringAtom, 0, 1024).Reply()
	if err == nil && prop.Type == stringAtom && len(prop.Value) > 0 {
		return string(prop.Value), nil
	}
	return "Unnamed", nil
}

func windowSkipped(conn *xgb.Conn, window xproto.Window) (bool, error) {
	if netWmStateAtom == 0 {
		a := "_NET_WM_STATE"
		atom, err := xproto.InternAtom(conn, false, uint16(len(a)), a).Reply()
		if err != nil {
			return false, err
		}
		netWmStateAtom = atom.Atom
	}

	if atomAtom == 0 {
		b := "ATOM"
		atom, err := xproto.InternAtom(conn, false, uint16(len(b)), b).Reply()
		if err != nil {
			return false, err
		}
		atomAtom = atom.Atom
	}

	prop, err := xproto.GetProperty(conn, false, window, netWmStateAtom, atomAtom, 0, 1024).Reply()
	if err == nil && prop.Type == atomAtom && len(prop.Value) > 0 {
		aid := xproto.Atom(xgb.Get32(prop.Value))
		reply, err := xproto.GetAtomName(conn, aid).Reply()
		if err != nil {
			return false, fmt.Errorf("AtomName: Error fetching name for ATOM "+
				"id '%d': %s", aid, err)
		}
		if reply.Name == "_NET_WM_STATE_SKIP_TASKBAR" {
			return true, nil
		}

		return false, nil
	}

	return false, err
}

func getOpacity(conn *xgb.Conn, window xproto.Window) (uint32, error) {
	if opacityAtom == 0 {
		name := "_NET_WM_WINDOW_OPACITY"
		opacityAtomReply, err := xproto.InternAtom(conn, false, uint16(len(name)), name).Reply()
		if err != nil {
			return opaque, err
		}
		opacityAtom = opacityAtomReply.Atom
	}

	reply, err := xproto.GetProperty(conn, false, window, opacityAtom, xproto.GetPropertyTypeAny, 0, (1<<32)-1).Reply()
	if err != nil {
		return opaque, err
	}
	if reply.Format == 0 {
		return opaque, os.ErrNotExist
	}
	if reply.Format != 32 {
		return opaque, fmt.Errorf("unexpected format %d", reply.Format)
	}
	return xgb.Get32(reply.Value), nil
}
