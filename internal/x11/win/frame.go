//go:build linux || openbsd || freebsd || netbsd
// +build linux openbsd freebsd netbsd

package win

import (
	"context"
	"image"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BurntSushi/xgb/shape"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgbutil/ewmh"
	"github.com/BurntSushi/xgbutil/icccm"
	"github.com/BurntSushi/xgbutil/xwindow"
	"github.com/FyshOS/saver"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"

	"fyshos.com/tyde"
	"fyshos.com/tyde/internal/x11"
	wmTheme "fyshos.com/tyde/theme"
	"fyshos.com/tyde/wm"
)

const unmaximizeThreshold = 84

type frame struct {
	x, y                                int16
	width, height                       uint16
	childWidth, childHeight             uint16
	resizeStartWidth, resizeStartHeight uint16
	mouseX, mouseY                      int16
	moveX, moveY                        int16
	resizeStartX, resizeStartY          int16
	resizeBottom, resizeTop             bool
	resizeLeft, resizeRight             bool
	moveOnly, ignoreDrag                bool

	borderTop, borderTopRight             xproto.Pixmap
	borderTopGC, borderTopRightGC, rectGC xproto.Gcontext
	borderTopWidth                        uint16

	hovered    desktop.Hoverable
	clickCount int
	cancelFunc context.CancelFunc

	pendingGeometry chan *configureGeometry
	pendingMu       sync.Mutex
	transparency    int
	closed          atomic.Bool

	canvas test.WindowlessCanvas
	client *client
}

type configureGeometry struct {
	x, y          int16
	width, height uint16
	force         bool
}

func newFrame(c *client) *frame {
	attrs, err := xproto.GetGeometry(c.wm.Conn(), xproto.Drawable(c.win)).Reply()
	if err != nil {
		fyne.LogError("Get Geometry Error", err)
		return nil
	}

	f, err := xwindow.Generate(c.wm.X())
	if err != nil {
		fyne.LogError("Generate Window Error", err)
		return nil
	}
	x, y, w, h := attrs.X, attrs.Y, attrs.Width, attrs.Height
	full := c.Fullscreened()
	decorated := c.Properties().Decorated()
	maximized := c.Maximized()
	screen := tyde.Instance().Screens().ScreenForGeometry(int(x), int(y), int(w), int(h))
	borderWidth := uint16(wm.ScaleToPixels(wmTheme.BorderWidth, screen))
	titleHeight := uint16(wm.ScaleToPixels(wmTheme.TitleHeight, screen))
	if full || maximized {
		// Remember the original geometry so a later unfullscreen/unmaximize
		// has somewhere to restore to (NotifyFullscreen/NotifyMaximize is not
		// called for windows that come up already in this state).
		c.restoreX = attrs.X
		c.restoreY = attrs.Y
		c.restoreWidth = attrs.Width
		c.restoreHeight = attrs.Height

		activeHead := tyde.Instance().Screens().ScreenForGeometry(int(attrs.X), int(attrs.Y), int(attrs.Width), int(attrs.Height))
		x = int16(activeHead.X)
		y = int16(activeHead.Y)
		if full {
			w = uint16(activeHead.Width)
			h = uint16(activeHead.Height)
		} else {
			maxX, maxY, maxWidth, maxHeight := tyde.Instance().ContentBoundsPixels(activeHead)
			x += int16(maxX)
			y += int16(maxY)
			w = uint16(maxWidth)
			h = uint16(maxHeight)
		}
	} else if decorated {
		x -= int16(borderWidth)
		y -= int16(titleHeight)
		if x < 0 {
			x = 0
		}
		if y < 0 {
			y = 0
		}
		if !maximized {
			w = attrs.Width + borderWidth*2
			h = attrs.Height + borderWidth + titleHeight
		}
	}
	framed := &frame{client: c}
	framed.x = x
	framed.y = y
	values := []uint32{xproto.EventMaskStructureNotify | xproto.EventMaskSubstructureNotify |
		xproto.EventMaskSubstructureRedirect | xproto.EventMaskExposure |
		xproto.EventMaskButtonPress | xproto.EventMaskButtonRelease | xproto.EventMaskButtonMotion |
		xproto.EventMaskKeyPress | xproto.EventMaskPointerMotion | xproto.EventMaskFocusChange |
		xproto.EventMaskPropertyChange | xproto.EventMaskLeaveWindow}
	err = xproto.CreateWindowChecked(c.wm.Conn(), c.wm.X().Screen().RootDepth, f.Id, c.wm.X().RootWin(),
		x, y, w, h, 0, xproto.WindowClassInputOutput, c.wm.X().Screen().RootVisual,
		xproto.CwEventMask, values).Check()
	if err != nil {
		fyne.LogError("Create Window Error", err)
		return nil
	}
	c.id = f.Id

	framed.width = w
	framed.height = h
	if full || !decorated {
		framed.childWidth = w
		framed.childHeight = h
	} else {
		framed.childWidth = w - borderWidth*2
		framed.childHeight = h - borderWidth - titleHeight
	}

	title := "Tyde Border"
	childTitle := c.props.Title()
	if strings.Contains(strings.ToLower(childTitle), "screensaver") ||
		strings.Contains(strings.ToLower(childTitle), "screen saver") ||
		strings.Contains(strings.ToLower(childTitle), saver.WindowTitle) {
		title = "Tyde Screensaver"
	}
	_ = ewmh.WmNameSet(c.wm.X(), f.Id, title)
	var offsetX, offsetY int16 = 0, 0
	if !full && decorated {
		offsetX = int16(borderWidth)
		offsetY = int16(titleHeight)
		xproto.ReparentWindow(c.wm.Conn(), c.win, c.id, int16(borderWidth), int16(titleHeight))
		err = ewmh.FrameExtentsSet(c.wm.X(), c.win, &ewmh.FrameExtents{
			Left:  int(borderWidth),
			Right: int(borderWidth),
			Top:   int(titleHeight), Bottom: int(borderWidth),
		})
		if err != nil {
			fyne.LogError("", err)
		}
	} else {
		xproto.ReparentWindow(c.wm.Conn(), c.win, c.id, attrs.X, attrs.Y)
		err = ewmh.FrameExtentsSet(c.wm.X(), c.win, &ewmh.FrameExtents{Left: 0, Right: 0, Top: 0, Bottom: 0})
		if err != nil {
			fyne.LogError("", err)
		}
	}

	if full || maximized {
		xproto.ConfigureWindow(c.wm.Conn(), c.win, xproto.ConfigWindowX|xproto.ConfigWindowY|
			xproto.ConfigWindowWidth|xproto.ConfigWindowHeight,
			[]uint32{uint32(offsetX), uint32(offsetY), uint32(framed.childWidth), uint32(framed.childHeight)})
	}

	windowStateSet(c.wm.X(), c.win, icccm.StateNormal)
	c.frame = framed // set early so ScreenForWindow can resolve the correct screen
	framed.show()
	framed.applyTheme(true)
	framed.notifyInnerGeometry()

	return framed
}

