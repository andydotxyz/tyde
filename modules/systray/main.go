//Note that you need to have github.com/knightpp/dbus-codegen-go installed from "custom" branch
//go:generate dbus-codegen-go -prefix org.kde -package notifier -output generated/notifier/status_notifier_item.go StatusNotifierItem.xml
//go:generate dbus-codegen-go -prefix org.kde -package watcher -output generated/watcher/status_notifier_watcher.go StatusNotifierWatcher.xml
//go:generate dbus-codegen-go -prefix com.canonical -package menu -output generated/menu/dbus_menu.go DbusMenu.xml

package systray

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/FyshOS/appie"
	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"

	"fyshos.com/tyde"
	"fyshos.com/tyde/modules/systray/generated/menu"
	"fyshos.com/tyde/modules/systray/generated/notifier"
	"fyshos.com/tyde/modules/systray/generated/watcher"
	wmtheme "fyshos.com/tyde/theme"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	deskDriver "fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	path     = "/StatusNotifierWatcher"
	hostPath = "/StatusNotifierHost"
)

var resourceID = 0

func init() {
	tyde.RegisterModule(trayMeta)
}

var trayMeta = tyde.ModuleMetadata{
	Name:        "SystemTray",
	NewInstance: NewTray,
}

type tray struct {
	conn *dbus.Conn
	menu *menu.Dbusmenu

	box   *fyne.Container
	lock  sync.Mutex
	nodes map[dbus.Sender]*node
}

type node struct {
	ico *multiButton
	ni  *notifier.StatusNotifierItem
	pid uint32
}

// NewTray creates a new module that will show a system tray in the status area
func NewTray() tyde.Module {
	iconSize := wmtheme.NarrowBarWidth
	grid := container.New(collapsingGridWrap(fyne.NewSize(iconSize, iconSize)))
	t := &tray{box: grid, nodes: make(map[dbus.Sender]*node)}

	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		fyne.LogError("Could not connect to DBus for system tray events", err)
		return t
	}
	t.conn = conn

	err = conn.ExportAll(struct{}{}, hostPath, "org.kde.StatusNotifierHost")
	if err != nil {
		fyne.LogError("Failed to export notifier host", err)
		return t
	}

	err = conn.ExportAll(t, path, "org.kde.StatusNotifierWatcher")
	if err != nil {
		fyne.LogError("Unable to register watcher", err)
		return t
	}

	_, err = conn.RequestName("org.kde.StatusNotifierWatcher", dbus.NameFlagDoNotQueue)
	if err != nil {
		log.Println("Failed to claim notifier watcher name", err)
		return t
	}

	_, err = prop.Export(conn, path, createPropSpec())
	if err != nil {
		log.Printf("Failed to export notifier item properties to bus")
		return t
	}

	node := introspect.Node{
		Name: path,
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			prop.IntrospectData,
			watcher.IntrospectDataStatusNotifierWatcher,
		},
	}
	err = conn.Export(introspect.NewIntrospectable(&node), path,
		"org.freedesktop.DBus.Introspectable")
	if err != nil {
		log.Printf("Failed to export introspection %v", err)
		return t
	}

	hostErr := t.RegisterStatusNotifierHost(conn.Names()[0])
	if hostErr != nil {
		fyne.LogError("Failed to register our systray host, another may already be running", hostErr)
	}

	watchErr := t.conn.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.DBus"), dbus.WithMatchObjectPath("/org/freedesktop/DBus"))
	_ = t.conn.AddMatchSignal(dbus.WithMatchInterface("org.kde.StatusNotifierItem"))
	if watchErr != nil {
		fyne.LogError("Failed to monitor systray name loss", watchErr)
	}

	c := make(chan *dbus.Signal, 10)
	t.conn.Signal(c)
	go func() {
		for v := range c {
			switch v.Name {
			case "org.freedesktop.DBus.NameOwnerChanged":
				name := v.Body[0]
				newOwner := v.Body[2]
				if newOwner == "" {
					t.removeNode(dbus.Sender(name.(string)))
				}
			case "org.kde.StatusNotifierItem.NewIcon":
				t.lock.Lock()
				item, ok := t.nodes[dbus.Sender(v.Sender)]
				t.lock.Unlock()
				if ok {
					icon := t.fetchIcon(item)
					fyne.Do(func() {
						item.ico.SetIcon(icon)
					})
				}
			default:
				log.Println("Also", v.Name)
				continue
			}
		}
	}()

	go t.monitorProcesses()

	return t
}

func (t *tray) Destroy() {
}

// removeNode drops a tray icon for the given sender, both from the visible
// tray and from our internal tracking map. Safe to call for unknown senders.
func (t *tray) removeNode(sender dbus.Sender) {
	t.lock.Lock()
	item, ok := t.nodes[sender]
	if ok {
		delete(t.nodes, sender)
	}
	t.lock.Unlock()
	if !ok {
		return
	}

	fyne.Do(func() {
		t.box.Remove(item.ico)
		t.box.Refresh()
	})
}

