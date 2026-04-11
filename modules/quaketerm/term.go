package launcher

import (
	_ "embed"
	"image/color"
	"time"

	"fyshos.com/fynedesk"
	wmTheme "fyshos.com/fynedesk/theme"
	"github.com/fyne-io/terminal"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
)

const (
	delay  = time.Second / 25
	height = 240
	step   = 40
)

var termMeta = fynedesk.ModuleMetadata{
	Name:        "Terminal Overlay",
	NewInstance: newTerm,
}

//go:embed terminal.svg
var resourceTerminalSvgData []byte

var resourceTerminal = &fyne.StaticResource{
	StaticName:    "terminal.svg",
	StaticContent: resourceTerminalSvgData,
}

type term struct {
	shown   bool
	running bool
	content fyne.CanvasObject
	console *terminal.Terminal
}

func (t *term) Destroy() {
}

func (t *term) Metadata() fynedesk.ModuleMetadata {
	return termMeta
}

func (t *term) Shortcuts() map[*fynedesk.Shortcut]func() {
	return map[*fynedesk.Shortcut]func(){
		{Name: "Open Terminal Overlay", KeyName: fyne.KeyBackTick, Modifier: fynedesk.UserModifier}: func() {
			t.toggle()
		},
	}
}

func (t *term) createTerm() {
	bg := canvas.NewRectangle(withTransparency(theme.Color(theme.ColorNameOverlayBackground)))
	img := canvas.NewImageFromResource(theme.NewThemedResource(resourceTerminal))
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(200, 200))
	over := canvas.NewRectangle(wmTheme.WidgetPanelBackground())
	matchTheme(bg, over)

	t.console = terminal.New()
	t.content = container.NewStack(img, bg, over, t.console)
}

func (t *term) hide() {
	var y float32
	end := -float32(height)
	for y > end {
		currY := y
		fyne.Do(func() {
			t.content.Move(fyne.NewPos(0, currY))
		})
		time.Sleep(delay)
		y -= step
	}

	t.shown = false
	fynedesk.Instance().HideOverlay(t.content)
}

func (t *term) show() {
	screen := fynedesk.Instance().Screens().Primary()
	scale := screen.CanvasScale()
	w := float32(screen.Width) / scale
	y := -float32(height)
	var end float32
	size := fyne.NewSize(w, height)

	fynedesk.Instance().ShowOverlay(t.content, size, fyne.NewPos(0, y))

	if !t.running {
		t.running = true
		go func() {
			err := t.console.RunLocalShell()
			if err != nil {
				fyne.LogError("Failed to open terminal", err)
			}
			t.running = false
			if t.shown {
				t.hide()
			}
			t.createTerm() // reset for next usage
		}()
	}

	for y < end {
		currY := y
		fyne.Do(func() {
			t.content.Resize(size)
			t.content.Move(fyne.NewPos(0, currY))
		})
		time.Sleep(delay)
		y += step
	}
	fyne.Do(func() {
		t.content.Move(fyne.NewPos(0, end))
		fynedesk.Instance().Root().Canvas().Focus(t.console)
	})
	t.shown = true
}

func (t *term) toggle() {
	if t.content == nil {
		t.createTerm()
	}

	if !t.shown {
		go t.show()
	} else {
		go t.hide()
	}
}

func matchTheme(bg, over *canvas.Rectangle) {
	fyne.CurrentApp().Settings().AddListener(func(_ fyne.Settings) {
		bg.FillColor = withTransparency(theme.Color(theme.ColorNameOverlayBackground))
		bg.Refresh()
		over.FillColor = wmTheme.WidgetPanelBackground()
		over.Refresh()
	})
}

func withTransparency(c color.Color) color.Color {
	r, g, b, _ := c.RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 0x99}
}

func newTerm() fynedesk.Module {
	return &term{}
}
