package ui

import "fyshos.com/fynedesk"

type embeddedScreensProvider struct {
	active  *fynedesk.Screen
	screens []*fynedesk.Screen
}

func (esp *embeddedScreensProvider) RefreshScreens() {
}

func (esp *embeddedScreensProvider) AddChangeListener(func()) {
	// no-op
}

func (esp *embeddedScreensProvider) Screens() []*fynedesk.Screen {
	s := esp.screens[0]
	w := fynedesk.Instance().Root()
	if w.Content() == nil { // during setup
		s.Width = 800
		s.Height = 600
	} else {
		s.Width = int(w.Content().Size().Width)
		s.Height = int(w.Content().Size().Height)
	}
	return esp.screens
}

func (esp *embeddedScreensProvider) SetActive(s *fynedesk.Screen) {
	esp.active = s
}

func (esp *embeddedScreensProvider) Active() *fynedesk.Screen {
	esp.Screens() // refresh
	return esp.active
}

func (esp *embeddedScreensProvider) Primary() *fynedesk.Screen {
	screens := esp.Screens()
	if len(screens) == 0 {
		return nil
	}
	return screens[0]
}

func (esp *embeddedScreensProvider) ScreenForWindow(win fynedesk.Window) *fynedesk.Screen {
	return esp.Screens()[0]
}

func (esp *embeddedScreensProvider) ScreenForGeometry(x int, y int, width int, height int) *fynedesk.Screen {
	return esp.Screens()[0]
}

// NewEmbeddedScreensProvider returns a screen provider for use in embedded desktop mode
func newEmbeddedScreensProvider() fynedesk.ScreenList {
	screen := &fynedesk.Screen{Name: "(Embedded)", X: 0, Y: 0, Width: 1280, Height: 1024, Scale: 1.0}
	return &embeddedScreensProvider{active: screen, screens: []*fynedesk.Screen{screen}}
}