func (f *frame) addBorder() {
	borderWidth := x11.BorderWidth(x11.XWin(f.client))
	titleHeight := x11.TitleHeight(x11.XWin(f.client))
	x := int16(borderWidth)
	y := int16(titleHeight)
	w := f.width
	h := f.height
	if !f.client.maximized {
		w := f.childWidth + borderWidth*2
		h := f.childHeight + borderWidth + titleHeight
		f.x -= x
		f.y -= y
		if f.x < 0 {
			f.x = 0
		}
		if f.y < 0 {
			f.y = 0
		}
		f.width = w
		f.height = h
	}
	f.applyTheme(true)

	xproto.ConfigureWindow(f.client.wm.Conn(), f.client.win, xproto.ConfigWindowX|xproto.ConfigWindowY|
		xproto.ConfigWindowWidth|xproto.ConfigWindowHeight,
		[]uint32{uint32(x), uint32(y), uint32(f.childWidth), uint32(f.childHeight)})
	xproto.ConfigureWindow(f.client.wm.Conn(), f.client.id, xproto.ConfigWindowX|xproto.ConfigWindowY|
		xproto.ConfigWindowWidth|xproto.ConfigWindowHeight,
		[]uint32{uint32(f.x), uint32(f.y), uint32(w), uint32(h)})

	err := ewmh.FrameExtentsSet(f.client.wm.X(), f.client.win, &ewmh.FrameExtents{Left: int(borderWidth), Right: int(borderWidth), Top: int(titleHeight), Bottom: int(borderWidth)})
	if err != nil {
		fyne.LogError("", err)
	}
	f.notifyInnerGeometry()
}

func (f *frame) applyBorderlessTheme() {
	xproto.ConfigureWindow(f.client.wm.Conn(), f.client.win, xproto.ConfigWindowX|xproto.ConfigWindowY|
		xproto.ConfigWindowWidth|xproto.ConfigWindowHeight,
		[]uint32{uint32(0), uint32(0), uint32(f.width), uint32(f.height)})
}

func (f *frame) applyTheme(force bool) {
	if f.client.Fullscreened() || !f.client.Properties().Decorated() {
		f.applyBorderlessTheme()
		return
	}

	f.checkScale()
	f.decorate(force)
}

func (f *frame) checkScale() {
	titleHeight := x11.TitleHeight(x11.XWin(f.client))
	borderWidth := x11.BorderWidth(x11.XWin(f.client))
	if !f.client.props.Decorated() {
		titleHeight = 0
		borderWidth = 0
	}

	if f.height-titleHeight-borderWidth != f.childHeight {
		f.updateGeometry(f.x, f.y, f.width, f.height, true)
		f.notifyInnerGeometry()
	}
}

func (f *frame) configureLoop() {
	f.pendingMu.Lock()
	ch := f.pendingGeometry
	f.pendingMu.Unlock()
	if ch == nil {
		return
	}

	var lastGeometry *configureGeometry
	change := false

	blanks := 0
	for {
		select {
		case g, ok := <-ch:
			if g == nil || !ok {
				return
			}
			lastGeometry = g
			change = true
			blanks = 0
		default:
			if change && lastGeometry != nil {
				f.updateGeometry(lastGeometry.x, lastGeometry.y, lastGeometry.width, lastGeometry.height, lastGeometry.force)
				change = false
			} else {
				blanks++
				if blanks > 1000 { // if 1000 ticks pass no resize pending
					f.endConfigureLoop()
					return
				}
			}
		}
	}
}

