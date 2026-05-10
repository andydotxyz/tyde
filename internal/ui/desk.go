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

	"fyshos.com/tyde"
	"fyshos.com/tyde/internal/notify"
	wmtheme "fyshos.com/tyde/theme"
	"fyshos.com/tyde/wm"
)

const (
	// RootWindowName is the base string that all root windows will have in their title and is used to identify root windows.
	RootWindowName = "Fyne Desktop"
)

// screenWindow holds the Fyne window and per-screen widgets for a single monitor.
type screenWindow struct {
	screen            *tyde.Screen
	win               fyne.Window
	compositor        *CompositorWidget
	compositorOverlay *CompositorWidget
	bg                *background
	overlay           *fyne.Container
}

// ScreenCompositors groups the compositor widgets for a single screen,
// passed to the platform compositor so it can route windows per-monitor.
type ScreenCompositors struct {
	Screen  *tyde.Screen
	Normal  *CompositorWidget
	Overlay *CompositorWidget
}

type desktop struct {
	wm.ShortcutHandler
	app      fyne.App
	wm       tyde.WindowManager
	icons    appie.Provider
	recent   []appie.AppData
	screens  tyde.ScreenList
	settings tyde.DeskSettings

	run         func()
	showMenu    func(*fyne.Menu, fyne.Position)
	moduleCache []tyde.Module

	bar             *bar
	widgets         *widgetPanel
	mouse           fyne.CanvasObject
	screenWindows   []*screenWindow
	primaryWin      *screenWindow
	desk            int
	deskAnim        *fyne.Animation
	deskAnimTargets map[tyde.Window]fyne.Position // where the in-flight animation is heading
	compositorDone  chan struct{}
}

func (l *desktop) Desktop() int {
	return l.desk
}

func (l *desktop) SetDesktop(id int) {
	diff := id - l.desk
	prevTargets := l.deskAnimTargets

	// Stop any in-flight animation; the new one takes over.
	if l.deskAnim != nil {
		l.deskAnim.Stop()
		l.deskAnim = nil
	}

	l.desk = id

	_, height := l.RootSizePixels()
	offPix := float32(diff * -int(height))
	wins := l.wm.Windows()

	starts := make([]fyne.Position, len(wins))
	targets := make(map[tyde.Window]fyne.Position, len(wins))
	for i, win := range wins {
		// If the previous animation was heading somewhere, start from
		// that target rather than the current (mid-flight) position.
		if prev, ok := prevTargets[win]; ok {
			starts[i] = prev
		} else {
			starts[i] = win.Position()
		}

		display := l.Screens().ScreenForWindow(win)
		off := offPix / display.CanvasScale()
		targets[win] = fyne.NewPos(starts[i].X, starts[i].Y+off)
	}

	type visualMover interface {
		MoveVisual(fyne.Position)
	}

	l.deskAnimTargets = targets
	var a *fyne.Animation
	a = fyne.NewAnimation(canvas.DurationStandard, func(f float32) {
		if l.deskAnim != a {
			return // superseded by a newer animation
		}
		for i, item := range wins {
			if item.Pinned() {
				continue
			}

			target := targets[item]
			newX := starts[i].X + (target.X-starts[i].X)*f
			newY := starts[i].Y + (target.Y-starts[i].Y)*f
			pos := fyne.NewPos(newX, newY)

			if f >= 1.0 {
				item.Move(pos)
				if i == len(wins)-1 {
					l.deskAnim = nil
					l.deskAnimTargets = nil
					for _, m := range l.Modules() {
						if desk, ok := m.(notify.DesktopNotify); ok {
							desk.DesktopChangeNotify(id)
						}
					}
				}
			} else if vm, ok := item.(visualMover); ok {
				vm.MoveVisual(pos)
			} else {
				item.Move(pos)
			}
		}
	})
	l.deskAnim = a
	a.Start()
}

func (l *desktop) ShowSettings() {
	l.widgets.showSettings()
}

