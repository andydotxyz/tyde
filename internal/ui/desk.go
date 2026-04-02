package ui

import (
	"image/color"
	"math"
	"os/exec"
	"strconv"

	"github.com/FyshOS/appie"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	deskDriver "fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"fyshos.com/fynedesk"
	"fyshos.com/fynedesk/internal/notify"
	wmtheme "fyshos.com/fynedesk/theme"
	"fyshos.com/fynedesk/wm"
)

const (
	// RootWindowName is the base string that all root windows will have in their title and is used to identify root windows.
	RootWindowName = "Fyne Desktop"
	// SkipTaskbarHint should be added to the title of normal windows that should be skipped like the X11 SkipTaskbar hint.
	SkipTaskbarHint = "FyneDesk:skip"
)

type desktop struct {
	wm.ShortcutHandler
	app      fyne.App
	wm       fynedesk.WindowManager
	icons    appie.Provider
	recent   []appie.AppData
	screens  fynedesk.ScreenList
	settings fynedesk.DeskSettings

	run         func()
	showMenu    func(*fyne.Menu, fyne.Position)
	moduleCache []fynedesk.Module

	bar     *bar
	widgets *widgetPanel
	mouse   fyne.CanvasObject
	overlay *fyne.Container
	root    fyne.Window
	desk    int
}

func (l *desktop) Desktop() int {
	return l.desk
}

func (l *desktop) SetDesktop(id int) {
	diff := id - l.desk
	l.desk = id

	_, height := l.RootSizePixels()
	offPix := float32(diff * -int(height))
	wins := l.wm.Windows()

	starts := make([]fyne.Position, len(wins))
	deltas := make([]fyne.Delta, len(wins))
	for i, win := range wins {
		starts[i] = win.Position()

		display := l.Screens().ScreenForWindow(win)
		off := offPix / display.Scale
		deltas[i] = fyne.NewDelta(0, off)
	}

	fyne.NewAnimation(canvas.DurationStandard, func(f float32) {
		for i, item := range l.wm.Windows() {
			if item.Pinned() {
				continue
			}

			newX := starts[i].X + deltas[i].DX*f
			newY := starts[i].Y + deltas[i].DY*f

			item.Move(fyne.NewPos(newX, newY))
		}
	}).Start()

	for _, m := range l.Modules() {
		if desk, ok := m.(notify.DesktopNotify); ok {
			desk.DesktopChangeNotify(id)
		}
	}
}

func (l *desktop) ShowSettings() {
	l.widgets.showSettings()
}

func (l *desktop) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	// Calculate the window origin (top-left of the bounding box of all screens)
	originX, originY := 0, 0
	for _, screen := range l.screens.Screens() {
		if screen.X < originX {
			originX = screen.X
		}
		if screen.Y < originY {
			originY = screen.Y
		}
	}

	// Position background, bar and widgets on the primary screen only.
	// Coordinates are relative to the window origin.
	primary := l.screens.Primary()
	scale := primary.CanvasScale()

	pX := float32(primary.X-originX) / scale
	pY := float32(primary.Y-originY) / scale
	pW := float32(primary.Width) / scale
	pH := float32(primary.Height) / scale

	bg := objects[0].(*background)
	bg.Resize(size)

	if l.Settings().NarrowLeftLauncher() {
		l.bar.Resize(fyne.NewSize(wmtheme.NarrowBarWidth, pH))
		l.bar.Move(fyne.NewPos(pX, pY))
	} else {
		barHeight := l.bar.MinSize().Height
		l.bar.Resize(fyne.NewSize(pW, barHeight+1)) // add 1 so rounding cannot trigger mouse out on bottom edge
		l.bar.Move(fyne.NewPos(pX, pY+pH-barHeight))
	}
	l.bar.Refresh()

	widgetsWidth := l.widgets.MinSize().Width
	l.widgets.Resize(fyne.NewSize(widgetsWidth, pH))
	l.widgets.Move(fyne.NewPos(pX+pW-widgetsWidth, pY))
	l.widgets.Refresh()

	// Size overlay objects (between widgets and mouse) to the full window
	for i := 3; i < len(objects)-1; i++ {
		objects[i].Resize(size)
		objects[i].Move(fyne.NewPos(0, 0))
	}
}

func (l *desktop) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(640, 480) // tiny - window manager will scale up to screen size
}