func (f *frame) endConfigureLoop() {
	f.pendingMu.Lock()
	if f.pendingGeometry != nil {
		close(f.pendingGeometry)
		f.pendingGeometry = nil
	}
	f.pendingMu.Unlock()

	// Sync the actual X11 window position to match the visual after drag
	f.updateGeometry(f.x, f.y, f.width, f.height, true)
}

func (f *frame) copyDecorationPixels(width, height, xoff, yoff uint32, img image.Image, pid xproto.Pixmap, draw xproto.Gcontext, depth byte) {
	// DATA is BGRx
	data := make([]byte, width*height*4)
	i := uint32(0)
	for y := uint32(0); y < height; y++ {
		for x := uint32(0); x < width; x++ {
			r, g, b, _ := img.At(int(xoff+x), int(yoff+y)).RGBA()

			data[i] = byte(b)
			data[i+1] = byte(g)
			data[i+2] = byte(r)
			data[i+3] = 0xff

			i += 4
		}
	}
	xproto.PutImage(f.client.wm.Conn(), xproto.ImageFormatZPixmap, xproto.Drawable(pid), draw,
		uint16(width), uint16(height), 0, int16(yoff), 0, depth, data)
}

func (f *frame) createPixmaps(depth byte) error {
	heightPix := x11.TitleHeight(x11.XWin(f.client))
	rightWidthPix := f.topRightPixelWidth()
	drawPix := f.width
	if f.canvas != nil {
		minPix := uint16(f.canvas.Content().MinSize().Width * f.canvas.Scale())
		if drawPix < minPix {
			drawPix = minPix
		}
	}
	f.borderTopWidth = drawPix - rightWidthPix

	pid, err := xproto.NewPixmapId(f.client.wm.Conn())
	if err != nil {
		return err
	}

	xproto.CreatePixmap(f.client.wm.Conn(), depth, pid,
		xproto.Drawable(f.client.wm.X().Screen().Root), f.borderTopWidth, heightPix)
	f.borderTop = pid

	pid, err = xproto.NewPixmapId(f.client.wm.Conn())
	if err != nil {
		return err
	}

	xproto.CreatePixmap(f.client.wm.Conn(), depth, pid,
		xproto.Drawable(f.client.wm.X().Screen().Root), rightWidthPix, heightPix)
	f.borderTopRight = pid

	backR, backG, backB, _ := theme.Color(theme.ColorNameDisabledButton).RGBA()
	if f.client.Focused() {
		backR, backG, backB, _ = theme.Color(theme.ColorNameOverlayBackground).RGBA()
	}
	bgColor := uint32(uint8(backR))<<16 | uint32(uint8(backG))<<8 | uint32(uint8(backB))

	f.rectGC, _ = xproto.NewGcontextId(f.client.wm.Conn())
	if err := xproto.CreateGCChecked(f.client.wm.Conn(), f.rectGC, xproto.Drawable(f.client.id), xproto.GcForeground, []uint32{bgColor}).Check(); err != nil {
		return err // frame window was destroyed
	}

	f.borderTopGC, _ = xproto.NewGcontextId(f.client.wm.Conn())
	xproto.CreateGC(f.client.wm.Conn(), f.borderTopGC, xproto.Drawable(f.borderTop), xproto.GcForeground, []uint32{bgColor})
	f.borderTopRightGC, _ = xproto.NewGcontextId(f.client.wm.Conn())
	xproto.CreateGC(f.client.wm.Conn(), f.borderTopRightGC, xproto.Drawable(f.borderTopRight), xproto.GcForeground, []uint32{bgColor})

	return nil
}

func (f *frame) decorate(force bool) {
	depth := f.client.wm.X().Screen().RootDepth

	fyne.Do(func() {
		if f.closed.Load() {
			return
		}

		refresh := force

		if refresh {
			f.freePixmaps()
		}
		if f.borderTop == 0 {
			err := f.createPixmaps(depth)
			if err != nil {
				fyne.LogError("New Pixmap Error", err)
				return
			}
			refresh = true
		}

		if refresh || f.canvas == nil {
			f.drawDecorationSync(f.borderTop, f.borderTopGC, f.borderTopRight, f.borderTopRightGC, depth)
		}

		f.applyDecorationToFrame()
	})
}

func (f *frame) applyDecorationToFrame() {
	heightPix := x11.TitleHeight(x11.XWin(f.client))
	rect := xproto.Rectangle{X: 0, Y: 0, Width: f.width, Height: f.height}
	if err := xproto.PolyFillRectangleChecked(f.client.wm.Conn(), xproto.Drawable(f.client.id), f.rectGC, []xproto.Rectangle{rect}).Check(); err != nil {
		return // frame window was destroyed
	}

	rightWidthPix := f.topRightPixelWidth()
	widthPix := f.width
	xproto.CopyArea(f.client.wm.Conn(), xproto.Drawable(f.borderTop), xproto.Drawable(f.client.id), f.borderTopGC,
		0, 0, 0, 0, widthPix, heightPix)
	xproto.CopyArea(f.client.wm.Conn(), xproto.Drawable(f.borderTopRight), xproto.Drawable(f.client.id), f.borderTopRightGC,
		0, 0, int16(f.width-rightWidthPix), 0, rightWidthPix, heightPix)
}

