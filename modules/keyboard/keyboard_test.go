package keyboard

import (
	"testing"

	"fyne.io/fyne/v2"
	"github.com/stretchr/testify/assert"

	"fyshos.com/tyde"
	wmTest "fyshos.com/tyde/test"
)

func newTestModule(t *testing.T) *module {
	t.Helper()

	tyde.SetInstance(wmTest.NewDesktop())
	newSenderFunc = func() (sender, error) { return &recorder{}, nil }
	t.Cleanup(func() {
		tyde.SetInstance(nil)
		newSenderFunc = newSender
	})

	return newKeyboard().(*module)
}

func TestModule_Metadata(t *testing.T) {
	m := newTestModule(t)
	assert.Equal(t, ModuleName, m.Metadata().Name)
}

func TestModule_IsRegistered(t *testing.T) {
	for _, mod := range tyde.AvailableModules() {
		if mod.Name == ModuleName {
			return
		}
	}
	t.Errorf("%q is not one of the available modules", ModuleName)
}

func TestModule_Shortcut(t *testing.T) {
	m := newTestModule(t)

	shortcuts := m.Shortcuts()
	if assert.Len(t, shortcuts, 1) {
		for s := range shortcuts {
			assert.Equal(t, fyne.KeyK, s.KeyName)
			assert.Equal(t, "Show Virtual Keyboard", s.Name)
		}
	}
}

// The keyboard is off screen until it is asked for - a keyboard that is always
// up is one that covers whatever is being typed into.
func TestModule_StartsHidden(t *testing.T) {
	m := newTestModule(t)

	assert.False(t, m.panel.shown)
	assert.NotNil(t, m.StatusAreaWidget())
}

func TestModule_StatusAreaWidgetToggles(t *testing.T) {
	m := newTestModule(t)

	icon := m.StatusAreaWidget().(interface{ Tapped(*fyne.PointEvent) })
	icon.Tapped(nil)
	assert.True(t, m.panel.shown)

	icon.Tapped(nil)
	assert.False(t, m.panel.shown)
}

func TestModule_ShortcutToggles(t *testing.T) {
	m := newTestModule(t)

	for _, action := range m.Shortcuts() {
		action()
		assert.True(t, m.panel.shown)
		action()
		assert.False(t, m.panel.shown)
	}
}

func TestModule_DestroyHidesKeyboard(t *testing.T) {
	m := newTestModule(t)
	m.ShowKeyboard()

	m.Destroy()
	assert.False(t, m.panel.shown)
}
