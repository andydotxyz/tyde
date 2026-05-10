package test

import "fyshos.com/tyde"

type testScreensProvider struct {
	screens []*tyde.Screen
	primary *tyde.Screen
	active  *tyde.Screen
}

// NewScreensProvider returns a simple screen manager for the specified screens
func NewScreensProvider(screens ...*tyde.Screen) tyde.ScreenList {
	if screens == nil {
		screens = []*tyde.Screen{{Name: "Screen0", X: 0, Y: 0, Width: 2000, Height: 1000, Scale: 1.0}}
	}
	return &testScreensProvider{screens: screens, active: screens[0], primary: screens[0]}
}

func (tsp *testScreensProvider) RefreshScreens() {
}

func (tsp *testScreensProvider) AddChangeListener(func()) {
	// no-op
}

func (tsp *testScreensProvider) Screens() []*tyde.Screen {
	return tsp.screens
}

func (tsp *testScreensProvider) SetActive(s *tyde.Screen) {
	tsp.active = s
}

func (tsp *testScreensProvider) Active() *tyde.Screen {
	return tsp.active
}

func (tsp *testScreensProvider) Primary() *tyde.Screen {
	return tsp.primary
}

func (tsp *testScreensProvider) Scale() float32 {
	return 1.0
}

func (tsp *testScreensProvider) ScreenForWindow(win tyde.Window) *tyde.Screen {
	return tsp.Screens()[0]
}

func (tsp *testScreensProvider) ScreenForGeometry(x int, y int, width int, height int) *tyde.Screen {
	return tsp.Screens()[0]
}
