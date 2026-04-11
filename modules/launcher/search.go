package launcher

import (
	"net/url"
	"runtime/debug"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"

	"fyshos.com/fynedesk"
)

var searchMeta = fynedesk.ModuleMetadata{
	Name:        "Launcher: Web Search",
	NewInstance: newSearchSuggest,
}

type search struct{}

func (s *search) Destroy() {
}

func (s *search) LaunchSuggestions(input string) []fynedesk.LaunchSuggestion {
	if input == "" {
		return nil
	}
	return []fynedesk.LaunchSuggestion{&searchItem{text: input}}
}

func (s *search) Metadata() fynedesk.ModuleMetadata {
	return searchMeta
}

// isExpression will return true if input is a mathematical expression unless it just contains a number
func (s *search) isExpression(input string) bool {
	return true
}

// newCalcSuggest creates a new module that will show an option to search the web for an string.
func newSearchSuggest() fynedesk.Module {
	return &search{}
}

type searchItem struct {
	text string
}

func (s *searchItem) Icon() fyne.Resource {
	return theme.SearchIcon()
}

func (s *searchItem) Title() string {
	return "Search in Duck Duck Go"
}

func (s *searchItem) Launch() {
	enc := url.QueryEscape(s.text)
	u, err := url.Parse("https://duck.com/?q=" + enc)
	if err != nil {
		fyne.LogError("Failed to set up web search", err)
		return
	}

	debug.PrintStack()
	_ = fyne.CurrentApp().OpenURL(u)
}