// monitorProcesses watches the processes backing each tray icon.
// We periodically confirm each backing process is still
// alive and remove the icon for any that have gone away.
func (t *tray) monitorProcesses() {
	for range time.Tick(time.Second * 5) {
		var dead []dbus.Sender
		t.lock.Lock()
		for sender, item := range t.nodes {
			if item.pid != 0 && !processAlive(item.pid) {
				dead = append(dead, sender)
			}
		}
		t.lock.Unlock()

		for _, sender := range dead {
			t.removeNode(sender)
		}
	}
}

// processID asks the bus for the process id behind a connection so we can keep
// an eye on it. It returns 0 if the owner could not be resolved.
func (t *tray) processID(sender dbus.Sender) uint32 {
	var pid uint32
	err := t.conn.BusObject().Call("org.freedesktop.DBus.GetConnectionUnixProcessID", 0,
		string(sender)).Store(&pid)
	if err != nil {
		return 0
	}
	return pid
}

// processAlive reports whether the given process id is still running.
// The process is considered gone if /proc has no entry for it, or if
// that entry reports the zombie ("Z") state.
func processAlive(pid uint32) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false // no /proc entry: the process is gone
	}

	if i := bytes.LastIndexByte(data, ')'); i >= 0 && i+2 < len(data) {
		return data[i+2] != 'Z'
	}
	return true
}

func (t *tray) RegisterStatusNotifierItem(service string, sender dbus.Sender) error {
	ni := notifier.NewStatusNotifierItem(t.conn.Object(string(sender), dbus.ObjectPath(service)))

	t.lock.Lock()
	item, ok := t.nodes[sender]
	if !ok {
		var ico *multiButton
		ico = newMultiButton(func() {
			_ = ni.Activate(t.conn.Context(), 5, 5)
		}, func() {
			if m, err := ni.GetMenu(t.conn.Context()); err == nil {
				t.showMenu(string(sender), m, ico)
				return
			}

			// try secondary if primary not known
			_ = ni.ContextMenu(t.conn.Context(), 5, 5)
		})
		ico.scroll = func(delta float32, horizontal bool) {
			if horizontal {
				_ = ni.Scroll(t.conn.Context(), int32(delta), "horizontal")
			} else {
				_ = ni.Scroll(t.conn.Context(), int32(delta), "vertical")
			}
		}

		ico.Importance = widget.LowImportance
		item = &node{ico: ico, ni: ni, pid: t.processID(sender)}
		t.nodes[sender] = item
		fyne.Do(func() {
			t.box.Add(ico)
		})
	}

	item.ni = ni
	t.lock.Unlock()

	icon := t.fetchIcon(item)
	fyne.Do(func() {
		item.ico.SetIcon(icon)
		t.box.Refresh()
	})

	return nil
}

func (t *tray) RegisterStatusNotifierHost(service string) error {
	return watcher.Emit(t.conn, &watcher.StatusNotifierWatcher_StatusNotifierHostRegisteredSignal{
		Path: dbus.ObjectPath(service),
		Body: &watcher.StatusNotifierWatcher_StatusNotifierHostRegisteredSignalBody{},
	})
}

func (t *tray) Metadata() tyde.ModuleMetadata {
	return trayMeta
}

func (t *tray) StatusAreaWidget() fyne.CanvasObject {
	return t.box
}

func (t *tray) parseMenu(parent int32, pos *fyne.Position, closer func()) fyne.CanvasObject {
	Y := pos.Y
	var items []*fyne.MenuItem
	_, l, _ := t.menu.GetLayout(t.conn.Context(), parent, 1, nil)
	for i, item := range l.V2 {
		data := item.Value().([]interface{})
		items = append(items, t.parseMenuItem(data[0].(int32), t.menu, data[1], pos, i, closer))

		Y += theme.TextSize() + theme.Padding()*2
	}
	m := fyne.NewMenu("", items...)
	return widget.NewMenu(m)
}

