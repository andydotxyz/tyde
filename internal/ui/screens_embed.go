package ui

import "fyshos.com/tyde"

type embeddedScreensProvider struct {
	active  *tyde.Screen
	screens []*tyde.Screen
}

func (esp *embeddedScreensProvider) RefreshScreens() {
}

func (esp *embeddedScreensProvider) AddChangeListener(func()) {
	// no-op
}

func (esp *embeddedScreensProvider) Screens() []*tyde.Screen {
	return esp.screens
}

func (esp *embeddedScreensProvider) SetActive(s *tyde.Screen) {
	esp.active = s
}

func (esp *embeddedScreensProvider) Active() *tyde.Screen {
	return esp.active
}

func (esp *embeddedScreensProvider) Primary() *tyde.Screen {
	return esp.Screens()[0]
}

func (esp *embeddedScreensProvider) ScreenForWindow(win tyde.Window) *tyde.Screen {
	return esp.Screens()[0]
}

func (esp *embeddedScreensProvider) ScreenForGeometry(x int, y int, width int, height int) *tyde.Screen {
	return esp.Screens()[0]
}

// UpdatePrimarySize updates the virtual screen dimensions to match the window size.
func (esp *embeddedScreensProvider) UpdatePrimarySize(w, h int) {
	s := esp.Primary()
	s.Width = w
	s.Height = h
}

// newEmbeddedScreensProvider returns a screen provider for use in embedded desktop mode
func newEmbeddedScreensProvider() tyde.ScreenList {
	screen := &tyde.Screen{Name: "(Embedded)", X: 0, Y: 0, Width: 1280, Height: 1024, Scale: 1.0}
	return &embeddedScreensProvider{active: screen, screens: []*tyde.Screen{screen}}
}
