package keyboard

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"fyshos.com/tyde"
	wmTheme "fyshos.com/tyde/theme"
)

const prefAtTop = "keyboard.attop"

// keyHeight is the height of one row - large for fitting fingertips.
var keyHeight = float32(48)

// panel is the on-screen keyboard overlay. One instance is reused for the life
// of the module: it is built on first show and then hidden and re-shown.
type panel struct {
	content fyne.CanvasObject
	header  fyne.CanvasObject
	keys    []*keyButton
	flipBtn *widget.Button

	send sender
	// sendBroken records that there is no way to type - no X server, or no XTEST
	sendBroken bool

	held   modifier // modifiers latched for the next keystroke only
	locked bool     // Caps: Shift stays on until it is tapped again
	atTop  bool
	shown  bool
}

// show builds the keyboard if needed and puts it on screen.
func (p *panel) show() {
	if p.shown {
		return
	}

	if p.content == nil {
		p.atTop = fyne.CurrentApp().Preferences().Bool(prefAtTop)
		p.build()
	}
	p.connect()

	size := p.size()
	p.shown = true
	tyde.Instance().ShowOverlay(p.content, size, p.position(size))
}

// hide takes the keyboard off-screen, releasing any latched modifiers so that
// the next time it opens it is in a known state. The content is kept for reuse.
func (p *panel) hide() {
	if !p.shown {
		return
	}

	p.shown = false
	p.held = 0
	p.refreshFaces()
	tyde.Instance().HideOverlay(p.content)
}

// toggle is what the tray icon and the keyboard shortcut both call, so the same
// gesture that opens the keyboard closes it again.
func (p *panel) toggle() {
	if p.shown {
		p.hide()
		return
	}
	p.show()
}

// destroy closes the keyboard and releases the X connection behind it.
func (p *panel) destroy() {
	p.hide()
	if p.send != nil {
		p.send.Close()
		p.send = nil
	}
}

// connect opens the connection typing goes through, once the keyboard is on its
// way up rather than on the first keystroke, so the first key is as quick as the
// rest.
func (p *panel) connect() {
	if p.send != nil || p.sendBroken {
		return
	}

	send, err := newSenderFunc()
	if err != nil {
		fyne.LogError("On screen keyboard cannot type", err)
		p.sendBroken = true
		return
	}
	p.send = send
}

// shifted reports whether the next keystroke is a shifted one, from either the
// one-shot Shift keys or Caps.
func (p *panel) shifted() bool {
	return p.locked || p.held&modShift != 0
}

// tapped handles a key press. Modifiers latch rather than repeat the keystroke,
// because a touchscreen cannot hold one key while pressing another.
func (p *panel) tapped(k key) {
	switch k.kind {
	case keyMod:
		p.held ^= k.mod
		p.refreshFaces()
	case keyLock:
		p.locked = !p.locked
		p.refreshFaces()
	default:
		p.press(k)
	}
}

// press types one key, then drops the latched modifiers the way a hand comes off
// them - except Caps, which stays on until it is tapped again.
func (p *panel) press(k key) {
	shifted := p.shifted()
	mods := p.held
	if shifted {
		mods |= modShift
	}

	if p.send != nil {
		if err := p.send.Send(symbolFor(k, shifted), mods); err != nil {
			fyne.LogError("Could not type from the on screen keyboard", err)
		}
	}

	if p.held != 0 {
		p.held = 0
		p.refreshFaces()
	}
}

// refreshFaces brings the key labels back in line with the modifier state.
func (p *panel) refreshFaces() {
	shifted := p.shifted()
	for _, b := range p.keys {
		latched := (b.def.kind == keyMod && p.held&b.def.mod != 0) ||
			(b.def.kind == keyLock && p.locked)
		b.setFace(shifted, latched)
	}
}

