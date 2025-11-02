package embed

import (
	"github.com/FyshOS/appie"

	"fyne.io/fyne/v2/container"
)

type icons struct {
	multi *container.MultipleWindows

	apps []appie.AppData
}

func NewIcons(multi *container.MultipleWindows) appie.Provider {
	return &icons{multi: multi,
		apps: []appie.AppData{
			newClock(multi),
			newTicTacToe(multi),
		}}
}

func (e *icons) AvailableApps() []appie.AppData {
	return e.apps
}

func (e *icons) AvailableThemes() []string {
	return nil
}

func (e *icons) ClearCache() {
}

func (e *icons) FindAppFromName(appName string) appie.AppData {
	if appName == "FyneTerm" {
		return e.apps[1]
	}
	return e.apps[0] // TODO search
}

func (e *icons) FindAppsMatching(pattern string) []appie.AppData {
	// TODO search
	return nil
}

func (e *icons) DefaultApps() []appie.AppData {
	return e.apps
}

func (e *icons) CategorizedApps() map[string][]appie.AppData {
	return nil
}
