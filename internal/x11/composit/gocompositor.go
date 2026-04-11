//go:build linux || openbsd || freebsd || netbsd
// +build linux openbsd freebsd netbsd

// Package composit provides an X11 compositor that captures window content
// and displays it as Fyne canvas images in the desktop window.
// Based on https://github.com/bvkgo/gcompositor and https://github.com/jmanc3/xcompmgr-simple/.
package composit

import (
	"errors"
	"fmt"
	"image"
	"log"
	"math"
	"os"
	"strings"
	"time"

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
	"fyshos.com/fynedesk/internal/ui"
	"fyshos.com/fynedesk/internal/x11"
)

type opaqueType string

type client struct {
	win          xproto.Window
	opacity      uint32
	opaqueType   opaqueType
	damaged      bool
	skipped      bool // Fyne Desktop window or other skipped windows
	fullscreened bool // unredirected for fullscreen bypass
	visualMoving bool // position being managed by VisualMoveCallback (drag/animation)

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
	return err
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

	return nil
}

// Run starts the X11 compositor event loop. It captures window content and
// renders it into per-screen widgets. The normal widget shows regular windows;
// the overlay widget shows fullscreen windows above desktop chrome.
// Run blocks until done is closed.
//
//gocyclo:ignore
func Run(done chan struct{}, screenComps []ui.ScreenCompositors) error {
	c, err := xgbutil.NewConn()
	if err != nil {
		return err
	}

	conn := c.Conn()
	defer conn.Close()

	ws := &widgets{}
	for _, sc := range screenComps {
		ws.screens = append(ws.screens, screenWidgets{
			screen:  sc.Screen,
			normal:  sc.Normal,
			overlay: sc.Overlay,
		})
	}

	// Set up visual move callback for fast drag repositioning.
	// Called from the main thread (fyne.Do context) so no additional queueing needed.
	x11.VisualMoveCallback = func(winID uint32, absX, absY int16, width, height uint16) {
		win := xproto.Window(winID)

		// Mark the client as visually moving so refreshWindows skips recapture
		if c := getClientFromWindow(win); c != nil {
			c.visualMoving = true
		}

		// Update position on all screens that have a cached entry, and
		// add to new screens if the window now overlaps them.
		for i := range ws.screens {
			sw := &ws.screens[i]
			localX := absX - int16(sw.screen.X)
			localY := absY - int16(sw.screen.Y)

			// Always update position for existing cached entries so
			// windows animate smoothly even as they leave the screen.
			for _, target := range []*ui.CompositorWidget{sw.normal, sw.overlay} {
				wi := target.GetWindow(winID)
				if wi == nil {
					continue
				}
				wi.X = localX
				wi.Y = localY
				wi.W = width
				wi.H = height
				scale := sw.screen.CanvasScale()
				wi.Img.Move(fyne.NewPos(float32(localX)/scale, float32(localY)/scale))
			}

			// If the window newly overlaps this screen and has no entry, create one.
			if intersectsScreen(absX, absY, width, height, sw.screen) {
				if sw.normal.GetWindow(winID) == nil && sw.overlay.GetWindow(winID) == nil {
					wi := sw.normal.EnsureWindow(winID)
					copyImageFromOtherScreen(ws, winID, wi, sw)
					wi.X = localX
					wi.Y = localY
					wi.W = width
					wi.H = height
					scale := sw.screen.CanvasScale()
					wi.Img.Move(fyne.NewPos(float32(localX)/scale, float32(localY)/scale))
					wi.Img.Resize(fyne.NewSize(float32(width)/scale, float32(height)/scale))
					sw.normal.Refresh()
				}
			}
		}
	}

	err = setup(conn)
	if err != nil {
		return err
	}

	// Wait for the WM to finish framing existing windows, then scan the
	// tree once in its settled state. This avoids the race between the WM's
	// async framing and the compositor's initial scan that causes flicker.
	time.Sleep(200 * time.Millisecond)

	// Drain events that accumulated during the wait
	conn.Sync()
	for {
		ev, err := conn.PollForEvent()
		if ev == nil && err == nil {
			break
		}
		_ = err
	}

	// Scan the tree in its final state — all windows are now framed
	tree, err := xproto.QueryTree(conn, rootWindow).Reply()
	if err != nil {
		return err
	}
	for _, child := range tree.Children {
		_ = addClient(conn, child)
	}

	// Populate widgets from the settled clients list
	for _, c := range clients {
		if c.skipped || c.attributes.MapState != xproto.MapStateViewable {
			continue
		}
		if isFullscreenClient(c) {
			updateFullscreen(conn, ws, c)
		} else {
			ensureWindowOnScreens(ws, c)
		}
	}
	syncOrder(ws)
	ws.refreshAll()

	// Initial capture of all visible windows
	refreshWindows(conn, ws)

	// Ensure the top window has focus after compositor settles
	if inst := fynedesk.Instance(); inst != nil {
		if top := inst.WindowManager().TopWindow(); top != nil {
			top.Focus()
		}
	}

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
						updateFullscreen(conn, ws, cl)
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

// screenWidgets holds the compositor widgets for a single screen.
type screenWidgets struct {
	screen  *fynedesk.Screen
	normal  *ui.CompositorWidget
	overlay *ui.CompositorWidget
}

// widgets holds per-screen compositor widget pairs.
type widgets struct {
	screens []screenWidgets
}

// targetFor returns the appropriate widget type name for a client based on fullscreen state.
func isFullscreenClient(c *client) bool {
	return checkFullscreen(c)
}

// screensForClient returns the screen widgets whose screens overlap the client's geometry.
func (ws *widgets) screensForClient(c *client) []*screenWidgets {
	var result []*screenWidgets
	for i := range ws.screens {
		if intersectsScreen(c.geom.X, c.geom.Y,
			c.geom.Width+c.geom.BorderWidth*2, c.geom.Height+c.geom.BorderWidth*2,
			ws.screens[i].screen) {
			result = append(result, &ws.screens[i])
		}
	}
	if len(result) == 0 && len(ws.screens) > 0 {
		// Fallback: assign to nearest screen (use ScreenForGeometry)
		inst := fynedesk.Instance()
		if inst != nil {
			s := inst.Screens().ScreenForGeometry(int(c.geom.X), int(c.geom.Y),
				int(c.geom.Width), int(c.geom.Height))
			for i := range ws.screens {
				if ws.screens[i].screen == s {
					result = append(result, &ws.screens[i])
					break
				}
			}
		}
	}
	return result
}

// refreshAll refreshes all screen widgets via fyne.Do.
func (ws *widgets) refreshAll() {
	fyne.Do(func() {
		for i := range ws.screens {
			ws.screens[i].normal.Refresh()
			ws.screens[i].overlay.Refresh()
		}
	})
}

// intersectsScreen returns whether a rectangle overlaps a screen.
func intersectsScreen(x, y int16, w, h uint16, screen *fynedesk.Screen) bool {
	return int(x) < screen.X+screen.Width &&
		int(x)+int(w) > screen.X &&
		int(y) < screen.Y+screen.Height &&
		int(y)+int(h) > screen.Y
}

// copyImageFromOtherScreen copies the canvas.Image data from another screen's
// widget entry for the same window ID into the target WindowImage.
func copyImageFromOtherScreen(ws *widgets, winID uint32, target *ui.WindowImage, exclude *screenWidgets) {
	for i := range ws.screens {
		sw := &ws.screens[i]
		if sw == exclude {
			continue
		}
		for _, w := range []*ui.CompositorWidget{sw.normal, sw.overlay} {
			if src := w.GetWindow(winID); src != nil && src.Img.Image != nil {
				target.Img.Image = src.Img.Image
				target.Img.Translucency = src.Img.Translucency
				return
			}
		}
	}
}

// wmWindow returns the WM's Window for a compositor client, or nil.
func wmWindow(c *client) fynedesk.Window {
	inst := fynedesk.Instance()
	if inst == nil || inst.WindowManager() == nil {
		return nil
	}
	for _, w := range inst.WindowManager().Windows() {
		xw, ok := w.(x11.XWin)
		if ok && xw.FrameID() == c.win {
			return w
		}
	}
	return nil
}

// checkFullscreen returns whether the WM considers this window fullscreen.
func checkFullscreen(c *client) bool {
	w := wmWindow(c)
	return w != nil && w.Fullscreened()
}

// updateFullscreen checks whether a client's fullscreen state changed and
// unredirects/redirects the window so that fullscreen windows bypass compositing.
func updateFullscreen(conn *xgb.Conn, ws *widgets, c *client) {
	if c.skipped {
		return // root window or screensaver — don't touch
	}

	winID := uint32(c.win)
	isFS := isFullscreenClient(c)

	if isFS && !c.fullscreened {
		// Unredirect: let X11 display the window directly, bypassing compositing.
		removeWindowFromAllScreens(ws, winID)
		freeClientPixmap(conn, c)
		if c.damage != 0 {
			_ = damage.Destroy(conn, c.damage)
			c.damage = 0
		}
		_ = composite.UnredirectWindowChecked(conn, c.win, composite.RedirectManual).Check()
		c.fullscreened = true
	} else if !isFS && c.fullscreened {
		// Re-redirect: bring the window back under compositing.
		_ = composite.RedirectWindowChecked(conn, c.win, composite.RedirectManual).Check()
		c.fullscreened = false
		if c.damage == 0 {
			dmg, err := damage.NewDamageId(conn)
			if err == nil {
				if err = damage.CreateChecked(conn, dmg, xproto.Drawable(c.win), damage.ReportLevelNonEmpty).Check(); err == nil {
					c.damage = dmg
				}
			}
		}
		ensureWindowOnScreens(ws, c)
		c.damaged = true
		allDamage = true
		syncOrder(ws)
		ws.refreshAll()
	}
}

// syncOrder rebuilds the image ordering in all screen widgets to match the clients list.
func syncOrder(ws *widgets) {
	order := make([]uint32, len(clients))
	for i, c := range clients {
		order[i] = uint32(c.win)
	}
	for i := range ws.screens {
		ws.screens[i].normal.Reorder(order)
		ws.screens[i].overlay.Reorder(order)
	}
}

// ensureWindowOnScreens adds a window to all screen widgets that overlap its geometry.
func ensureWindowOnScreens(ws *widgets, c *client) {
	winID := uint32(c.win)
	isFS := isFullscreenClient(c)
	for _, sw := range ws.screensForClient(c) {
		if isFS {
			sw.overlay.EnsureWindow(winID)
		} else {
			sw.normal.EnsureWindow(winID)
		}
	}
}

// removeWindowFromAllScreens removes a window from all screen widgets.
func removeWindowFromAllScreens(ws *widgets, winID uint32) {
	for i := range ws.screens {
		ws.screens[i].normal.RemoveWindow(winID)
		ws.screens[i].overlay.RemoveWindow(winID)
	}
}

// refreshWindows captures all damaged windows and updates the Fyne widgets.
func refreshWindows(conn *xgb.Conn, ws *widgets) {
	type captured struct {
		c            *client
		img          *image.NRGBA
		translucency float64
		totalW       uint16
		totalH       uint16
	}
	var caps []captured

	for _, c := range clients {
		if !c.damaged || c.skipped || c.fullscreened || c.visualMoving {
			continue
		}
		if c.attributes.MapState != xproto.MapStateViewable {
			continue
		}
		if c.geom.X+int16(c.geom.Width) < 1 || c.geom.Y+int16(c.geom.Height) < 1 ||
			c.geom.X >= int16(rootWidth) || c.geom.Y >= int16(rootHeight) {
			continue
		}

		// Ensure we have a named pixmap
		if c.pixmap == 0 {
			pixmap, err := xproto.NewPixmapId(conn)
			if err != nil {
				continue
			}
			if err = composite.NameWindowPixmapChecked(conn, c.win, pixmap).Check(); err != nil {
				continue
			}
			c.pixmap = pixmap
		}

		totalW := c.geom.Width + c.geom.BorderWidth*2
		totalH := c.geom.Height + c.geom.BorderWidth*2
		isARGB := c.opaqueType == argb
		img := capturePixmap(conn, xproto.Drawable(c.pixmap), totalW, totalH, isARGB)
		if img == nil {
			continue
		}
		w := wmWindow(c)
		if w == nil || (!w.Fullscreened() && !w.Maximized()) {
			// Use primary screen scale for corner rounding as a reasonable default
			scale := float32(1)
			if len(ws.screens) > 0 {
				scale = ws.screens[0].screen.CanvasScale()
			}
			roundCorners(img, int(5*scale))
		}

		caps = append(caps, captured{
			c:            c,
			img:          img,
			translucency: computeTranslucency(conn, c),
			totalW:       totalW,
			totalH:       totalH,
		})
	}

	if len(caps) == 0 {
		return
	}

	fyne.Do(func() {
		for _, cap := range caps {
			c := cap.c
			winID := uint32(c.win)
			isFS := isFullscreenClient(c)

			for _, sw := range ws.screensForClient(c) {
				var target *ui.CompositorWidget
				if isFS {
					target = sw.overlay
				} else {
					target = sw.normal
				}
				wi := target.GetWindow(winID)
				if wi == nil {
					continue
				}

				wi.Img.Image = cap.img
				wi.Img.Translucency = cap.translucency

				localX := c.geom.X - int16(sw.screen.X)
				localY := c.geom.Y - int16(sw.screen.Y)
				scale := sw.screen.CanvasScale()

				// Set position/size if not yet initialized (first capture).
				if wi.W == 0 {
					wi.X = localX
					wi.Y = localY
					wi.W = cap.totalW
					wi.H = cap.totalH
					wi.Img.Move(fyne.NewPos(float32(localX)/scale, float32(localY)/scale))
					wi.Img.Resize(fyne.NewSize(float32(cap.totalW)/scale, float32(cap.totalH)/scale))
				}
				canvas.Refresh(wi.Img)
			}
		}
	})
}

// refreshTranslucency updates the translucency of all visible windows
// without recapturing their content.
func refreshTranslucency(conn *xgb.Conn, ws *widgets) {
	type update struct {
		wi           *ui.WindowImage
		translucency float64
	}
	var updates []update

	for _, c := range clients {
		if c.skipped || c.fullscreened || c.attributes.MapState != xproto.MapStateViewable {
			continue
		}
		winID := uint32(c.win)
		translucency := computeTranslucency(conn, c)

		for i := range ws.screens {
			sw := &ws.screens[i]
			for _, target := range []*ui.CompositorWidget{sw.normal, sw.overlay} {
				wi := target.GetWindow(winID)
				if wi == nil {
					continue
				}
				if wi.Img.Translucency != translucency {
					updates = append(updates, update{wi, translucency})
				}
			}
		}
	}

	if len(updates) > 0 {
		fyne.Do(func() {
			for _, u := range updates {
				u.wi.Img.Translucency = u.translucency
			}
			ws.refreshAll()
		})
	}
}

func computeTranslucency(conn *xgb.Conn, c *client) float64 {
	// Check if this is the top visible window
	isTop := true
	idx := -1
	for i, cl := range clients {
		if cl.win == c.win {
			idx = i
			break
		}
	}
	if idx > 0 {
		for j := idx - 1; j >= 0; j-- {
			if clients[j].skipped {
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

// isScreensaver detects screensaver windows.
func isScreensaver(title string, attr *xproto.GetWindowAttributesReply, geom *xproto.GetGeometryReply) bool {
	lower := strings.ToLower(title)
	if strings.Contains(lower, "screensaver") || strings.Contains(lower, "screen saver") ||
		strings.Contains(lower, saver.WindowTitle) || title == "FyneDesk Screensaver" {
		return true
	}

	// Override-redirect windows covering a full screen are likely screensavers
	if attr.OverrideRedirect {
		if inst := fynedesk.Instance(); inst != nil {
			for _, screen := range inst.Screens().Screens() {
				if geom.Width >= uint16(screen.Width) && geom.Height >= uint16(screen.Height) {
					return true
				}
			}
		}
	}

	return false
}

func getClientFromWindow(window xproto.Window) *client {
	for _, c := range clients {
		if c.win == window {
			return c
		}
	}
	return nil
}

func addClient(conn *xgb.Conn, window xproto.Window) error {
	if getClientFromWindow(window) != nil {
		return nil
	}

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
		win:        window,
		attributes: *attr,
		geom:       *geom,
		opacity:    opaque,
		damaged:    false,
	}

	// Skip the Fyne Desktop root window, skip-hinted windows, and screensavers.
	if strings.Contains(name, ui.RootWindowName) || isScreensaver(name, attr, geom) {
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

	if !c.skipped {
		name, _ := windowTitle(conn, c.win)
		geom := &c.geom
		if strings.Contains(name, ui.RootWindowName) || isScreensaver(name, &c.attributes, geom) {
			c.skipped = true
			_ = composite.UnredirectWindowChecked(conn, c.win, composite.RedirectManual).Check()
			if c.damage != 0 {
				_ = damage.Destroy(conn, c.damage).Check()
				c.damage = 0
			}
		}
	}

	freeClientPixmap(conn, c)

	if ws != nil && !c.skipped {
		ensureWindowOnScreens(ws, c)
		syncOrder(ws)
		ws.refreshAll()
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
		removeWindowFromAllScreens(ws, uint32(c.win))
		ws.refreshAll()
	}

	allDamage = true
}

func destroyWin(conn *xgb.Conn, ws *widgets, window xproto.Window) {
	var i int
	var c *client
	for i, c = range clients {
		if c.win == window {
			goto found
		}
	}
	return

found:
	freeClientPixmap(conn, c)
	if c.damage != 0 {
		_ = damage.Destroy(conn, c.damage).Check()
		c.damage = 0
	}

	if ws != nil && !c.skipped {
		removeWindowFromAllScreens(ws, uint32(c.win))
		ws.refreshAll()
	}

	clients = append(clients[:i], clients[i+1:]...)
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

	client.visualMoving = false // X11 position synced, drag/animation ended

	resized := client.geom.Width != e.Width || client.geom.Height != e.Height
	if resized {
		freeClientPixmap(conn, client)
	}

	client.geom.X = e.X
	client.geom.Y = e.Y
	client.geom.Width = e.Width
	client.geom.Height = e.Height
	client.geom.BorderWidth = e.BorderWidth
	client.attributes.OverrideRedirect = e.OverrideRedirect

	restackClientOnly(e.Window, e.AboveSibling)

	if client.skipped {
		return nil
	}

	updateFullscreen(conn, ws, client)
	if client.fullscreened {
		return nil
	}

	// Update screen membership: remove from screens the window no longer overlaps,
	// add to screens it now overlaps.
	winID := uint32(client.win)
	isFS := isFullscreenClient(client)
	overlapping := ws.screensForClient(client)
	screenChanged := false
	for i := range ws.screens {
		sw := &ws.screens[i]
		hasNormal := sw.normal.GetWindow(winID) != nil
		hasOverlay := sw.overlay.GetWindow(winID) != nil
		if !hasNormal && !hasOverlay {
			// Window appeared on a new screen — create the entry and
			// copy image data from whichever screen previously had it.
			var wi *ui.WindowImage
			if isFS {
				wi = sw.overlay.EnsureWindow(winID)
			} else {
				wi = sw.normal.EnsureWindow(winID)
			}
			copyImageFromOtherScreen(ws, winID, wi, sw)
			screenChanged = true
		}
	}

	if screenChanged {
		// Force recapture so the new screen gets a fresh image
		client.damaged = true
		allDamage = true
	}

	syncOrder(ws)
	ws.refreshAll()

	totalW := client.geom.Width + client.geom.BorderWidth*2
	totalH := client.geom.Height + client.geom.BorderWidth*2

	for _, sw := range overlapping {
		var target *ui.CompositorWidget
		if isFS {
			target = sw.overlay
		} else {
			target = sw.normal
		}
		wi := target.GetWindow(winID)
		if wi == nil {
			continue
		}

		localX := client.geom.X - int16(sw.screen.X)
		localY := client.geom.Y - int16(sw.screen.Y)

		if resized || screenChanged {
			fyne.Do(func() {
				wi.X = localX
				wi.Y = localY
				wi.W = totalW
				wi.H = totalH
				target.Refresh()
			})
			client.damaged = true
			allDamage = true
		} else {
			wi.X = localX
			wi.Y = localY
			scale := sw.screen.CanvasScale()
			fyne.Do(func() {
				wi.Img.Move(fyne.NewPos(float32(localX)/scale, float32(localY)/scale))
			})
		}
	}

	return nil
}

func restackClientOnly(window, target xproto.Window) {
	i := -1
	for idx, c := range clients {
		if c.win == window {
			i = idx
			break
		}
	}
	if i == -1 {
		return
	}
	c := clients[i]
	clients = append(clients[:i], clients[i+1:]...)

	if target == 0 {
		clients = append(clients, c)
	} else {
		j := -1
		for idx, c := range clients {
			if c.win == target {
				j = idx
				break
			}
		}
		if j == -1 {
			clients = append(clients, c)
		} else {
			clients = append(clients[:j], append([]*client{c}, clients[j:]...)...)
		}
	}
}

func restackWin(ws *widgets, window, target xproto.Window) {
	restackClientOnly(window, target)

	if ws != nil {
		syncOrder(ws)
		ws.refreshAll()
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
