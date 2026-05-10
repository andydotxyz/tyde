package launcher

import (
	"net/url"
	"strings"

	"fyne.io/fyne/v2"

	"fyshos.com/tyde"
	wmTheme "fyshos.com/tyde/theme"
)

var urlMeta = tyde.ModuleMetadata{
	Name:        "Launcher: Open URLs",
	NewInstance: newURLs,
}

type urls struct{}

func (u *urls) Destroy() {
}

func (u *urls) LaunchSuggestions(input string) []tyde.LaunchSuggestion {
	if u.isURL(input) {
		return []tyde.LaunchSuggestion{&urlResult{input}}
	}

	return nil
}

func (u *urls) Metadata() tyde.ModuleMetadata {
	return urlMeta
}

func (u *urls) isURL(input string) bool {
	isHTTP := strings.Index(input, "http://") == 0 || strings.Index(input, "https://") == 0
	dotPos := strings.Index(input, ".")

	return isHTTP && dotPos != -1 && dotPos <= len(input)-3
}

// newURLs creates a new module that will show URLs in the launcher suggestions
func newURLs() tyde.Module {
	return &urls{}
}

type urlResult struct {
	url string
}

func (r *urlResult) Icon() fyne.Resource {
	return wmTheme.InternetIcon
}

func (r *urlResult) Title() string {
	return r.url
}

func (r *urlResult) Launch() {
	u, err := url.Parse(r.url)
	if err != nil {
		fyne.LogError("Could not parse URL", err)
		return
	}

	_ = fyne.CurrentApp().OpenURL(u)
}