func (l *desktop) Root() fyne.Window {
	return l.root
}

func (l *desktop) ShowMenuAt(menu *fyne.Menu, pos fyne.Position) {
	l.showMenu(menu, pos)
}

// rootRaiser is implemented by window managers that can raise/lower the desktop root window.
type rootRaiser interface {
	RaiseRoot()
	LowerRoot()
}

// backdrop is a full-screen widget that dismisses an overlay on tap or mouse-in
// (mouse-in on the backdrop means the cursor left the overlay content).
type backdrop struct {
	widget.BaseWidget
	onDismiss func()
}

func (b *backdrop) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(canvas.NewRectangle(color.Transparent))
}

func (b *backdrop) Tapped(*fyne.PointEvent) {
	if b.onDismiss != nil {
		b.onDismiss()
	}
}

func (b *backdrop) MouseIn(*deskDriver.MouseEvent) {
	// Cursor entered the backdrop = left the overlay content
	if b.onDismiss != nil {
		b.onDismiss()
	}
}

func (b *backdrop) MouseMoved(*deskDriver.MouseEvent) {}
func (b *backdrop) MouseOut()                         {}

func newBackdrop(onDismiss func()) *backdrop {
	b := &backdrop{onDismiss: onDismiss}
	b.ExtendBaseWidget(b)
	return b
}

// hoverCatch is a wrapper that absorbs hover events, preventing them from
// falling through to the backdrop when the cursor is between child widgets.
type hoverCatch struct {
	widget.BaseWidget
	content fyne.CanvasObject
}

func (h *hoverCatch) CreateRenderer() fyne.WidgetRenderer {
	// Use WithoutLayout so the content keeps its own size
	// and doesn't get stretched to the full hoverCatch area.
	return widget.NewSimpleRenderer(container.NewWithoutLayout(h.content))
}

func (h *hoverCatch) MouseIn(*deskDriver.MouseEvent)    {}
func (h *hoverCatch) MouseMoved(*deskDriver.MouseEvent) {}
func (h *hoverCatch) MouseOut()                         {}

func newHoverCatch(content fyne.CanvasObject) *hoverCatch {
	h := &hoverCatch{content: content}
	h.ExtendBaseWidget(h)
	return h
}

// ShowOverlayWithBackdrop shows an overlay with click-outside and mouse-out dismiss.
// catchSize defines the hover-sensitive area (use content size + room for submenus).
// Returns the combined object (backdrop + content) for use with HideOverlay.
func (l *desktop) ShowOverlayWithBackdrop(content fyne.CanvasObject, size fyne.Size, catchSize fyne.Size, pos fyne.Position) fyne.CanvasObject {
	var combined fyne.CanvasObject
	dismiss := func() {
		l.HideOverlay(combined)
	}

	bg := newBackdrop(dismiss)
	catch := newHoverCatch(content)
	catch.Resize(catchSize)
	catch.Move(pos)
	content.Move(fyne.NewPos(0, 0))
	content.Resize(size)
	combined = container.NewStack(bg, container.NewWithoutLayout(catch))

	// Size the combined to fill the full window
	winSize := l.root.Canvas().Size()
	l.ShowOverlay(combined, winSize, fyne.NewPos(0, 0))
	return combined
}

// ShowOverlay adds content to the desktop overlay layer, above all chrome and windows.
// The root window is raised so that Fyne receives mouse events instead of X11 frame windows.
func (l *desktop) ShowOverlay(content fyne.CanvasObject, size fyne.Size, pos fyne.Position) {
	fyne.Do(func() {
		content.Resize(size)
		content.Move(pos)
		l.overlay.Add(content)
		l.overlay.Refresh()
	})

	if rr, ok := l.wm.(rootRaiser); ok {
		rr.RaiseRoot()
	}
}

// HideOverlay removes content from the desktop overlay layer.
// When no overlays remain, the root window is lowered so X11 windows receive events again.
func (l *desktop) HideOverlay(content fyne.CanvasObject) {
	fyne.Do(func() {
		l.overlay.Remove(content)
		l.overlay.Refresh()

		if len(l.overlay.Objects) == 0 {
			if rr, ok := l.wm.(rootRaiser); ok {
				rr.LowerRoot()
			}
		}
	})
}