func (l *desktop) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if esp, ok := l.screens.(*embeddedScreensProvider); ok {
		esp.UpdatePrimarySize(int(size.Width), int(size.Height))
	}

	// Each window covers exactly one screen, so origin is always 0,0.
	pW := size.Width
	pH := size.Height

	// objects order: background, [compositor], bar, widgets, [compositorOverlay], overlay, mouse
	// Size all full-window layers (everything except bar and widgets) to fill.
	for _, o := range objects {
		if o == l.bar || o == l.widgets || o == l.mouse {
			continue
		}
		o.Resize(size)
		o.Move(fyne.NewPos(0, 0))
	}

	l.bar.Resize(fyne.NewSize(wmtheme.NarrowBarWidth, pH))
	l.bar.Move(fyne.NewPos(0, 0))
	l.bar.Refresh()

	widgetsWidth := l.widgets.MinSize().Width
	l.widgets.Resize(fyne.NewSize(widgetsWidth, pH))
	l.widgets.Move(fyne.NewPos(pW-widgetsWidth, 0))
	l.widgets.Refresh()
}

func (l *desktop) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(640, 480) // tiny - window manager will scale up to screen size
}

func (l *desktop) Root() fyne.Window {
	if l.primaryWin == nil {
		return nil
	}
	return l.primaryWin.win
}

func (l *desktop) ShowMenuAt(menu *fyne.Menu, pos fyne.Position) {
	l.showMenu(menu, pos)
}

// inputShaper is implemented by window managers that can update X11 input shapes
// on the root and frame windows to control which areas receive mouse events.
type inputShaper interface {
	SetOverlayActive(active bool)
}

// backdrop is a full-screen widget that dismisses an overlay on tap or mouse-in
// (mouse-in on the backdrop means the cursor left the overlay content).
// Mouse-out dismiss only activates after the mouse has been inside the content at least once.
type backdrop struct {
	widget.BaseWidget
	onDismiss func()
	armed     bool // true after mouse has entered the content area
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
	// Only dismiss if the mouse was previously inside the content
	if b.armed && b.onDismiss != nil {
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
// It also arms the backdrop for mouse-out dismiss once the mouse enters.
type hoverCatch struct {
	widget.BaseWidget
	content  fyne.CanvasObject
	backdrop *backdrop
}

func (h *hoverCatch) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewWithoutLayout(h.content))
}

func (h *hoverCatch) MouseIn(*deskDriver.MouseEvent) {
	if h.backdrop != nil {
		h.backdrop.armed = true
	}
}

func (h *hoverCatch) MouseMoved(*deskDriver.MouseEvent) {}
func (h *hoverCatch) MouseOut()                         {}

func newHoverCatch(content fyne.CanvasObject, bg *backdrop) *hoverCatch {
	h := &hoverCatch{content: content, backdrop: bg}
	h.ExtendBaseWidget(h)
	return h
}

// ShowOverlayWithBackdrop shows an overlay with click-outside and mouse-out dismiss.
// catchSize defines the hover-sensitive area (use content size + room for submenus).
// Returns the combined object (backdrop + content) for use with HideOverlay.
func (l *desktop) ShowOverlayWithBackdrop(content fyne.CanvasObject, size fyne.Size, catchSize fyne.Size, pos fyne.Position, contentOffset fyne.Position) fyne.CanvasObject {
	return l.showOverlayWithBackdrop(content, size, catchSize, pos, nil, contentOffset)
}

func (l *desktop) showOverlayWithBackdrop(content fyne.CanvasObject, size fyne.Size, catchSize fyne.Size, pos fyne.Position, focus fyne.Focusable, contentOffset fyne.Position) fyne.CanvasObject {
	var combined fyne.CanvasObject
	dismiss := func() {
		l.HideOverlay(combined)
	}

	bg := newBackdrop(dismiss)
	catch := newHoverCatch(content, bg)
	catch.Resize(catchSize)
	catch.Move(pos)
	content.Move(contentOffset)
	content.Resize(size)
	combined = container.NewStack(bg, container.NewWithoutLayout(catch))

	// Size the combined to fill the full window
	winSize := l.primaryWin.win.Canvas().Size()
	l.showOverlay(combined, winSize, fyne.NewPos(0, 0), focus)
	return combined
}

