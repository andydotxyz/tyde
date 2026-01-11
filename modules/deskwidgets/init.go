package deskwidgets

import "fyshos.com/fynedesk"

var widgetsMeta = fynedesk.ModuleMetadata{
	Name:        "Desktop Widgets",
	NewInstance: newDesktopWidgets,
}

func init() {
	fynedesk.RegisterModule(widgetsMeta)
}
