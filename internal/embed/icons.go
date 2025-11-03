package embed

import (
	"strings"

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
	for _, a := range e.apps {
		if a.Name() == appName {
			return a
		}
	}
	return nil
}

func (e *icons) FindAppsMatching(pattern string) []appie.AppData {
	var apps []appie.AppData

	for _, a := range e.apps {
		if strings.Contains(strings.ToLower(a.Name()), strings.ToLower(pattern)) {
			apps = append(apps, a)
		}
	}

	return apps
}

func (e *icons) DefaultApps() []appie.AppData {
	return e.apps
}

func (e *icons) CategorizedApps() map[string][]appie.AppData {
	return nil
}