// ShowOverlay adds content to the desktop overlay layer, above all chrome and windows.
// The root window's input shape is expanded and frame input shapes are cleared
// so that Fyne receives mouse events for the overlay.
func (l *desktop) ShowOverlay(content fyne.CanvasObject, size fyne.Size, pos fyne.Position) {
	l.showOverlay(content, size, pos, nil)
}

func (l *desktop) showOverlay(content fyne.CanvasObject, size fyne.Size, pos fyne.Position, focus fyne.Focusable) {
	overlay := l.primaryWin.overlay
	win := l.primaryWin.win
	fyne.Do(func() {
		content.Resize(size)
		content.Move(pos)
		overlay.Add(content)
		overlay.Refresh()

		if is, ok := l.wm.(inputShaper); ok {
			is.SetOverlayActive(true)
		}

		if focus != nil {
			win.Canvas().Focus(focus)
		}
	})
}

// HideOverlay removes content from the desktop overlay layer.
// When no overlays remain, input shapes are restored to normal.
func (l *desktop) HideOverlay(content fyne.CanvasObject) {
	overlay := l.primaryWin.overlay
	fyne.Do(func() {
		overlay.Remove(content)
		overlay.Refresh()

		if len(overlay.Objects) == 0 {
			if is, ok := l.wm.(inputShaper); ok {
				is.SetOverlayActive(false)
			}
		}
	})
}

func (l *desktop) updateBackgrounds(path string) {
	for _, sw := range l.screenWindows {
		if sw.bg != nil {
			sw.bg.updateBackground(path)
		}
	}
}

func (l *desktop) createPrimaryContent(sw *screenWindow) fyne.CanvasObject {
	l.bar = newBar(l)
	l.widgets = newWidgetPanel(l)
	l.mouse = newMouse()
	l.mouse.Hide()

	sw.bg = newBackground()

	// Order: background -> compositor -> bar -> widgets -> compositor overlay -> UI overlay -> mouse
	objects := []fyne.CanvasObject{sw.bg}

	// Normal compositor for regular windows below desktop chrome
	if sw.compositor != nil {
		objects = append(objects, sw.compositor)
	}

	objects = append(objects, l.bar, l.widgets)

	// Compositor overlay for fullscreen windows above desktop chrome
	if sw.compositorOverlay != nil {
		objects = append(objects, sw.compositorOverlay)
	}

	// UI overlay for menus, dialogs, switcher, notifications
	sw.overlay = container.NewWithoutLayout()
	objects = append(objects, sw.overlay, l.mouse)
	return container.New(l, objects...)
}

// secondaryLayout is a simple layout for non-primary screen windows (no bar/widgets).
type secondaryLayout struct{}

func (s *secondaryLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objects {
		o.Resize(size)
		o.Move(fyne.NewPos(0, 0))
	}
}

func (s *secondaryLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(640, 480)
}

func (l *desktop) createSecondaryContent(sw *screenWindow) fyne.CanvasObject {
	sw.bg = newBackground()

	objects := []fyne.CanvasObject{sw.bg}

	// Normal compositor for regular windows
	if sw.compositor != nil {
		objects = append(objects, sw.compositor)
	}

	// Compositor overlay for fullscreen windows
	if sw.compositorOverlay != nil {
		objects = append(objects, sw.compositorOverlay)
	}

	sw.overlay = container.NewWithoutLayout()
	objects = append(objects, sw.overlay)
	return container.New(&secondaryLayout{}, objects...)
}