// drawDecorationSync renders the window border into pixmaps and copies them to the frame.
// Must be called from the main thread (via fyne.Do) to avoid font cache races.
func (f *frame) drawDecorationSync(pidTop xproto.Pixmap, drawTop xproto.Gcontext, pidTopRight xproto.Pixmap, drawTopRight xproto.Gcontext, depth byte) {
	screen := tyde.Instance().Screens().ScreenForWindow(f.client)
	scale := screen.CanvasScale()

	canMaximize := true
	if windowSizeFixed(f.client.wm.X(), f.client.win) ||
		!windowSizeCanMaximize(f.client.wm.X(), f.client) {
		canMaximize = false
	}

	heightPix := x11.TitleHeight(x11.XWin(f.client))
	rightWidthPix := f.topRightPixelWidth()

	if f.canvas == nil {
		cnv := software.NewCanvas()
		cnv.SetPadded(false)

		b := wm.NewBorder(f.client, f.client.Properties().Icon(), canMaximize)
		trans := &transparentTheme{Theme: theme.DefaultTheme(), frame: f}
		cnv.SetContent(container.NewThemeOverride(b, trans))
		f.canvas = cnv
	} else {
		b := f.canvas.Content().(*container.ThemeOverride).Content.(*wm.Border)
		b.CloseIntercept = func() {
			f.client.Close()
		}
		b.SetTitle(f.client.props.Title())
		b.SetMaximized(f.client.maximized)
		b.SetIcon(f.client.Properties().Icon())
		b.Refresh()
	}
	f.canvas.SetScale(scale)

	minWidth := f.canvas.Content().MinSize().Width
	winPixWidth := f.borderTopWidth + rightWidthPix
	winPtWidth := float32(winPixWidth) / scale
	drawWidth := fyne.Max(minWidth, winPtWidth)
	f.canvas.Resize(fyne.NewSize(drawWidth, wmTheme.TitleHeight+16))
	widthPix := uint16(drawWidth*f.canvas.Scale()) - rightWidthPix
	img := f.canvas.Capture()

	for i := uint16(0); i < heightPix; i++ {
		f.copyDecorationPixels(uint32(widthPix), 1, 0, uint32(i), img, pidTop, drawTop, depth)
	}
	f.copyDecorationPixels(uint32(rightWidthPix), uint32(heightPix), uint32(uint16(img.Bounds().Dx())-rightWidthPix), 0, img, pidTopRight, drawTopRight, depth)
}

func (f *frame) freePixmaps() {
	if f.borderTop != 0 {
		xproto.FreePixmap(f.client.wm.Conn(), f.borderTop)
		f.borderTop = 0
	}
	if f.borderTopRight != 0 {
		xproto.FreePixmap(f.client.wm.Conn(), f.borderTopRight)
		f.borderTopRight = 0
	}

	if f.rectGC != 0 {
		xproto.FreeGC(f.client.wm.Conn(), f.rectGC)
		f.rectGC = 0
	}
	if f.borderTopGC != 0 {
		xproto.FreeGC(f.client.wm.Conn(), f.borderTopGC)
		f.borderTopGC = 0
	}
	if f.borderTopRightGC != 0 {
		xproto.FreeGC(f.client.wm.Conn(), f.borderTopRightGC)
		f.borderTopRightGC = 0
	}
}

func (f *frame) getInnerWindowCoordinates(w uint16, h uint16) (uint32, uint32, uint32, uint32) {
	if f.client.Fullscreened() || !f.client.Properties().Decorated() {
		constrainW, constrainH := w, h
		if !f.client.Properties().Decorated() {
			adjustedW, adjustedH := windowSizeWithIncrement(f.client.wm.X(), f.client.win, w, h)
			constrainW, constrainH = windowSizeConstrain(f.client.wm.X(), f.client.win,
				adjustedW, adjustedH)
		}
		f.width = constrainW
		f.height = constrainH
		f.height = constrainH
		return 0, 0, uint32(constrainW), uint32(constrainH)
	}

	borderWidth := x11.BorderWidth(x11.XWin(f.client))
	titleHeight := x11.TitleHeight(x11.XWin(f.client))

	extraWidth := 2 * borderWidth
	extraHeight := borderWidth + titleHeight
	// uint underflow
	if w > extraWidth {
		w -= extraWidth
	} else {
		w = 0
	}
	if h > extraHeight {
		h -= extraHeight
	} else {
		h = 0
	}

	adjustedW, adjustedH := windowSizeWithIncrement(f.client.wm.X(), f.client.win, w, h)
	constrainW, constrainH := windowSizeConstrain(f.client.wm.X(), f.client.win,
		adjustedW, adjustedH)
	f.width = constrainW + extraWidth
	f.height = constrainH + extraHeight

	return uint32(borderWidth), uint32(titleHeight), uint32(constrainW), uint32(constrainH)
}

func (f *frame) hide() {
	stack := f.client.wm.Windows()
	for i := 0; i < len(stack); i++ {
		if stack[i] == (interface{})(f.client).(tyde.Window) {
			continue
		}

		if stack[i].Iconic() {
			continue
		}

		stack[i].Focus()
		break
	}

	borderWidth := x11.BorderWidth(x11.XWin(f.client))
	titleHeight := x11.TitleHeight(x11.XWin(f.client))
	x, y := f.x+int16(borderWidth), f.y+int16(titleHeight)
	xproto.ReparentWindow(f.client.wm.Conn(), f.client.win, f.client.wm.X().RootWin(), x, y)
	xproto.UnmapWindow(f.client.wm.Conn(), f.client.win)
}

