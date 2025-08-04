package composit

// A simple compositor written in go, based on https://github.com/bvkgo/gcompositor
// which in turn is a Go re-write of https://github.com/jmanc3/xcompmgr-simple/.
// Many thanks to both!

import (
	"fmt"
	"log"
	"os"
	"slices"

	"fyne.io/fyne/v2"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/composite"
	"github.com/BurntSushi/xgb/damage"
	"github.com/BurntSushi/xgb/render"
	"github.com/BurntSushi/xgb/shape"
	"github.com/BurntSushi/xgb/xfixes"
	"github.com/BurntSushi/xgb/xproto"
)

type client struct {
	title           string
	win             xproto.Window
	damaged, shaped bool

	geom                         xproto.GetGeometryReply
	shapeBounds                  xproto.Rectangle
	borderExtents, extents, clip xfixes.Region

	buffer       xproto.Pixmap
	attributes   xproto.GetWindowAttributesReply
	damage       damage.Damage
	pixels, mask render.Picture
}

var (
	defaultScreen    int
	pictFormats      *render.QueryPictFormatsReply
	rootWindow       xproto.Window
	rootVisualFormat *render.Pictforminfo
	rootWidth        uint16
	rootHeight       uint16
	rootPicture      render.Picture
	rootBuffer       render.Picture
	rootTile         render.Picture
	allDamage        xfixes.Region
	clipChanged      bool
	clients          []*client

	netWmNameAtom  xproto.Atom
	utf8StringAtom xproto.Atom
	wmNameAtom     xproto.Atom
	stringAtom     xproto.Atom
)

type cookieReply[R any] interface {
	Reply() (R, error)
}

