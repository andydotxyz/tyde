package rpc

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"fyshos.com/tyde"
	wmTest "fyshos.com/tyde/test"

	"github.com/stretchr/testify/assert"
)

func TestSocketPath_XDGRuntime(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	assert.Equal(t, "/run/user/1000/fynedesk.sock", SocketPath())
}

func TestSocketPath_Fallback(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	expected := "/tmp/fynedesk-" + strconv.Itoa(os.Getuid()) + ".sock"
	assert.Equal(t, expected, SocketPath())
}

func TestRPCModule_Metadata(t *testing.T) {
	m := &rpcModule{}
	assert.Equal(t, "RPC", m.Metadata().Name)
}

func TestRPCModule_Destroy_Empty(t *testing.T) {
	m := &rpcModule{}
	m.Destroy() // listener nil, should not panic
}

func TestLaunchInput_NoDesktop(t *testing.T) {
	tyde.SetInstance(nil)
	_, err := LaunchInput("anything")
	assert.EqualError(t, err, "desktop not running")
}

func TestService_Launch_NoDesktop(t *testing.T) {
	tyde.SetInstance(nil)
	var reply string
	err := (&Service{}).Launch("anything", &reply)
	assert.EqualError(t, err, "desktop not running")
}

func TestService_ListModules_NoDesktop(t *testing.T) {
	tyde.SetInstance(nil)
	var reply []string
	err := (&Service{}).ListModules("", &reply)
	assert.EqualError(t, err, "desktop not running")
}

// fakeSuggestModule is a LaunchSuggestionModule used to test the rpc service.
type fakeSuggestModule struct {
	name        string
	suggestions []tyde.LaunchSuggestion
}

func (f *fakeSuggestModule) Destroy() {}

func (f *fakeSuggestModule) Metadata() tyde.ModuleMetadata {
	return tyde.ModuleMetadata{Name: f.name}
}

func (f *fakeSuggestModule) LaunchSuggestions(_ string) []tyde.LaunchSuggestion {
	return f.suggestions
}

func newDeskWithModules(mods []tyde.Module) *wmTest.Desktop {
	d := wmTest.NewDesktop()
	d.SetModules(mods)
	return d
}

func TestService_ListModules(t *testing.T) {
	mods := []tyde.Module{
		&fakeSuggestModule{name: "Launcher: Calculate"},
		&fakeSuggestModule{name: "Launcher: Open URLs"},
		&fakeSuggestModule{name: "RPC"},
	}
	tyde.SetInstance(newDeskWithModules(mods))
	t.Cleanup(func() { tyde.SetInstance(nil) })

	var reply []string
	err := (&Service{}).ListModules("", &reply)
	assert.NoError(t, err)
	assert.Equal(t, []string{"Calculate", "Open URLs", "RPC"}, reply)
}

func TestLaunchInput_NoMatch(t *testing.T) {
	mods := []tyde.Module{
		&fakeSuggestModule{name: "Launcher: Calculate"},
	}
	tyde.SetInstance(newDeskWithModules(mods))
	t.Cleanup(func() { tyde.SetInstance(nil) })

	_, err := LaunchInput("xyz")
	if assert.Error(t, err) {
		assert.True(t, strings.HasPrefix(err.Error(), "no match for "))
	}
}

func TestLaunchInput_SkipsSearchModule(t *testing.T) {
	// "Search" module produces a suggestion but should be skipped (catch-all).
	mods := []tyde.Module{
		&fakeSuggestModule{
			name:        "Launcher: Search",
			suggestions: []tyde.LaunchSuggestion{&fakeSuggestion{title: "search-result"}},
		},
	}
	tyde.SetInstance(newDeskWithModules(mods))
	t.Cleanup(func() { tyde.SetInstance(nil) })

	_, err := LaunchInput("anything")
	assert.Error(t, err) // no non-search match
}

type fakeSuggestion struct {
	title string
}

func (f *fakeSuggestion) Icon() fyne.Resource { return nil }
func (f *fakeSuggestion) Title() string       { return f.title }
func (f *fakeSuggestion) Launch()             {}