func (l *desktop) updateBackgrounds(path string) {
	root := l.root.Content().(*fyne.Container).Objects[0]
	if back, ok := root.(*background); ok {
		back.updateBackground(path)
	} else { // embed mode has another container
		root.(*fyne.Container).Objects[0].(*background).updateBackground(path)
	}
}

func (l *desktop) createPrimaryContent() fyne.CanvasObject {
	l.bar = newBar(l)
	l.widgets = newWidgetPanel(l)
	l.mouse = newMouse()
	l.mouse.Hide()

	// Order: background (wallpapers + files + compositor) -> bar -> widgets -> overlays -> mouse
	objects := []fyne.CanvasObject{newBackground(), l.bar, l.widgets}

	// Add overlay widgets (e.g. fullscreen windows) above desktop chrome
	for _, m := range l.Modules() {
		if om, ok := m.(fynedesk.OverlayModule); ok {
			if wid := om.OverlayWidget(); wid != nil {
				objects = append(objects, wid)
			}
		}
	}

	// UI overlay for menus, dialogs, switcher, notifications
	l.overlay = container.NewWithoutLayout()
	objects = append(objects, l.overlay, l.mouse)
	return container.New(l, objects...)
}

func (l *desktop) createRoot(screens fynedesk.ScreenList) fyne.Window {
	win := l.newDesktopWindowFull()

	win.SetContent(l.createPrimaryContent())

	return win
}

func (l *desktop) setupRoot() {
	if l.root == nil {
		l.root = l.createRoot(l.screens)
	}

	// Size the root window to cover all screens so the compositor can render windows on any display
	w, h := l.RootSizePixels()
	scale := l.screens.Primary().CanvasScale()
	l.root.Resize(fyne.NewSize(float32(w)/scale, float32(h)/scale))
}

func (l *desktop) RecentApps() []appie.AppData {
	return l.recent
}

func (l *desktop) Run() {
	go l.wm.Run()
	go l.watchScreenActivity()
	l.run() // use the configured run method
}

func (l *desktop) RunApp(app appie.AppData) error {
	return l.runExec(app, app.Run)
}

func (l *desktop) RunAppAction(app appie.AppData, id int) error {
	if app.Actions() == nil || len(app.Actions())-1 < id {
		return nil
	}

	return l.runExec(app, app.Actions()[id].Run)
}

func (l *desktop) runExec(app appie.AppData, runner func(env []string) error) error {
	vars := l.scaleVars(l.Screens().Active().CanvasScale())
	err := runner(vars)

	if err == nil {
		l.recent = append([]appie.AppData{app}, l.recent...)
		// remove if it was already on the list
		for i := 1; i < len(l.recent); i++ {
			if l.recent[i] == app {
				if i == len(l.recent)-1 {
					l.recent = l.recent[:i]
				} else {
					l.recent = append(l.recent[:i], l.recent[i+1:]...)
				}
				break
			}
		}
		// limit to 5 items
		if len(l.recent) > 5 {
			l.recent = l.recent[:5]
		}
		l.settings.(*deskSettings).saveRecents()
	}
	return err
}

func (l *desktop) Settings() fynedesk.DeskSettings {
	return l.settings
}

func (l *desktop) ContentBoundsPixels(screen *fynedesk.Screen) (x, y, w, h uint32) {
	screenW := uint32(screen.Width)
	screenH := uint32(screen.Height)
	pad := wmtheme.WidgetPanelWidth
	if l.Settings().NarrowWidgetPanel() {
		pad = wmtheme.NarrowBarWidth
	}
	if l.screens.Primary() == screen {
		bar := uint32(0)
		if l.Settings().NarrowLeftLauncher() {
			bar = uint32(wmtheme.NarrowBarWidth * screen.CanvasScale())
		}
		wid := uint32(pad * screen.CanvasScale())
		return bar, 0, screenW - bar - wid, screenH
	}
	return 0, 0, screenW, screenH
}

func (l *desktop) RootSizePixels() (w, h uint32) {
	for _, screen := range l.Screens().Screens() {
		right := uint32(screen.X + screen.Width)
		bottom := uint32(screen.Y + screen.Height)

		if right > w {
			w = right
		}
		if bottom > h {
			h = bottom
		}
	}

	return w, h
}

func (l *desktop) IconProvider() appie.Provider {
	return l.icons
}

func (l *desktop) WindowManager() fynedesk.WindowManager {
	return l.wm
}