func (l *desktop) setupRoot() {
	primary := l.screens.Primary()

	// Build or update screenWindow for each screen
	existingByName := make(map[string]*screenWindow, len(l.screenWindows))
	for _, sw := range l.screenWindows {
		existingByName[sw.screen.Name] = sw
	}

	var newWindows []*screenWindow
	for _, screen := range l.screens.Screens() {
		sw := existingByName[screen.Name]
		if sw != nil {
			// Update screen pointer (geometry may have changed)
			sw.screen = screen
			if sw.compositor != nil {
				sw.compositor.Screen = screen
			}
			if sw.compositorOverlay != nil {
				sw.compositorOverlay.Screen = screen
			}
			delete(existingByName, screen.Name)
		} else {
			// Create new screenWindow
			sw = &screenWindow{screen: screen}
			if l.compositorDone != nil {
				sw.compositor = NewCompositorWidget(screen)
				sw.compositorOverlay = NewCompositorWidget(screen)
			}
			win := l.app.NewWindow(RootWindowName + screen.Name)
			win.SetPadded(false)
			sw.win = win

			if screen == primary {
				win.SetMaster()
				win.SetOnClosed(func() {
					if l.compositorDone != nil {
						close(l.compositorDone)
					}
					l.wm.Close()
				})
				win.SetContent(l.createPrimaryContent(sw))
			} else {
				win.SetContent(l.createSecondaryContent(sw))
			}
		}

		newWindows = append(newWindows, sw)
		if screen == primary {
			l.primaryWin = sw
		}
	}

	// Close windows for disconnected screens
	for _, sw := range existingByName {
		sw.win.Close()
	}

	l.screenWindows = newWindows

	// Resize each window to cover its screen
	for _, sw := range l.screenWindows {
		scale := sw.screen.CanvasScale()
		sw.win.Resize(fyne.NewSize(float32(sw.screen.Width)/scale, float32(sw.screen.Height)/scale))
	}
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

func (l *desktop) Settings() tyde.DeskSettings {
	return l.settings
}

func (l *desktop) ContentBoundsPixels(screen *tyde.Screen) (x, y, w, h uint32) {
	screenW := uint32(screen.Width)
	screenH := uint32(screen.Height)
	pad := wmtheme.WidgetPanelWidth
	if l.Settings().NarrowWidgetPanel() {
		pad = wmtheme.NarrowBarWidth
	}
	if l.screens.Primary() == screen {
		bar := uint32(wmtheme.NarrowBarWidth * screen.CanvasScale())
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

func (l *desktop) WindowManager() tyde.WindowManager {
	return l.wm
}

func (l *desktop) clearModuleCache() {
	for _, mod := range l.moduleCache {
		mod.Destroy()
	}

	l.moduleCache = nil
}

func (l *desktop) Modules() []tyde.Module {
	if l.moduleCache != nil {
		return l.moduleCache
	}

	var mods []tyde.Module
	for _, meta := range tyde.AvailableModules() {
		if !isModuleEnabled(meta.Name, l.settings) {
			continue
		}

		instance := meta.NewInstance()
		mods = append(mods, instance)

		if bind, ok := instance.(tyde.KeyBindModule); ok {
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

func (l *desktop) fireSettingsChangeListener(s tyde.DeskSettings) {
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
	l.AddShortcut(tyde.NewShortcut("Show Launcher", fyne.KeySpace, tyde.UserModifier),
		ShowAppLauncher)
	l.AddShortcut(tyde.NewShortcut("Switch App Next", fyne.KeyTab, tyde.UserModifier),
		func() {
			// dummy - the wm handles app switcher
		})
	l.AddShortcut(tyde.NewShortcut("Switch App Previous", fyne.KeyTab, tyde.UserModifier|fyne.KeyModifierShift),
		func() {
			// dummy - the wm handles app switcher
		})
	l.AddShortcut(tyde.NewShortcut("Iconify Window", fyne.KeyF9, tyde.UserModifier),
		l.iconifyCurrentWindow)
	l.AddShortcut(tyde.NewShortcut("Maximize Window", fyne.KeyF10, tyde.UserModifier),
		l.maximizeCurrentWindow)
	l.AddShortcut(tyde.NewShortcut("FullScreen Window", fyne.KeyF11, tyde.UserModifier),
		l.fullscreenCurrentWindow)
	l.AddShortcut(tyde.NewShortcut("Print Window", deskDriver.KeyPrintScreen, fyne.KeyModifierShift),
		l.screenshotWindow)
	l.AddShortcut(tyde.NewShortcut("Print Screen", deskDriver.KeyPrintScreen, 0),
		l.screenshot)
	l.AddShortcut(tyde.NewShortcut("Calculator", tyde.KeyCalculator, 0),
		l.calculator)
	l.AddShortcut(tyde.NewShortcut("Lock screen", fyne.KeyL, tyde.UserModifier),
		func() {
			l.TriggerScreenSaver(false)
		})
}

// Screens returns the screens provider of the current desktop environment for access to screen functionality.
func (l *desktop) Screens() tyde.ScreenList {
	return l.screens
}

// CompositorRunFunc is a function that runs a platform compositor using the
// provided per-screen widgets. It blocks until done is closed.
type CompositorRunFunc func(done chan struct{}, screens []ScreenCompositors) error

// NewDesktop creates the full desktop environment with window management.
// If compositorRun is non-nil, the compositor is started in a background goroutine.
func NewDesktop(app fyne.App, mgr tyde.WindowManager, icons appie.Provider, screenProvider tyde.ScreenList, compositorRun CompositorRunFunc) tyde.Desktop {
	desk := newDesktop(app, mgr, icons)
	desk.run = desk.runFull
	if compositorRun != nil {
		desk.compositorDone = make(chan struct{})
	}
	screenProvider.AddChangeListener(desk.setupRoot)
	desk.screens = screenProvider

	desk.setupRoot()

	if compositorRun != nil {
		go func() {
			screens := desk.screenCompositors()
			if err := compositorRun(desk.compositorDone, screens); err != nil {
				fyne.LogError("Compositor failed", err)
			}
		}()
	}

	wm.StartAuthAgent()
	if desk.Settings().ScreenSaverType() == "XScreensaver" {
		go desk.startXscreensaver()
	}
	return desk
}

// screenCompositors returns the per-screen compositor widget pairs.
func (l *desktop) screenCompositors() []ScreenCompositors {
	var out []ScreenCompositors
	for _, sw := range l.screenWindows {
		if sw.compositor != nil {
			out = append(out, ScreenCompositors{
				Screen:  sw.screen,
				Normal:  sw.compositor,
				Overlay: sw.compositorOverlay,
			})
		}
	}
	return out
}

// NewEmbeddedDesktop creates a new windowed desktop for test purposes.
// An ApplicationProvider is used to lookup application icons from the operating system.
// If run during CI for testing it will return an in-memory window using the
// fyne/test package.
func NewEmbeddedDesktop(app fyne.App, icons appie.Provider) tyde.Desktop {
	wm := &embededWM{}
	desk := newDesktop(app, wm, icons)
	desk.run = desk.runEmbed
	desk.showMenu = desk.showMenuEmbed

	win := desk.newDesktopWindowEmbed()
	sw := &screenWindow{
		screen: desk.screens.Primary(),
		win:    win,
	}
	desk.screenWindows = []*screenWindow{sw}
	desk.primaryWin = sw
	over := wm.setWindow(win)
	win.SetContent(container.NewStack(desk.createPrimaryContent(sw), over))
	return desk
}

func newDesktop(app fyne.App, wm tyde.WindowManager, icons appie.Provider) *desktop {
	desk := &desktop{app: app, wm: wm, icons: icons, screens: newEmbeddedScreensProvider()}
	desk.showMenu = desk.showMenuFull

	tyde.SetInstance(desk)
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