func (f *frame) maximizeApply() {
	// Per EWMH, _NET_WM_STATE_FULLSCREEN overrides WM_NORMAL_HINTS size limits,
	// so only honour the size-hint guards when the request is a maximize.
	if !f.client.Fullscreened() {
		if windowSizeFixed(f.client.wm.X(), f.client.win) ||
			!windowSizeCanMaximize(f.client.wm.X(), f.client) {
			return
		}
	}
	f.client.restoreWidth = f.width
	f.client.restoreHeight = f.height
	f.client.restoreX = f.x
	f.client.restoreY = f.y

	head := tyde.Instance().Screens().ScreenForWindow(f.client)
	maxX, maxY, maxWidth, maxHeight := tyde.Instance().ContentBoundsPixels(head)
	if f.client.Fullscreened() {
		maxX, maxY = 0, 0
		maxWidth = uint32(head.Width)
		maxHeight = uint32(head.Height)
	}
	f.updateGeometry(int16(head.X+int(maxX)), int16(head.Y+int(maxY)), uint16(maxWidth), uint16(maxHeight), true)
	f.notifyInnerGeometry()
	f.applyTheme(true)
}

func (f *frame) mouseDrag(x, y int16) {
	if f.client.Fullscreened() || f.ignoreDrag {
		return
	}
	moveDeltaX := x - f.mouseX
	moveDeltaY := y - f.mouseY
	if moveDeltaX == 0 && moveDeltaY == 0 {
		return
	}

	if f.client.Maximized() {
		screen := tyde.Instance().Screens().ScreenForWindow(f.client)
		scale := screen.CanvasScale()
		outsideBar := y > f.y+int16(x11.TitleHeight(x11.XWin(f.client))) || y < f.y

		if outsideBar && uint16(math.Abs(float64(moveDeltaY))) >
			uint16(math.Ceil(float64(unmaximizeThreshold)*float64(scale))) {
			diff := f.mouseX - f.x
			scale := float64(f.client.restoreWidth) / float64(f.width)

			f.client.restoreX = f.mouseX - int16(math.Ceil(float64(diff)*scale))
			f.client.restoreY = f.mouseY

			f.moveX = f.client.restoreX
			f.moveY = f.client.restoreY
			f.client.Unmaximize()
		}
		return
	}

	f.mouseX = x
	f.mouseY = y

	if f.moveOnly {
		f.moveX += moveDeltaX
		f.moveY += moveDeltaY
		// Fast path: update compositor visual directly, skip X11 and queueGeometry.
		// mouseDrag runs on the main thread (via fyne.Do) so this is safe.
		if x11.VisualMoveCallback != nil {
			f.x = f.moveX
			f.y = f.moveY
			x11.VisualMoveCallback(uint32(f.client.id), f.moveX, f.moveY, f.width, f.height)
		} else {
			f.queueGeometry(f.moveX, f.moveY, f.width, f.height, false)
		}
	}
	if f.resizeTop || f.resizeBottom || f.resizeLeft || f.resizeRight && !windowSizeFixed(f.client.wm.X(), f.client.win) {
		deltaX := x - f.resizeStartX
		deltaY := y - f.resizeStartY
		width := int16(f.resizeStartWidth)
		height := int16(f.resizeStartHeight)
		if f.resizeTop {
			f.moveY += moveDeltaY
			height -= deltaY
		} else if f.resizeBottom {
			height += deltaY
		}
		if f.resizeLeft {
			f.moveX += moveDeltaX
			width -= deltaX
		} else if f.resizeRight {
			width += deltaX
		}

		// avoid uint underflow
		if width < 1 {
			width = 1
		}
		if height < 1 {
			height = 1
		}
		f.queueGeometry(f.moveX, f.moveY, uint16(width), uint16(height), false)
	}
}

func (f *frame) mouseMotion(x, y int16) {
	relX := x - f.x
	relY := y - f.y

	if uint16(relX) > f.width-f.topRightPixelWidth() && f.canvas != nil {
		relX = int16(f.canvas.Content().Size().Width*f.canvas.Scale()) - (int16(f.width) - relX)
	}

	refresh := false
	obj := wm.FindObjectAtPixelPositionMatching(int(relX), int(relY), f.canvas,
		func(obj fyne.CanvasObject) bool {
			_, ok := obj.(desktop.Cursorable)
			if ok {
				return true
			}

			_, ok = obj.(desktop.Hoverable)
			return ok
		},
	)

	cursor := x11.DefaultCursor
	if obj != nil {
		if cur, ok := obj.(desktop.Cursorable); ok {
			if cur.Cursor() == desktop.PointerCursor {
				cursor = x11.CloseCursor
			}
		}
		if hov, ok := obj.(desktop.Hoverable); ok {
			if f.hovered == nil {
				hov.MouseIn(&desktop.MouseEvent{})
				f.hovered = hov
				refresh = true
			} else if obj.(desktop.Hoverable) != f.hovered {
				f.hovered.MouseOut()
				hov.MouseIn(&desktop.MouseEvent{})
				f.hovered = hov
				refresh = true
			} else {
				hov.MouseMoved(&desktop.MouseEvent{})
			}
		}
	} else if !f.client.Maximized() && !f.client.Fullscreened() && !windowSizeFixed(f.client.wm.X(), f.client.win) {
		cursor = f.lookupResizeCursor(relX, relY)
	}

	if obj == nil && f.hovered != nil {
		f.hovered.MouseOut()
		f.hovered = nil
		refresh = true
	}
	if refresh {
		f.applyTheme(true)
	}
	xproto.ChangeWindowAttributes(f.client.wm.Conn(), f.client.id, xproto.CwCursor,
		[]uint32{uint32(cursor)})
}