func (l *desktop) clearModuleCache() {
	for _, mod := range l.moduleCache {
		mod.Destroy()
	}

	l.moduleCache = nil
}

func (l *desktop) Modules() []fynedesk.Module {
	if l.moduleCache != nil {
		return l.moduleCache
	}

	var mods []fynedesk.Module
	for _, meta := range fynedesk.AvailableModules() {
		if !isModuleEnabled(meta.Name, l.settings) {
			continue
		}

		instance := meta.NewInstance()
		mods = append(mods, instance)

		if bind, ok := instance.(fynedesk.KeyBindModule); ok {
			for sh, f := range bind.Shortcuts() {
				l.AddShortcut(sh, f)
			}
		}
	}

	l.moduleCache = mods
	return mods
}

func (l *desktop) qtScreenScales() string {
	screenScales := ""
	for i, screen := range l.Screens().Screens() {
		if i > 0 {
			screenScales += ";"
		}
		// Qt toolkit cannot handle scale < 1
		positiveScale := math.Max(1.0, float64(screen.CanvasScale()))
		screenScales += screen.Name + "=" + strconv.FormatFloat(positiveScale, 'f', 1, 32)
	}
	return screenScales
}

func (l *desktop) scaleVars(scale float32) []string {
	intScale := int(math.Round(float64(scale)))

	return []string{
		"QT_SCREEN_SCALE_FACTORS=" + l.qtScreenScales(),
		"GDK_SCALE=" + strconv.Itoa(intScale),
		"ELM_SCALE=" + strconv.FormatFloat(float64(scale), 'f', 1, 32),
	}
}

// MouseInNotify can be called by the window manager to alert the desktop that the cursor has entered the canvas
func (l *desktop) MouseInNotify(pos fyne.Position) {
	if l.bar == nil {
		return
	}

	fyne.Do(func() {
		mouseX, mouseY := pos.X, pos.Y
		barX, barY := l.bar.Position().X, l.bar.Position().Y
		barWidth, barHeight := l.bar.Size().Width, l.bar.Size().Height
		if mouseX >= barX && mouseX <= barX+barWidth {
			if mouseY >= barY && mouseY <= barY+barHeight {
				l.bar.MouseIn(&deskDriver.MouseEvent{PointEvent: fyne.PointEvent{AbsolutePosition: pos, Position: pos}})
			}
		}
	})
}

// MouseOutNotify can be called by the window manager to alert the desktop that the cursor has left the canvas
func (l *desktop) MouseOutNotify() {
	if l.bar == nil {
		return
	}
	fyne.Do(l.bar.MouseOut)
}

func (l *desktop) fireSettingsChangeListener(s fynedesk.DeskSettings) {
	l.clearModuleCache()
	l.updateBackgrounds(s.Background())
	l.widgets.reloadModules(l.Modules())

	l.bar.iconSize = l.Settings().LauncherIconSize()
	l.bar.iconScale = l.Settings().LauncherZoomScale()
	l.bar.disableZoom = l.Settings().LauncherDisableZoom()
	l.bar.updateIcons()
	l.bar.updateIconOrder()
	l.bar.updateTaskbar()
}

func (l *desktop) addSettingsChangeListener() {
	l.Settings().AddChangeListener(l.fireSettingsChangeListener)

	l.app.Settings().AddListener(func(_ fyne.Settings) {
		l.updateBackgrounds(l.Settings().Background())
	})
}

func (l *desktop) registerShortcuts() {
	l.AddShortcut(fynedesk.NewShortcut("Show Launcher", fyne.KeySpace, fynedesk.UserModifier),
		ShowAppLauncher)
	l.AddShortcut(fynedesk.NewShortcut("Switch App Next", fyne.KeyTab, fynedesk.UserModifier),
		func() {
			// dummy - the wm handles app switcher
		})
	l.AddShortcut(fynedesk.NewShortcut("Switch App Previous", fyne.KeyTab, fynedesk.UserModifier|fyne.KeyModifierShift),
		func() {
			// dummy - the wm handles app switcher
		})
	l.AddShortcut(fynedesk.NewShortcut("Iconify Window", fyne.KeyF9, fynedesk.UserModifier),
		l.iconifyCurrentWindow)
	l.AddShortcut(fynedesk.NewShortcut("Maximize Window", fyne.KeyF10, fynedesk.UserModifier),
		l.maximizeCurrentWindow)
	l.AddShortcut(fynedesk.NewShortcut("FullScreen Window", fyne.KeyF11, fynedesk.UserModifier),
		l.fullscreenCurrentWindow)
	l.AddShortcut(fynedesk.NewShortcut("Print Window", deskDriver.KeyPrintScreen, fyne.KeyModifierShift),
		l.screenshotWindow)
	l.AddShortcut(fynedesk.NewShortcut("Print Screen", deskDriver.KeyPrintScreen, 0),
		l.screenshot)
	l.AddShortcut(fynedesk.NewShortcut("Calculator", fynedesk.KeyCalculator, 0),
		l.calculator)
	l.AddShortcut(fynedesk.NewShortcut("Lock screen", fyne.KeyL, fynedesk.UserModifier),
		func() {
			l.TriggerScreenSaver(false)
		})
}