func (t *tray) parseMenuItem(id int32, menu *menu.Dbusmenu, in interface{}, pos *fyne.Position, off int, closer func()) *fyne.MenuItem {
	data := in.(map[string]dbus.Variant)
	ret := &fyne.MenuItem{}
	if ty, ok := data["type"]; ok {
		if ty.String() == "\"separator\"" {
			ret.IsSeparator = true
		}
	} else {
		ret.Label = fmt.Sprintf("%s", data["label"].Value())
		if checkType, ok := data["toggle-type"]; ok && checkType.Value() == "checkmark" {
			if checkState, ok := data["toggle-state"]; ok && checkState.Value().(int32) > 0 {
				ret.Checked = true
			}
		}
		ret.Action = func() {
			err := menu.Event(t.conn.Context(), id, "clicked", dbus.MakeVariant(id), uint32(time.Now().Unix()))
			if err != nil {
				fyne.LogError("Failed to message menu tap", err)
			}
			closer()
		}

		if s, ok := data["shortcut"]; ok {
			if short := parseShortcut(ret.Label, s); short != nil {
				ret.Shortcut = short
			}
		}
	}

	if i, ok := data["icon-data"]; ok {
		ret.Icon = fyne.NewStaticResource(fmt.Sprintf("systray-icon-%d", id), i.Value().([]byte))
	}
	if e, ok := data["enabled"]; ok && e.Value() == false {
		ret.Disabled = true
	}

	if t, ok := data["toggle-type"]; ok && t.String() == "\"checkmark\"" {
		if s, ok := data["toggle-state"]; ok && s.Value() == true {
			ret.Checked = true
		}
	}

	if s, ok := data["children-display"]; ok && s.String() == "\"submenu\"" {
		ret.Action = func() {
			w := fyne.CurrentApp().Driver().(deskDriver.Driver).CreateSplashWindow()
			w.SetOnClosed(closer)
			childPos := &fyne.Position{}

			w.SetContent(t.parseMenu(id, childPos, func() {
				w.Close()
				closer()
			}))

			size := w.Content().MinSize()
			w.Resize(size)
			sub := (*pos).AddXY(-size.Width, float32(off)*(18+theme.Padding()*4))
			screen := tyde.Instance().Screens().Primary()
			if sub.Y+size.Height > float32(screen.Height)/screen.CanvasScale() {
				sub.Y = float32(screen.Height)/screen.CanvasScale() - size.Height
			}
			childPos.X, childPos.Y = sub.X, sub.Y

			tyde.Instance().WindowManager().ShowOverlay(w, size, *childPos)
		}

		ret.ChildMenu = fyne.NewMenu("")
	}
	return ret
}

func (t *tray) showMenu(sender string, name dbus.ObjectPath, from fyne.CanvasObject) {
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(from)
	w := fyne.CurrentApp().Driver().(deskDriver.Driver).CreateSplashWindow()
	t.menu = menu.NewDbusmenu(t.conn.Object(sender, name))
	w.SetContent(t.parseMenu(0, &pos, func() {
		w.Close()
	}))

	size := w.Content().MinSize()
	if size.IsZero() { // empty menu - weird but don't crash
		size = fyne.NewSquareSize(1)
	}
	w.Resize(size)

	pos.X -= size.Width
	screen := tyde.Instance().Screens().Primary()
	if pos.Y+size.Height > float32(screen.Height)/screen.CanvasScale() {
		pos.Y = float32(screen.Height)/screen.CanvasScale() - size.Height
	}
	tyde.Instance().WindowManager().ShowOverlay(w, size, pos)
}

func (t *tray) fetchIcon(i *node) fyne.Resource {
	ic, _ := i.ni.GetIconPixmap(t.conn.Context())
	if len(ic) > 0 {
		img := pixelsToImage(ic[0])
		unique := strconv.Itoa(resourceID) + ".png"
		resourceID++
		w := &bytes.Buffer{}
		_ = png.Encode(w, img)
		return fyne.NewStaticResource(unique, w.Bytes())
	}

	name, _ := i.ni.GetIconName(t.conn.Context())
	path, _ := i.ni.GetIconThemePath(t.conn.Context())
	fullPath := ""
	if path != "" {
		fullPath = filepath.Join(path, name+".png")
		if _, err := os.Stat(fullPath); err != nil { // not found, search instead
			fullPath = appie.FdoLookupIconPathInTheme("64", filepath.Join(path, "hicolor"), "", name)
		}
	} else {
		fullPath = appie.FdoLookupIconPath("", 64, name)
	}
	img, err := os.ReadFile(fullPath)
	if err != nil {
		fyne.LogError("Failed to load status icon", err)
		return wmtheme.BrokenImageIcon
	}
	return fyne.NewStaticResource(name, img)
}

func createPropSpec() map[string]map[string]*prop.Prop {
	return map[string]map[string]*prop.Prop{
		"org.kde.StatusNotifierWatcher": {
			"RegisteredStatusNotifierItems": {
				Value:    []string{},
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: nil,
			},
			"IsStatusNotifierHostRegistered": {
				Value:    true,
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: nil,
			},
			"ProtocolVersion": {
				Value:    int32(25),
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: nil,
			},
		},
	}
}

type img struct {
	w, h int
	data []byte
}

func (i *img) ColorModel() color.Model {
	return color.NRGBAModel
}

func (i *img) Bounds() image.Rectangle {
	return image.Rect(0, 0, i.w, i.h)
}

func (i *img) At(x, y int) color.Color {
	off := (y*i.w + x) * 4

	a, r, g, b := i.data[off], i.data[off+1], i.data[off+2], i.data[off+3]

	return color.NRGBA{r, g, b, a}
}

func pixelsToImage(in struct {
	V0 int32
	V1 int32
	V2 []byte
},
) image.Image {
	return &img{int(in.V0), int(in.V1), in.V2}
}