func (f *frame) lookupResizeCursor(x, y int16) xproto.Cursor {
	cornerSize := x11.ButtonWidth(x11.XWin(f.client))
	edgeSize := x11.BorderWidth(x11.XWin(f.client))

	if y < int16(x11.TitleHeight(x11.XWin(f.client))) { // top left or right
		if x < int16(edgeSize) {
			return x11.ResizeTopLeftCursor
		} else if x >= int16(f.width-edgeSize) {
			return x11.ResizeTopRightCursor
		}
	} else if y >= int16(f.height-cornerSize) { // bottom
		if x < int16(cornerSize) {
			return x11.ResizeBottomLeftCursor
		} else if x >= int16(f.width-cornerSize) {
			return x11.ResizeBottomRightCursor
		} else {
			return x11.ResizeBottomCursor
		}
	} else { // center (sides)
		if x < int16(cornerSize) {
			return x11.ResizeLeftCursor
		} else if x >= int16(f.width-cornerSize) {
			return x11.ResizeRightCursor
		}
	}

	return x11.DefaultCursor
}

func (f *frame) mousePress(x, y int16, b xproto.Button, mods uint16) {
	if b >= xproto.ButtonIndex4 && mods > 0 {
		f.transparency -= 5
		if b == xproto.ButtonIndex5 {
			f.transparency += 10
		}

		if f.transparency < 0 {
			f.transparency = 0
		} else if f.transparency >= 90 {
			f.transparency = 90
		}

		_ = ewmh.WmWindowOpacitySet(f.client.wm.X(), f.client.id, float64(100-f.transparency)/100.0)
		return
	}

	if b != xproto.ButtonIndex1 {
		return
	}
	if !f.client.Focused() {
		f.client.RaiseToTop()
		f.client.Focus()
		return
	}
	if f.client.Fullscreened() {
		return
	}

	relX := x - f.x
	relY := y - f.y

	titlebarX := relX
	if uint16(titlebarX) > f.width-f.topRightPixelWidth() && f.canvas != nil {
		titlebarX = int16(f.canvas.Content().Size().Width*f.canvas.Scale()) - (int16(f.width) - titlebarX)
	}
	obj := wm.FindObjectAtPixelPositionMatching(int(titlebarX), int(relY), f.canvas,
		func(obj fyne.CanvasObject) bool {
			_, ok := obj.(fyne.Tappable)
			return ok
		},
	)
	if _, ok := obj.(desktop.Cursorable); ok { // a button
		f.ignoreDrag = true
		return
	}

	buttonWidth := x11.ButtonWidth(x11.XWin(f.client))
	borderWidth := x11.BorderWidth(x11.XWin(f.client))
	titleHeight := x11.TitleHeight(x11.XWin(f.client))
	f.mouseX = x
	f.mouseY = y
	f.resizeStartX = x
	f.resizeStartY = y
	f.moveX = f.x
	f.moveY = f.y
	f.resizeStartWidth = f.width
	f.resizeStartHeight = f.height
	f.resizeBottom = false
	f.resizeLeft = false
	f.resizeRight = false
	f.resizeTop = false
	f.moveOnly = false

	if relY < int16(titleHeight) && relX >= int16(borderWidth) && relX < int16(f.width-borderWidth) {
		f.moveOnly = true
	} else if !windowSizeFixed(f.client.wm.X(), f.client.win) && !f.client.Maximized() {
		if relY < int16(titleHeight) {
			if relX < int16(borderWidth) {
				f.resizeLeft = true
				f.resizeTop = true
			} else if relX >= int16(f.width-borderWidth) {
				f.resizeRight = true
				f.resizeTop = true
			}
		} else {
			if relY >= int16(f.height-buttonWidth) {
				f.resizeBottom = true
			}
			if relX < int16(buttonWidth) {
				f.resizeLeft = true
			} else if relX >= int16(f.width-buttonWidth) {
				f.resizeRight = true
			}
		}
	}

	f.client.wm.RaiseToTop(f.client)
}

func (f *frame) mouseRelease(x, y int16, b xproto.Button) {
	f.ignoreDrag = false
	if b != xproto.ButtonIndex1 {
		return
	}
	titleHeight := x11.TitleHeight(x11.XWin(f.client))

	relX := x - f.x
	relY := y - f.y
	barYMax := int16(titleHeight)
	if relY > barYMax {
		return
	}
	f.clickCount++

	if f.cancelFunc != nil {
		f.cancelFunc()
		return
	}

	go func() {
		time.Sleep(time.Second / 2)
		f.decorate(true)
	}()
	go f.mouseReleaseWaitForDoubleClick(int(relX), int(relY))
}

