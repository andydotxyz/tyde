// Package ai adds an AI assistant to Tyde. It is a launch-suggestion module:
// typing in the launcher offers an "Ask AI" option at the end of the
// suggestions which opens a chat window backed by an LLM. Both Anthropic
// (Claude) and OpenAI are supported through the langchaingo library, selected
// and configured (provider + API key) from the module's settings panel.
//
// The module is off by default - enable it and add a key from the "Set up AI"
// step of the first-run setup, or from the AI tab of the settings window.
package ai

import (
	_ "embed"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyshos.com/tyde/modules/launcher"

	"fyshos.com/tyde"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/tmc/langchaingo/llms/openai"
)

// ModuleName is the registered name of the AI assistant module. It is exported
// so the desktop can identify the module (e.g. to order its launcher
// suggestion after the web search item).
const ModuleName = "AI Assistant"

// Supported providers.
const (
	ProviderClaude = "Claude"
	ProviderOpenAI = "OpenAI"
)

// Preference keys. Keys are stored per-provider so switching provider does not
// discard the other provider's token.
const (
	prefProvider  = "ai.provider"
	prefClaudeKey = "ai.key.claude"
	prefOpenAIKey = "ai.key.openai"
	prefModel     = "ai.model"
)

// Default models per provider. Claude defaults to Sonnet 4.6 rather than a
// newer Opus/Sonnet 5 model because langchaingo (v0.1.14) always sends a
// "temperature" field, which the Opus 4.7+/Sonnet 5 family reject with a 400 -
// Sonnet 4.6 still accepts it. Users can override the model in settings.
const (
	defaultClaudeModel = "claude-sonnet-4-6"
	defaultOpenAIModel = "gpt-4o"
)

//go:embed assistant.svg
var assistantSvg []byte

// Icon is the assistant's themed icon, exported for reuse in the settings and
// setup UIs so the module and its configuration screens share one glyph.
var Icon fyne.Resource = theme.NewThemedResource(&fyne.StaticResource{
	StaticName:    "assistant.svg",
	StaticContent: assistantSvg,
})

var meta = tyde.ModuleMetadata{
	Name:        ModuleName,
	NewInstance: newAI,
}

// assistant is the module instance. It owns a single chat window that is reused
// (hidden rather than destroyed) across launches so the conversation persists.
type assistant struct {
	win  fyne.Window
	chat *chatUI
}

func newAI() tyde.Module {
	return &assistant{}
}

func (a *assistant) Metadata() tyde.ModuleMetadata {
	return meta
}

func (a *assistant) Destroy() {
	if a.win != nil {
		a.win.Close()
		a.win = nil
	}
}

// LaunchSuggestions offers an "Ask AI" item whenever the launcher has input and
// an API key is configured - without a key the assistant can't answer, so the
// suggestion is hidden until it's set up.
func (a *assistant) LaunchSuggestions(input string) []tyde.LaunchSuggestion {
	if strings.TrimSpace(input) == "" {
		return nil
	}
	if strings.TrimSpace(loadConfig().key) == "" {
		return nil
	}
	return []tyde.LaunchSuggestion{&launchItem{a: a, text: input}}
}

// open shows the chat window (creating it on first use) and, if given, asks an
// initial question. Must be called on the UI thread - it is, via the launcher
// button tap.
func (a *assistant) open(prompt string) {
	if a.win == nil {
		a.chat = newChatUI()
		win := fyne.CurrentApp().NewWindow("AI Assistant")
		win.SetIcon(Icon)
		win.SetContent(a.chat.build())
		win.Resize(fyne.NewSize(440, 540))
		win.SetCloseIntercept(win.Hide) // keep the conversation for next time
		a.chat.win = win
		a.win = win
	}

	a.win.Show()
	a.win.RequestFocus()
	if strings.TrimSpace(prompt) != "" {
		a.chat.ask(prompt)
	}
}

// launchItem is the "Ask AI" launcher suggestion.
type launchItem struct {
	a    *assistant
	text string
}

func (i *launchItem) Icon() fyne.Resource { return Icon }

func (i *launchItem) Title() string { return "Ask AI: " + launcher.TruncatePrompt(i.text) }

func (i *launchItem) Launch() { i.a.open(i.text) }

// config is a snapshot of the persisted AI settings.
type config struct {
	provider string
	model    string // optional override; empty means use the provider default
	key      string // API key for the active provider
}

// loadConfig reads the current AI configuration from app preferences.
func loadConfig() config {
	p := fyne.CurrentApp().Preferences()
	prov := p.StringWithFallback(prefProvider, ProviderClaude)

	c := config{provider: prov, model: strings.TrimSpace(p.String(prefModel))}
	if prov == ProviderOpenAI {
		c.key = p.String(prefOpenAIKey)
	} else {
		c.key = p.String(prefClaudeKey)
	}
	return c
}

// modelName resolves the model to use, falling back to the provider default.
func (c config) modelName() string {
	if c.model != "" {
		return c.model
	}
	if c.provider == ProviderOpenAI {
		return defaultOpenAIModel
	}
	return defaultClaudeModel
}

// newLLM builds a langchaingo model for the configured provider, or an error
// describing what the user still needs to set up.
func (c config) newLLM() (llms.Model, error) {
	if strings.TrimSpace(c.key) == "" {
		return nil, fmt.Errorf("no %s API key set - add one in the AI settings", c.provider)
	}

	if c.provider == ProviderOpenAI {
		return openai.New(openai.WithToken(c.key), openai.WithModel(c.modelName()))
	}
	return anthropic.New(anthropic.WithToken(c.key), anthropic.WithModel(c.modelName()))
}
