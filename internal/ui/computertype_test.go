package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/assert"

	"fyshos.com/tyde"
)

// TestComputerTypeChoice checks that the picker highlights the type in use and
// that choosing another one moves the highlight and reports the new type.
func TestComputerTypeChoice(t *testing.T) {
	test.NewApp()

	chosen := ""
	buttons := computerButtons(t, newComputerTypeChoice(tyde.ComputerLaptop, func(kind string) {
		chosen = kind
	}))
	assert.Len(t, buttons, len(computerTypes))

	for _, button := range buttons {
		if button.Text == tyde.ComputerLaptop {
			assert.Equal(t, widget.HighImportance, button.Importance)
		} else {
			assert.Equal(t, widget.LowImportance, button.Importance)
		}
	}

	for _, button := range buttons {
		if button.Text != tyde.ComputerTablet {
			continue
		}

		button.Tapped(nil)
	}
	assert.Equal(t, tyde.ComputerTablet, chosen)

	for _, button := range buttons {
		if button.Text == tyde.ComputerTablet {
			assert.Equal(t, widget.HighImportance, button.Importance)
		} else {
			assert.Equal(t, widget.LowImportance, button.Importance)
		}
	}
}

// computerButtons pulls the choice buttons out of the container built by
// newComputerTypeChoice.
func computerButtons(t *testing.T, o fyne.CanvasObject) []*widget.Button {
	t.Helper()

	box, ok := o.(*fyne.Container)
	if !ok {
		t.Fatalf("expected a container, got %T", o)
	}
	grid, ok := box.Objects[1].(*fyne.Container)
	if !ok {
		t.Fatalf("expected a button grid, got %T", box.Objects[1])
	}

	var buttons []*widget.Button
	for _, obj := range grid.Objects {
		button, ok := obj.(*widget.Button)
		if !ok {
			t.Fatalf("expected a button, got %T", obj)
		}
		buttons = append(buttons, button)
	}
	return buttons
}
