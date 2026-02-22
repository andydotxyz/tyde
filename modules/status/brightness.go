package status

import (
	"image/color"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/FyshOS/dryvers"

	"fyshos.com/fynedesk"
	wmtheme "fyshos.com/fynedesk/theme"
)

var brightnessMeta = fynedesk.ModuleMetadata{
	Name:        "Brightness",
	NewInstance: newBrightness,
}

type brightType int

const (
	noBacklight brightType = iota
	xbacklight
	brightnessctl
)

// Brightness is a progress bar module to modify screen brightness
type brightness struct {
	bright *dryvers.Brightness

	bar *widget.ProgressBar
}

func (b *brightness) Destroy() {
}

func (b *brightness) offsetValue(diff int) {
	floatVal, _ := b.bright.Get()
	if floatVal <= 0.01 { // don't start doing 6, 11 etc just because we were on 1 (min)
		floatVal = 0
	}
	value := int(floatVal*100) + diff

	b.setValue(value)
}

func (b *brightness) setValue(value int) {
	if value < 1 {
		value = 1
	} else if value > 100 {
		value = 100
	}

	_ = b.bright.Set(float64(value) / 100)

	newVal, _ := b.bright.Get()
	b.bar.SetValue(newVal)
}

func (b *brightness) LaunchSuggestions(input string) []fynedesk.LaunchSuggestion {
	if val, err := b.bright.Get(); err != nil || val == 0 {
		return nil // don't load if not present
	}

	lower := strings.ToLower(input)
	matches := false
	val := lower
	if startsWith(lower, "brightness ") {
		matches = true
		if len(lower) > 11 {
			val = lower[11:]
		} else {
			val = ""
		}
	} else if startsWith(lower, "bright ") {
		matches = true
		if len(lower) > 7 {
			val = lower[7:]
		} else {
			val = ""
		}
	} else if startsWith(lower, "backlight ") {
		matches = true
		if len(lower) > 10 {
			val = lower[10:]
		} else {
			val = ""
		}
	}

	if matches {
		return []fynedesk.LaunchSuggestion{&brightItem{input: val, b: b}}
	}

	return nil
}

func (b *brightness) Metadata() fynedesk.ModuleMetadata {
	return brightnessMeta
}

func (b *brightness) Shortcuts() map[*fynedesk.Shortcut]func() {
	return map[*fynedesk.Shortcut]func(){
		fynedesk.NewShortcut("Increase Screen Brightness", fynedesk.KeyBrightnessDown, fynedesk.AnyModifier): func() {
			b.offsetValue(-5)
		},
		fynedesk.NewShortcut("Reduce Screen Brightness", fynedesk.KeyBrightnessUp, fynedesk.AnyModifier): func() {
			b.offsetValue(5)
		},
	}
}

func (b *brightness) StatusAreaWidget() fyne.CanvasObject {
	if val, err := b.bright.Get(); err != nil || val == 0 {
		return nil
	}

	b.bar = widget.NewProgressBar()
	brightnessIcon := widget.NewIcon(wmtheme.BrightnessIcon)
	prop := canvas.NewRectangle(color.Transparent)
	prop.SetMinSize(brightnessIcon.MinSize().Add(fyne.NewSize(theme.Padding()*4, 0)))
	icon := container.NewCenter(prop, brightnessIcon)

	less := &widget.Button{Icon: theme.ContentRemoveIcon(), Importance: widget.LowImportance, OnTapped: func() {
		b.offsetValue(-5)
	}}

	more := &widget.Button{Icon: theme.ContentAddIcon(), Importance: widget.LowImportance, OnTapped: func() {
		b.offsetValue(5)
	}}

	bright := container.NewBorder(nil, nil, less, more, b.bar)

	go b.offsetValue(0)
	return container.New(&handleNarrow{}, icon, bright)
}

// newBrightness creates a new module that will show screen brightness in the status area
func newBrightness() fynedesk.Module {
	return &brightness{bright: dryvers.NewBrightness()}
}

type brightItem struct {
	input string
	b     *brightness
}

func (i *brightItem) Icon() fyne.Resource {
	return wmtheme.BrightnessIcon
}

func (i *brightItem) Title() string {
	if startsWith(i.input, "down") {
		return "Brightness down"
	} else if _, err := strconv.Atoi(i.input); err == nil {
		return "Brightness " + i.input + "%"
	}

	return "Brightness up"
}

func (i *brightItem) Launch() {
	if startsWith(i.input, "down") {
		i.b.offsetValue(-5)
	} else if val, err := strconv.Atoi(i.input); err == nil {
		i.b.setValue(val)
	}

	i.b.offsetValue(5)
}