func (f *frame) mouseReleaseWaitForDoubleClick(relX int, relY int) {
	var ctx context.Context
	ctx, f.cancelFunc = context.WithDeadline(context.TODO(), time.Now().Add(time.Millisecond*300))
	defer f.cancelFunc()

	if uint16(relX) > f.width-f.topRightPixelWidth() {
		relX = int(f.canvas.Content().Size().Width*f.canvas.Scale()) - (int(f.width) - relX)
	}

	<-ctx.Done()
	clickCount := f.clickCount
	f.clickCount = 0
	f.cancelFunc = nil

	fyne.Do(func() {
		if clickCount == 2 {
			obj := wm.FindObjectAtPixelPositionMatching(relX, relY, f.canvas,
				func(obj fyne.CanvasObject) bool {
					_, ok := obj.(fyne.DoubleTappable)
					return ok
				},
			)
			if obj != nil {
				obj.(fyne.DoubleTappable).DoubleTapped(&fyne.PointEvent{})
			}
		} else {
			obj := wm.FindObjectAtPixelPositionMatching(relX, relY, f.canvas,
				func(obj fyne.CanvasObject) bool {
					_, ok := obj.(fyne.Tappable)
					return ok
				},
			)
			if obj != nil {
				obj.(fyne.Tappable).Tapped(&fyne.PointEvent{})
			}
		}
	})
}

// Notify the child window that it's geometry has changed to update menu positions etc.
// This should be used sparingly as it can impact performance on the child window.
func (f *frame) notifyInnerGeometry() {
	innerX, innerY, innerW, innerH := f.getInnerWindowCoordinates(f.width, f.height)
	ev := xproto.ConfigureNotifyEvent{
		Event: f.client.win, Window: f.client.win, AboveSibling: 0,
		X: f.x + int16(innerX), Y: f.y + int16(innerY), Width: uint16(innerW), Height: uint16(innerH),
		BorderWidth: x11.BorderWidth(x11.XWin(f.client)), OverrideRedirect: false,
	}
	xproto.SendEvent(f.client.wm.Conn(), false, f.client.win, xproto.EventMaskStructureNotify, string(ev.Bytes()))
}

func (f *frame) removeBorder() {
	borderWidth := x11.BorderWidth(x11.XWin(f.client))
	titleHeight := x11.TitleHeight(x11.XWin(f.client))

	if !f.client.maximized {
		f.x += int16(borderWidth)
		f.y += int16(titleHeight)
		f.width = f.childWidth
		f.height = f.childHeight
	}
	f.applyTheme(true)

	xproto.ConfigureWindow(f.client.wm.Conn(), f.client.id, xproto.ConfigWindowX|xproto.ConfigWindowY|
		xproto.ConfigWindowWidth|xproto.ConfigWindowHeight,
		[]uint32{uint32(f.x), uint32(f.y), uint32(f.width), uint32(f.height)})
	xproto.ConfigureWindow(f.client.wm.Conn(), f.client.win, xproto.ConfigWindowX|xproto.ConfigWindowY|
		xproto.ConfigWindowWidth|xproto.ConfigWindowHeight,
		[]uint32{0, 0, uint32(f.childWidth), uint32(f.childHeight)})

	err := ewmh.FrameExtentsSet(f.client.wm.X(), f.client.win, &ewmh.FrameExtents{Left: 0, Right: 0, Top: 0, Bottom: 0})
	if err != nil {
		fyne.LogError("", err)
	}
	f.notifyInnerGeometry()
}

func (f *frame) show() {
	c := f.client
	xproto.MapWindow(c.wm.Conn(), c.id)

	xproto.ChangeSaveSet(c.wm.Conn(), xproto.SetModeInsert, c.win)
	xproto.MapWindow(c.wm.Conn(), c.win)
	xproto.GrabButton(f.client.wm.Conn(), true, f.client.id,
		xproto.EventMaskButtonPress, xproto.GrabModeSync, xproto.GrabModeSync,
		f.client.wm.X().RootWin(), xproto.CursorNone, xproto.ButtonIndex1, xproto.ModMaskAny)
	f.updateInputShape()

	userMod := uint16(xproto.ModMask4)
	if tyde.Instance().Settings().KeyboardModifier() == fyne.KeyModifierAlt {
		userMod = xproto.ModMask1
	}

	xproto.GrabButton(f.client.wm.Conn(), false, f.client.id,
		xproto.EventMaskButtonPress, xproto.GrabModeSync, xproto.GrabModeSync,
		0, xproto.CursorNone, xproto.ButtonIndex4, userMod)
	xproto.GrabButton(f.client.wm.Conn(), false, f.client.id,
		xproto.EventMaskButtonPress, xproto.GrabModeSync, xproto.GrabModeSync,
		0, xproto.CursorNone, xproto.ButtonIndex5, userMod)

	c.RaiseToTop()
	c.Focus()
}

func (f *frame) topRightPixelWidth() uint16 {
	screen := tyde.Instance().Screens().ScreenForWindow(f.client)
	scale := screen.CanvasScale()

	iconPix := uint16(0)
	if f.client.Properties().Icon() != nil {
		iconPix = x11.ButtonWidth(x11.XWin(f.client))
	}
	iconAndBorderPix := iconPix + x11.BorderWidth(x11.XWin(f.client))*2 + uint16(theme.Padding()*scale)
	if tyde.Instance().Settings().BorderButtonPosition() == "Right" {
		iconAndBorderPix = 3*iconAndBorderPix - uint16(theme.Padding()*scale)
	}

	return iconAndBorderPix - uint16(theme.Padding()*scale)
}