func initExtension[S any, T cookieReply[S]](conn *xgb.Conn, initFunc func(conn *xgb.Conn) error,
	verFunc func(*xgb.Conn, uint32, uint32) T, major, minor uint32) error {
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
	if err := initExtension(conn, render.Init, render.QueryVersion, 0, 11); err != nil {
		return err
	}
	if err := initExtension(conn, composite.Init, composite.QueryVersion, 0, 2); err != nil {
		return err
	}
	if err := initExtension(conn, damage.Init, damage.QueryVersion, 1, 1); err != nil {
		return err
	}
	if err := initExtension(conn, xfixes.Init, xfixes.QueryVersion, 5, 0); err != nil {
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
			log.Printf("Failed to redirect subwindows: %v", err)
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
		if err = shape.SelectInputChecked(conn, rootWindow, true).Check(); err != nil {
			return err
		}

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

	return paintAll(conn, 0)
}

func run(done chan struct{}) error {
	conn, err := xgb.NewConn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = setup(conn)
	if err != nil {
		return err
	}

	var exposeRects []xproto.Rectangle
	for {
		select {
		case <-done:
			return nil
		default:
			ev, err := conn.WaitForEvent()
			if err != nil {
				fyne.LogError("error waiting for event", err)
				continue
			}

			var repaint bool
			switch e := ev.(type) {
			case xproto.CreateNotifyEvent:
				if err := addClient(conn, e.Window); err != nil {
					fyne.LogError("failed to add client", err)
					repaint = true
				}
			case xproto.ConfigureNotifyEvent:
				if err := configureClient(conn, e); err != nil {
					fyne.LogError("failed to configure client", err)
					repaint = true
				}
			case xproto.DestroyNotifyEvent:
				destroyWin(conn, e.Window, true)
			case xproto.MapNotifyEvent:
				if err := mapWin(conn, e.Window); err != nil {
					fyne.LogError("failed to map window", err)
					repaint = true
				}
			case xproto.UnmapNotifyEvent:
				if err := unmapWin(conn, e.Window); err != nil {
					fyne.LogError("failed to unmap window", err)
					repaint = true
				}
			case xproto.ReparentNotifyEvent:
				if e.Parent == rootWindow {
					if err := addClient(conn, e.Window); err != nil {
						fyne.LogError("failed to add client", err)
						repaint = true
					}
				} else {
					destroyWin(conn, e.Window, false)
				}

			case xproto.CirculateNotifyEvent:
				circulateClient(e)
			case xproto.ExposeEvent:
				if e.Window == rootWindow {
					exposeRects = append(exposeRects, xproto.Rectangle{
						X:      int16(e.X),
						Y:      int16(e.Y),
						Width:  e.Width,
						Height: e.Height,
					})
					if e.Count == 0 {
						if err := exposeRoot(conn, exposeRects); err != nil {
							fyne.LogError("failed to expose root", err)
							repaint = true
						}
						exposeRects = nil
					}
				}
			case xproto.PropertyNotifyEvent:
				for _, prop := range []string{"_XROOTPMAP_ID", "_XSETROOT_ID"} {
					atom, err := xproto.InternAtom(conn, false, uint16(len(prop)), prop).Reply()
					if err == nil && e.Atom == atom.Atom {
						if rootTile != 0 {
							render.FreePicture(conn, rootTile)
							rootTile = 0
						}
						break
					}
				}

			case shape.NotifyEvent:
				if err := shapeWin(conn, &e); err != nil {
					fyne.LogError("failed to send shape notify", err)
					repaint = true
				}
			case *shape.NotifyEvent:
				if err := shapeWin(conn, e); err != nil {
					fyne.LogError("failed to send shape notify", err)
					repaint = true
				}
			case damage.NotifyEvent:
				if err := damageClient(conn, &e); err != nil {
					fyne.LogError("failed to send damage notify", err)
					repaint = true
				}
			}

			if allDamage != 0 {
				if err := paintAll(conn, allDamage); err != nil {
					fyne.LogError("failed to paint all damage", err)
					repaint = true
				}
			}

			if repaint {
				if err := paintAll(conn, 0); err != nil {
					fyne.LogError("failed to paint all damage", err)
				}
			}

			conn.Sync()

			allDamage = 0
			clipChanged = false
		}
	}
}

func findPictFormat(formats *render.QueryPictFormatsReply, id render.Pictformat) *render.Pictforminfo {
	for _, f := range formats.Formats {
		if f.Id == id {
			return &f
		}
	}
	return nil
}

func registerManager(conn *xgb.Conn, screen int) error {
	atomName := fmt.Sprintf("_NET_WM_CM_S%d", screen)
	atom, err := xproto.InternAtom(conn, false, uint16(len(atomName)), atomName).Reply()
	if err != nil {
		log.Printf("Failed to intern %s: %v", atomName, err)
		return err
	}
	owner, err := xproto.GetSelectionOwner(conn, atom.Atom).Reply()
	if err != nil {
		log.Printf("could not query current compositor: %v", err)
		return err
	}
	if owner.Owner != 0 {
		log.Printf("Another composite manager is running")
		return fmt.Errorf("another composite manager is running: %w", os.ErrExist)
	}
	win, err := xproto.NewWindowId(conn)
	if err != nil {
		log.Printf("Failed to create window: %v", err)
		return err
	}
	err = xproto.CreateWindowChecked(conn, xproto.WindowClassCopyFromParent, win, rootWindow,
		0, 0, 1, 1, 0, xproto.WindowClassInputOutput, 0, 0, nil).Check()
	if err != nil {
		log.Printf("Failed to create window: %v", err)
		return err
	}
	err = xproto.SetSelectionOwnerChecked(conn, win, atom.Atom, xproto.TimeCurrentTime).Check()
	if err != nil {
		log.Printf("Failed to set selection owner: %v", err)
		return err
	}
	return nil
}

func setupRoot(conn *xgb.Conn) (err error) {
	screen := xproto.Setup(conn).DefaultScreen(conn)
	defaultScreen = conn.DefaultScreen
	rootWindow = screen.Root
	rootWidth = screen.WidthInPixels
	rootHeight = screen.HeightInPixels

	rootVisual := screen.RootVisual
	pictFormats, err = render.QueryPictFormats(conn).Reply()
	if err != nil {
		return err
	}
	var visualFormat *render.Pictforminfo
	for _, v := range pictFormats.Screens[defaultScreen].Depths {
		for _, f := range v.Visuals {
			if f.Visual == rootVisual {
				visualFormat = findPictFormat(pictFormats, f.Format)
				break
			}
		}
	}
	if visualFormat == nil {
		return fmt.Errorf("could not find visual format for root window")
	}
	rootVisualFormat = visualFormat

	rootPicture, err = render.NewPictureId(conn)
	if err != nil {
		return err
	}
	err = render.CreatePictureChecked(conn, rootPicture, xproto.Drawable(rootWindow), rootVisualFormat.Id, render.CpSubwindowMode, []uint32{xproto.SubwindowModeIncludeInferiors}).Check()
	if err != nil {
		return err
	}

	return nil
}

func createRootTile(conn *xgb.Conn) (render.Picture, error) {
	pixmap, err := xproto.NewPixmapId(conn)
	if err != nil {
		return render.PictureNone, err
	}
	err = xproto.CreatePixmapChecked(conn, xproto.Setup(conn).DefaultScreen(conn).RootDepth, pixmap,
		xproto.Drawable(rootWindow), 1, 1).Check()
	if err != nil {
		return render.PictureNone, err
	}
	picture, err := render.NewPictureId(conn)
	if err != nil {
		return render.PictureNone, err
	}
	err = render.CreatePictureChecked(conn, picture, xproto.Drawable(pixmap), rootVisualFormat.Id, render.CpRepeat, []uint32{1}).Check()
	if err != nil {
		return render.PictureNone, err
	}
	color := render.Color{Red: 0x8080, Green: 0x8080, Blue: 0x8080, Alpha: 0xffff}
	err = render.FillRectanglesChecked(conn, render.PictOpSrc, picture, color, []xproto.Rectangle{{Width: 1, Height: 1}}).Check()
	if err != nil {
		return render.PictureNone, err
	}
	return picture, nil
}

func paintRoot(conn *xgb.Conn) error {
	if rootTile == render.PictureNone {
		tile, err := createRootTile(conn)
		if err != nil {
			return err
		}
		rootTile = tile
	}

	err := render.CompositeChecked(conn, render.PictOpSrc, rootTile, 0, rootBuffer,
		0, 0, 0, 0, 0, 0, rootWidth, rootHeight).Check()
	if err != nil {
		return err
	}
	return nil
}

func getClientArea(conn *xgb.Conn, client *client) (xfixes.Region, error) {
	rect := xproto.Rectangle{
		X:      client.geom.X,
		Y:      client.geom.Y,
		Width:  client.geom.Width + client.geom.BorderWidth*2,
		Height: client.geom.Height + client.geom.BorderWidth*2,
	}
	region, err := xfixes.NewRegionId(conn)
	if err != nil {
		return 0, err
	}
	if err = xfixes.CreateRegionChecked(conn, region, []xproto.Rectangle{rect}).Check(); err != nil {
		return 0, err
	}
	return region, nil
}

func getBorderArea(conn *xgb.Conn, client *client) (xfixes.Region, error) {
	region, err := xfixes.NewRegionId(conn)
	if err != nil {
		return 0, err
	}
	err = xfixes.CreateRegionFromWindowChecked(conn, region, client.win, shape.SkBounding).Check()
	if err != nil {
		return 0, err
	}
	dx := client.geom.X + int16(client.geom.BorderWidth)
	dy := client.geom.Y + int16(client.geom.BorderWidth)
	err = xfixes.TranslateRegionChecked(conn, region, dx, dy).Check()
	if err != nil {
		return 0, err
	}
	return region, nil
}

func paintAll(conn *xgb.Conn, region xfixes.Region) error {
	if region == 0 {
		rect := xproto.Rectangle{X: 0, Y: 0, Width: rootWidth, Height: rootHeight}
		r, err := xfixes.NewRegionId(conn)
		if err != nil {
			return err
		}
		if err = xfixes.CreateRegionChecked(conn, r, []xproto.Rectangle{rect}).Check(); err != nil {
			return err
		}
		region = r
	}
	defer func() {
		_ = xfixes.DestroyRegionChecked(conn, region).Check()
	}()

	if rootBuffer == 0 {
		pixmap, err := xproto.NewPixmapId(conn)
		if err != nil {
			return err
		}
		err = xproto.CreatePixmapChecked(conn, xproto.Setup(conn).DefaultScreen(conn).RootDepth, pixmap,
			xproto.Drawable(rootWindow), rootWidth, rootHeight).Check()
		if err != nil {
			return err
		}
		rootBuffer, err = render.NewPictureId(conn)
		if err != nil {
			return err
		}
		if err = render.CreatePictureChecked(conn, rootBuffer, xproto.Drawable(pixmap), rootVisualFormat.Id, 0, nil).Check(); err != nil {
			return err
		}
		xproto.FreePixmap(conn, pixmap)
	}

	err := xfixes.SetPictureClipRegionChecked(conn, rootPicture, region, 0, 0).Check()
	if err != nil {
		return err
	}

	for _, c := range clients {
		if !c.damaged {
			continue
		}
		if c.geom.X+int16(c.geom.Width) < 1 || c.geom.Y+int16(c.geom.Height) < 1 ||
			c.geom.X >= int16(rootWidth) || c.geom.Y >= int16(rootHeight) {
			continue
		}

		if c.pixels == 0 {
			if c.buffer == 0 {
				pixmap, err := xproto.NewPixmapId(conn)
				if err != nil {
					return err
				}
				if err = composite.NameWindowPixmapChecked(conn, c.win, pixmap).Check(); err != nil {
					return err
				}
				c.buffer = pixmap
			}

			var visualFormat render.Pictformat
			for _, v := range pictFormats.Screens[defaultScreen].Depths {
				for _, f := range v.Visuals {
					if f.Visual == c.attributes.Visual {
						visualFormat = f.Format
						break
					}
				}
			}
			c.pixels, err = render.NewPictureId(conn)
			if err != nil {
				fyne.LogError("Error creating pictureId", err)
				continue
			}
			err = render.CreatePictureChecked(conn, c.pixels, xproto.Drawable(c.buffer), visualFormat,
				render.CpSubwindowMode, []uint32{xproto.SubwindowModeIncludeInferiors}).Check()
			if err != nil {
				fyne.LogError("Error create=ing picture", err)
				return err
			}
		}

		if clipChanged {
			if c.borderExtents != 0 {
				_ = xfixes.DestroyRegion(conn, c.borderExtents)
				c.borderExtents = 0
			}
			if c.extents != 0 {
				_ = xfixes.DestroyRegion(conn, c.extents)
				c.extents = 0
			}
			if c.clip != 0 {
				xfixes.DestroyRegion(conn, c.clip)
				c.clip = 0
			}
		}
		if c.borderExtents == 0 {
			area, err := getBorderArea(conn, c)
			if err != nil {
				return err
			}
			c.borderExtents = area
		}
		if c.extents == 0 {
			extents, err := getClientArea(conn, c)
			if err != nil {
				return err
			}
			c.extents = extents
		}

		x, y := c.geom.X, c.geom.Y
		w, h := c.geom.Width+c.geom.BorderWidth*2, c.geom.Height+c.geom.BorderWidth*2
		if err = xfixes.SetPictureClipRegionChecked(conn, rootBuffer, region, 0, 0).Check(); err != nil {
			return err
		}
		err = xfixes.SubtractRegionChecked(conn, region, c.borderExtents, region).Check()
		if err != nil {
			return err
		}
		err = render.CompositeChecked(conn, render.PictOpSrc, c.pixels, 0, rootBuffer,
			0, 0, 0, 0, x, y, w, h).Check()
		if err != nil {
			return err
		}

		if c.clip == 0 {
			c.clip, err = xfixes.NewRegionId(conn)
			if err != nil {
				return err
			}
			err = xfixes.CreateRegionChecked(conn, c.clip, nil).Check()
			if err != nil {
				return err
			}
			err = xfixes.CopyRegionChecked(conn, region, c.clip).Check()
			if err != nil {
				return err
			}
		}
	}

	err = xfixes.SetPictureClipRegionChecked(conn, rootBuffer, region, 0, 0).Check()
	if err != nil {
		return err
	}

	if err = paintRoot(conn); err != nil {
		return err
	}
	for i := len(clients) - 1; i >= 0; i-- {
		c := clients[i]
		if !c.damaged {
			continue
		}
		if c.geom.X+int16(c.geom.Width) < 1 || c.geom.Y+int16(c.geom.Height) < 1 ||
			c.geom.X >= int16(rootWidth) || c.geom.Y >= int16(rootHeight) {
			continue
		}

		err = xfixes.SetPictureClipRegionChecked(conn, rootBuffer, c.clip, 0, 0).Check()
		if err != nil {
			return err
		}

		if c.clip != 0 {
			_ = xfixes.DestroyRegion(conn, c.clip)
			c.clip = 0
		}
	}

	if rootBuffer != rootPicture {
		err = xfixes.SetPictureClipRegionChecked(conn, rootBuffer, 0, 0, 0).Check()
		if err != nil {
			return err
		}
		err = render.CompositeChecked(conn, render.PictOpSrc, rootBuffer, 0, rootPicture,
			0, 0, 0, 0, 0, 0, rootWidth, rootHeight).Check()
		if err != nil {
			return err
		}
	}
	return nil
}

func addDamage(conn *xgb.Conn, damage xfixes.Region) error {
	if allDamage == 0 {
		allDamage = damage
		return nil
	}
	err := xfixes.UnionRegionChecked(conn, allDamage, damage, allDamage).Check()
	if err != nil {
		return err
	}
	if err = xfixes.DestroyRegionChecked(conn, damage).Check(); err != nil {
		return err
	}
	return nil
}

func finishUnmapClient(conn *xgb.Conn, client *client) error {
	client.damaged = false
	if client.extents != 0 {
		if err := addDamage(conn, client.extents); err != nil {
			fyne.LogError("Failed to add damage for "+client.title, err)
		}
		client.extents = 0
	}
	if client.buffer != 0 {
		xproto.FreePixmap(conn, client.buffer)
		client.buffer = 0
	}
	if client.pixels != 0 {
		render.FreePicture(conn, client.pixels)
		client.pixels = 0
	}
	err := xproto.ChangeWindowAttributesChecked(conn, client.win, xproto.CwEventMask, []uint32{0}).Check()
	if err != nil {
		fyne.LogError("Error clearing event mask for"+client.title, err)
	}
	if client.borderExtents != 0 {
		_ = xfixes.DestroyRegion(conn, client.borderExtents)
		client.borderExtents = 0
	}
	if client.clip != 0 {
		_ = xfixes.DestroyRegion(conn, client.clip)
		client.clip = 0
	}
	clipChanged = true
	return nil
}

func getClientFromWindow(window xproto.Window) *client {
	i := slices.IndexFunc(clients, func(c *client) bool {
		return c.win == window
	})
	if i == -1 {
		return nil
	}
	return clients[i]
}

func unmapWin(conn *xgb.Conn, window xproto.Window) error {
	c := getClientFromWindow(window)
	if c == nil {
		return nil // gone?
	}
	c.attributes.MapState = xproto.MapStateUnmapped
	_ = finishUnmapClient(conn, c)
	return nil
}

func mapWin(conn *xgb.Conn, window xproto.Window) error {
	c := getClientFromWindow(window)
	if c == nil {
		return fmt.Errorf("could not get client for window %x", window)
	}

	mask := []uint32{xproto.EventMaskPropertyChange}
	_ = xproto.ChangeWindowAttributes(conn, window, xproto.CwEventMask, mask)

	c.attributes.MapState = xproto.MapStateViewable
	c.damaged = false
	return nil
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
		title:       name,
		win:         window,
		attributes:  *attr,
		geom:        *geom,
		shaped:      false,
		shapeBounds: xproto.Rectangle{X: geom.X, Y: geom.Y, Width: geom.Width, Height: geom.Height},
		damaged:     false,
	}

	if attr.Class != xproto.WindowClassInputOnly {
		c.damage, err = damage.NewDamageId(conn)
		if err != nil {
			return err
		}
		if err = damage.CreateChecked(conn, c.damage, xproto.Drawable(window), damage.ReportLevelNonEmpty).Check(); err != nil {
			return err
		}
		if err = shape.SelectInputChecked(conn, window, true).Check(); err != nil {
			return err
		}
	}

	clients = append([]*client{c}, clients...)
	if c.attributes.MapState == xproto.MapStateViewable {
		if err = mapWin(conn, window); err != nil {
			return err
		}
	}
	return nil
}