// flip moves the keyboard to the other edge of the screen, for when it is
// covering the very field being typed into.
func (p *panel) flip() {
	p.atTop = !p.atTop
	fyne.CurrentApp().Preferences().SetBool(prefAtTop, p.atTop)
	p.updateFlipIcon()

	if !p.shown {
		return
	}
	// Re-register rather than just moving: the window manager makes the area an
	// overlay covers click-through, and that area is set when it is shown.
	desk := tyde.Instance()
	desk.HideOverlay(p.content)
	p.content.Refresh() // the background overshoots the other edge now
	size := p.size()
	desk.ShowOverlay(p.content, size, p.position(size))
}

func (p *panel) updateFlipIcon() {
	if p.flipBtn == nil {
		return
	}
	if p.atTop {
		p.flipBtn.SetIcon(theme.MoveDownIcon())
		return
	}
	p.flipBtn.SetIcon(theme.MoveUpIcon())
}

// size is the keyboard's on-screen size: the full width of the content area,
// and as tall as its rows plus the strip of controls above them.
func (p *panel) size() fyne.Size {
	pad := theme.Padding()
	height := keyHeight*float32(len(rows)) + pad*float32(len(rows)+3)
	if p.header != nil {
		height += p.header.MinSize().Height
	}

	_, area, ok := contentArea()
	if !ok {
		return fyne.NewSize(float32(800), height)
	}
	if height > area.Height {
		height = area.Height // a screen too short for the keyboard still gets one
	}
	return fyne.NewSize(area.Width, height)
}

// position docks the keyboard against the top or the bottom of the content area,
// flush with that edge so no strip of desktop shows between the two.
func (p *panel) position(size fyne.Size) fyne.Position {
	pos, area, ok := contentArea()
	if !ok {
		return fyne.NewPos(0, 0)
	}

	if p.atTop {
		return pos
	}
	return fyne.NewPos(pos.X, pos.Y+area.Height-size.Height)
}

// contentArea is the part of the primary screen left once the bar and the widget
// panel have taken theirs - the same area a maximised window fills - converted
// from the pixels the desktop reports into the units the overlay is placed in.
func contentArea() (fyne.Position, fyne.Size, bool) {
	desk := tyde.Instance()
	if desk == nil {
		return fyne.Position{}, fyne.Size{}, false
	}
	screen := desk.Screens().Primary()
	if screen == nil {
		return fyne.Position{}, fyne.Size{}, false
	}
	scale := screen.CanvasScale()
	if scale == 0 {
		return fyne.Position{}, fyne.Size{}, false
	}

	x, y, w, h := desk.ContentBoundsPixels(screen)
	return fyne.NewPos(float32(x)/scale, float32(y)/scale),
		fyne.NewSize(float32(w)/scale, float32(h)/scale), true
}

func (p *panel) build() {
	rowObjects := make([]fyne.CanvasObject, 0, len(rows))
	for _, row := range rows {
		widths := make([]float32, len(row))
		keys := make([]fyne.CanvasObject, len(row))
		for i, def := range row {
			widths[i] = def.width
			button := newKeyButton(def, nil)
			button.OnTapped = func() { p.tapped(button.def) }

			p.keys = append(p.keys, button)
			keys[i] = button
		}
		rowObjects = append(rowObjects, container.New(&rowLayout{widths: widths}, keys...))
	}
	grid := container.New(layout.NewGridLayoutWithRows(len(rowObjects)), rowObjects...)

	p.flipBtn = &widget.Button{Importance: widget.LowImportance, OnTapped: p.flip}
	p.updateFlipIcon()
	closeBtn := &widget.Button{Icon: theme.CancelIcon(), Importance: widget.LowImportance,
		OnTapped: p.hide}
	p.header = container.NewHBox(layout.NewSpacer(), p.flipBtn, closeBtn)

	bg := canvas.NewRectangle(wmTheme.WidgetPanelBackground())
	p.content = container.NewStack(bg,
		container.NewPadded(container.NewBorder(p.header, nil, nil, nil, grid)))
	p.refreshFaces()
}