func (f *frame) unmaximizeApply() {
	// When leaving fullscreen, NotifyUnFullscreen calls this with c.full still
	// true so we can detect the transition and bypass the maximize guards.
	if !f.client.Fullscreened() {
		if windowSizeFixed(f.client.wm.X(), f.client.win) ||
			!windowSizeCanMaximize(f.client.wm.X(), f.client) {
			return
		}
	}
	if f.client.restoreWidth == 0 && f.client.restoreHeight == 0 {
		screen := tyde.Instance().Screens().ScreenForWindow(f.client)
		f.client.restoreWidth = uint16(screen.Width / 2)
		f.client.restoreHeight = uint16(screen.Height / 2)
	}
	f.updateGeometry(f.client.restoreX, f.client.restoreY, f.client.restoreWidth, f.client.restoreHeight, true)
	f.notifyInnerGeometry()
	f.applyTheme(true)
}

func (f *frame) queueGeometry(x int16, y int16, width uint16, height uint16, force bool) {
	f.pendingMu.Lock()
	if f.pendingGeometry == nil {
		f.pendingGeometry = make(chan *configureGeometry, 50)
		go f.configureLoop()
	}
	ch := f.pendingGeometry
	f.pendingMu.Unlock()
	ch <- &configureGeometry{x, y, width, height, force}
}

func (f *frame) updateGeometry(x, y int16, w, h uint16, force bool) {
	var move, resize bool
	if !force {
		resize = w != f.width || h != f.height
		move = x != f.x || y != f.y
		if !move && !resize {
			return
		}
	}

	currentScreen := tyde.Instance().Screens().ScreenForWindow(f.client)

	f.x = x
	f.y = y

	innerX, innerY, innerW, innerH := f.getInnerWindowCoordinates(w, h)

	f.childWidth = uint16(innerW)
	f.childHeight = uint16(innerH)

	// During drag (pendingGeometry active) and move-only (no resize),
	// skip expensive X11 ConfigureWindow and only update the compositor visual.
	// The actual X11 position is synced when the drag ends.
	if f.pendingGeometry != nil && move && !resize && x11.VisualMoveCallback != nil {
		x11.VisualMoveCallback(uint32(f.client.id), f.x, f.y, f.width, f.height)
		return
	}

	xproto.ConfigureWindow(f.client.wm.Conn(), f.client.id, xproto.ConfigWindowX|xproto.ConfigWindowY|
		xproto.ConfigWindowWidth|xproto.ConfigWindowHeight,
		[]uint32{uint32(f.x), uint32(f.y), uint32(f.width), uint32(f.height)})
	xproto.ConfigureWindow(f.client.wm.Conn(), f.client.win, xproto.ConfigWindowX|xproto.ConfigWindowY|
		xproto.ConfigWindowWidth|xproto.ConfigWindowHeight,
		[]uint32{innerX, innerY, uint32(f.childWidth), uint32(f.childHeight)})

	newScreen := tyde.Instance().Screens().ScreenForWindow(f.client)
	if newScreen != currentScreen {
		f.updateScale()
		tyde.Instance().Screens().SetActive(newScreen)
	}

	f.updateInputShape()
}

func (f *frame) updateScale() {
	// update border offset for current scale and redraw borders
	f.updateGeometry(f.x, f.y, f.width, f.height, true)
	f.applyTheme(true)
}

// updateInputShape sets the frame's X11 input shape to exclude desktop panel areas.
// This allows mouse events in panel regions to pass through to the Fyne root window.
func (f *frame) updateInputShape() {
	inst := tyde.Instance()
	if inst == nil {
		return
	}

	screen := inst.Screens().ScreenForWindow(f.client)
	if screen == nil || screen != inst.Screens().Primary() {
		// Only the primary screen has panels; other screens need no clipping
		return
	}

	cbX, cbY, cbW, cbH := inst.ContentBoundsPixels(screen)

	// Build rectangles that represent the frame area INSIDE the content bounds.
	// Any part of the frame outside the content bounds (i.e. under a panel) is excluded.
	var rects []xproto.Rectangle

	// Clip frame bounds to content bounds
	fx, fy := int32(f.x), int32(f.y)
	fw, fh := int32(f.width), int32(f.height)

	cx1, cy1 := int32(cbX)+int32(screen.X), int32(cbY)+int32(screen.Y)
	cx2, cy2 := cx1+int32(cbW), cy1+int32(cbH)

	// Intersection of frame and content area
	ix1 := max32(fx, cx1)
	iy1 := max32(fy, cy1)
	ix2 := min32(fx+fw, cx2)
	iy2 := min32(fy+fh, cy2)

	if ix1 < ix2 && iy1 < iy2 {
		// There is an intersection — set input shape to just that area (in frame-local coords)
		rects = append(rects, xproto.Rectangle{
			X:      int16(ix1 - fx),
			Y:      int16(iy1 - fy),
			Width:  uint16(ix2 - ix1),
			Height: uint16(iy2 - iy1),
		})
	}

	if len(rects) == 0 {
		// Frame is entirely under panels — make it fully input-transparent
		rects = append(rects, xproto.Rectangle{X: 0, Y: 0, Width: 0, Height: 0})
	}

	_ = shape.RectanglesChecked(f.client.wm.Conn(), shape.SoSet, shape.SkInput,
		0, f.client.id, 0, 0, rects).Check()
}

func max32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

func min32(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}