func restackWin(window, target xproto.Window) {
	i := slices.IndexFunc(clients, func(c *client) bool { return c.win == window })
	if i == -1 {
		return
	}
	c := clients[i]
	clients = slices.Delete(clients, i, i+1)

	if target == 0 {
		clients = append(clients, c)
		return
	}

	j := slices.IndexFunc(clients, func(c *client) bool { return c.win == target })
	if j == -1 {
		clients = append(clients, c)
		return
	}

	clients = slices.Insert(clients, j, c)
}

func configureClient(conn *xgb.Conn, e xproto.ConfigureNotifyEvent) error {
	client := getClientFromWindow(e.Window)
	if client == nil {
		if e.Window == rootWindow {
			if rootBuffer != 0 {
				render.FreePicture(conn, rootBuffer)
				rootBuffer = 0
			}
			rootWidth = e.Width
			rootHeight = e.Height
		}
		return nil
	}

	dam, err := xfixes.NewRegionId(conn)
	if err != nil {
		return err
	}
	err = xfixes.CreateRegionChecked(conn, dam, nil).Check()
	if err != nil {
		return err
	}
	if client.extents != 0 {
		if err = xfixes.CopyRegionChecked(conn, client.extents, dam).Check(); err != nil {
			return err
		}
	}
	client.shapeBounds.X -= client.geom.X
	client.shapeBounds.Y -= client.geom.Y
	client.geom.X = e.X
	client.geom.Y = e.Y
	if client.geom.Width != e.Width || client.geom.Height != e.Height {
		if client.buffer != 0 {
			xproto.FreePixmap(conn, client.buffer)
			client.buffer = 0
			if client.pixels != 0 {
				render.FreePicture(conn, client.pixels)
				client.pixels = 0
			}
		}
	}
	client.geom.Width = e.Width
	client.geom.Height = e.Height
	client.geom.BorderWidth = e.BorderWidth
	client.attributes.OverrideRedirect = e.OverrideRedirect

	restackWin(e.Window, e.AboveSibling)

	extents, err := getClientArea(conn, client)
	if err != nil {
		return err
	}
	if err = xfixes.UnionRegionChecked(conn, dam, extents, dam).Check(); err != nil {
		return err
	}
	if err = xfixes.DestroyRegionChecked(conn, extents).Check(); err != nil {
		return err
	}
	if err = addDamage(conn, dam); err != nil {
		return err
	}
	client.shapeBounds.X += client.geom.X
	client.shapeBounds.Y += client.geom.Y
	if !client.shaped {
		client.shapeBounds.Width = client.geom.Width
		client.shapeBounds.Height = client.geom.Height
	}
	clipChanged = true
	return nil
}