// Screens returns the screens provider of the current desktop environment for access to screen functionality.
func (l *desktop) Screens() fynedesk.ScreenList {
	return l.screens
}

// NewDesktop creates a new desktop in fullscreen for main usage.
// The WindowManager passed in will be used to manage the screen it is loaded on.
// An ApplicationProvider is used to lookup application icons from the operating system.
func NewDesktop(app fyne.App, mgr fynedesk.WindowManager, icons appie.Provider, screenProvider fynedesk.ScreenList) fynedesk.Desktop {
	desk := newDesktop(app, mgr, icons)
	desk.run = desk.runFull
	screenProvider.AddChangeListener(desk.setupRoot)
	desk.screens = screenProvider

	desk.setupRoot()
	wm.StartAuthAgent()
	if desk.Settings().ScreenSaverType() == "XScreensaver" {
		go desk.startXscreensaver()
	}
	return desk
}

// NewEmbeddedDesktop creates a new windowed desktop for test purposes.
// An ApplicationProvider is used to lookup application icons from the operating system.
// If run during CI for testing it will return an in-memory window using the
// fyne/test package.
func NewEmbeddedDesktop(app fyne.App, icons appie.Provider) fynedesk.Desktop {
	wm := &embededWM{}
	desk := newDesktop(app, wm, icons)
	desk.run = desk.runEmbed
	desk.showMenu = desk.showMenuEmbed

	desk.root = desk.newDesktopWindowEmbed()
	over := wm.setWindow(desk.root)
	desk.root.SetContent(container.NewStack(desk.createPrimaryContent(), over))
	return desk
}

func newDesktop(app fyne.App, wm fynedesk.WindowManager, icons appie.Provider) *desktop {
	desk := &desktop{app: app, wm: wm, icons: icons, screens: newEmbeddedScreensProvider()}
	desk.showMenu = desk.showMenuFull

	fynedesk.SetInstance(desk)
	desk.settings = newDeskSettings()
	desk.addSettingsChangeListener()

	desk.registerShortcuts()
	return desk
}

func (l *desktop) calculator() {
	err := exec.Command("calculator").Start()
	if err != nil {
		fyne.LogError("Failed to open calculator", err)
	}
}

func (l *desktop) fullscreenCurrentWindow() {
	if len(l.WindowManager().Windows()) == 0 {
		return
	}

	w := l.WindowManager().Windows()[0]
	if w.Fullscreened() {
		w.Unfullscreen()
	} else {
		w.Fullscreen()
	}
}

func (l *desktop) iconifyCurrentWindow() {
	if len(l.WindowManager().Windows()) == 0 {
		return
	}

	w := l.WindowManager().Windows()[0]
	w.Iconify()
}

func (l *desktop) maximizeCurrentWindow() {
	if len(l.WindowManager().Windows()) == 0 {
		return
	}

	w := l.WindowManager().Windows()[0]
	if w.Maximized() {
		w.Unmaximize()
	} else {
		w.Maximize()
	}
}

//func (l *desktop) runCommand() {
//	w := l.app.NewWindow("Run Command")
//	input := widget.NewEntry()
//	// TODO add history etc...
//	run := widget.NewButton("Run", func() {
//
//	})
//	run.Importance = widget.HighImportance
//
//	w.SetContent(container.NewVBox(widget.NewLabel("Enter command to run:"),
//		container.NewBorder(nil, nil, nil, run, input)))
//	w.Resize(fyne.NewSize(250, 40))
//	w.Show()
//}
