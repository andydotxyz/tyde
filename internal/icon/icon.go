package icon

import (
	"runtime"

	"fyshos.com/fynedesk"
	"github.com/FyshOS/appie"
)

// FindAppFromWinInfo searches the known applications and tries to find one
// based on the properties of an open window.
func FindAppFromWinInfo(win fynedesk.Window, provider appie.Provider) appie.AppData {
	if runtime.GOOS == "darwin" { // simpler handling when we are not the desktop environment
		apps := provider.FindAppsMatching(win.Properties().Title())
		if len(apps) > 0 {
			return apps[0]
		}
		return nil
	}

	apps := provider.FindAppsMatching(win.Properties().Command())
	if len(apps) > 0 {
		return apps[0]
	}

	for _, class := range win.Properties().Class() {
		apps = provider.FindAppsMatching(class)
		if len(apps) > 0 {
			return apps[0]
		}
	}

	apps = provider.FindAppsMatching(win.Properties().IconName())
	if len(apps) > 0 {
		return apps[0]
	}
	return nil
}