func circulateClient(e xproto.CirculateNotifyEvent) {
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
	restackWin(client.win, target)
	clipChanged = true
}

func destroyWin(conn *xgb.Conn, window xproto.Window, gone bool) {
	i := slices.IndexFunc(clients, func(c *client) bool {
		return c.win == window
	})
	if i == -1 {
		return
	}

	client := clients[i]
	if gone {
		_ = finishUnmapClient(conn, client)
	}

	if client.pixels != 0 {
		render.FreePicture(conn, client.pixels)
		client.pixels = 0
	}
	if client.mask != 0 {
		render.FreePicture(conn, client.mask)
		client.mask = 0
	}
	if client.damage != 0 {
		_ = damage.Destroy(conn, client.damage).Check()
		client.damage = 0
	}

	clients = slices.Delete(clients, i, i+1)
	return
}

func damageClient(conn *xgb.Conn, e *damage.NotifyEvent) error {
	client := getClientFromWindow(xproto.Window(e.Drawable))
	if client == nil {
		return nil
	}

	var parts xfixes.Region
	if !client.damaged {
		ps, err := getClientArea(conn, client)
		if err != nil {
			return err
		}
		parts = ps
		if err = damage.SubtractChecked(conn, client.damage, xfixes.RegionNone, xfixes.RegionNone).Check(); err != nil {
			return err
		}
	} else {
		ps, err := xfixes.NewRegionId(conn)
		if err != nil {
			return err
		}
		parts = ps
		if err = xfixes.CreateRegionChecked(conn, parts, nil).Check(); err != nil {
			return err
		}
		if err = damage.SubtractChecked(conn, client.damage, xfixes.RegionNone, parts).Check(); err != nil {
			return err
		}
		dx := client.geom.X + int16(client.geom.BorderWidth)
		dy := client.geom.Y + int16(client.geom.BorderWidth)
		if err = xfixes.TranslateRegionChecked(conn, parts, dx, dy).Check(); err != nil {
			return err
		}
	}
	if err := addDamage(conn, parts); err != nil {
		return err
	}
	client.damaged = true
	return nil
}

func shapeWin(conn *xgb.Conn, e *shape.NotifyEvent) error {
	client := getClientFromWindow(e.AffectedWindow)
	if client == nil {
		return nil
	}
	if e.ShapeKind == shape.SkBounding || e.ShapeKind == shape.SkClip {
		clipChanged = true
		region0, err := xfixes.NewRegionId(conn)
		if err != nil {
			return err
		}
		err = xfixes.CreateRegionChecked(conn, region0, []xproto.Rectangle{client.shapeBounds}).Check()
		if err != nil {
			return err
		}
		if e.Shaped {
			client.shaped = true
			client.shapeBounds.X = client.geom.X + e.ExtentsX
			client.shapeBounds.Y = client.geom.Y + e.ExtentsY
			client.shapeBounds.Width = e.ExtentsWidth
			client.shapeBounds.Height = e.ExtentsHeight
		} else {
			client.shaped = false
			client.shapeBounds.X = client.geom.X
			client.shapeBounds.Y = client.geom.Y
			client.shapeBounds.Width = client.geom.Width
			client.shapeBounds.Height = client.geom.Height
		}
		region1, err := xfixes.NewRegionId(conn)
		if err != nil {
			return err
		}
		err = xfixes.CreateRegionChecked(conn, region1, []xproto.Rectangle{client.shapeBounds}).Check()
		if err != nil {
			return err
		}
		err = xfixes.UnionRegionChecked(conn, region0, region1, region0).Check()
		if err != nil {
			return err
		}
		xfixes.DestroyRegion(conn, region1)
		if err = paintAll(conn, region0); err != nil {
			return err
		}
	}
	return nil
}

func exposeRoot(conn *xgb.Conn, rects []xproto.Rectangle) error {
	region, err := xfixes.NewRegionId(conn)
	if err != nil {
		return err
	}
	err = xfixes.CreateRegionChecked(conn, region, rects).Check()
	if err != nil {
		return err
	}
	return addDamage(conn, region)
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
