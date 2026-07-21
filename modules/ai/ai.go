// Package ai adds an AI assistant to Tyde. It is a launch-suggestion module:
// typing in the launcher offers an "Ask AI" option at the end of the
// suggestions which opens a chat window backed by an LLM. Anthropic (Claude)
// and OpenAI are cloud providers reached with an API key; "Local AI" talks to
// an OpenAI-compatible server running on the machine (ollama, the Kronk model
// server, LM Studio, llama.cpp's server, ...) so nothing but the desktop's own
// HTTP request leaves the box. All three go through the langchaingo library and
// are selected and configured from the module's settings panel.
//
// The module is off by default - enable it and pick a provider (adding a key
// for the cloud ones, or a server URL for Local AI) from the "Set up AI" step
// of the first-run setup, or from the AI tab of the settings window.
package ai

import (
	_ "embed"
	"fmt"
	"net/http"
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

// Supported providers. Claude and OpenAI are cloud services reached over the
// network with an API key; Local AI is an OpenAI-compatible server the user
// runs on the machine, so it needs a base URL instead of a key.
const (
	ProviderClaude = "Claude"
	ProviderOpenAI = "OpenAI"
	ProviderLocal  = "Local AI"
)

// Preference keys. Cloud keys are stored per-provider so switching provider
// does not discard the other provider's token; Local AI stores its server URL.
const (
	prefProvider      = "ai.provider"
	prefClaudeKey     = "ai.key.claude"
	prefOpenAIKey     = "ai.key.openai"
	prefLocalEndpoint = "ai.endpoint.local"
	prefLocalThinking = "ai.local.thinking" // let a reasoning model think (slower)
	prefModel         = "ai.model"
)

// Default models for the cloud providers. Claude defaults to Sonnet 4.6 rather
// than a newer Opus/Sonnet 5 model because langchaingo (v0.1.14) always sends a
// "temperature" field, which the Opus 4.7+/Sonnet 5 family reject with a 400 -
// Sonnet 4.6 still accepts it. Users can override either in settings.
const (
	defaultClaudeModel = "claude-sonnet-4-6"
	defaultOpenAIModel = "gpt-4o"
)

// Local AI depends on which server we expect. With the kronk binary present
// tyde runs and owns a kronk server (fixed port 11435; models named by Hugging
// Face GGUF id) - so there is no URL to configure, just a model. Otherwise we
// assume the user runs their own server, defaulting to ollama (port 11434; its
// own model names) but allowing any OpenAI-compatible server via the Base URL.
// Endpoints include the "/v1" prefix because langchaingo appends
// "/chat/completions".
const (
	kronkEndpoint  = "http://localhost:11435/v1"
	kronkModel     = "unsloth/Qwen3-0.6B-Q8_0"
	ollamaEndpoint = "http://localhost:11434/v1"
	ollamaModel    = "llama3.2"
)

// defaultLocalModel picks the kronk or ollama model so a beginner with either
// installed works with no configuration.
func defaultLocalModel() string {
	if kronkAvailable() {
		return kronkModel
	}
	return ollamaModel
}

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
	// If Local AI is the active provider, start a managed kronk server now (when
	// the module loads) so it is ready by the time the user asks something. A
	// no-op unless kronk is installed and nothing already answers.
	if cfg := loadConfig(); cfg.provider == ProviderLocal {
		serverMgr.ensure()
	}
	return &assistant{}
}

func (a *assistant) Metadata() tyde.ModuleMetadata {
	return meta
}

func (a *assistant) Destroy() {
	// The module cache is torn down and rebuilt on any settings change, not just
	// when the assistant is switched off - so only stop the managed local server
	// when the module is genuinely no longer enabled, otherwise an unrelated
	// tweak would needlessly bounce (and re-download into) the server.
	if !moduleEnabled() {
		serverMgr.stop()
	}
	if a.win != nil {
		a.win.Close()
		a.win = nil
	}
}

// moduleEnabled reports whether the AI Assistant is still in the user's enabled
// module list (preference "modulenames", "|"-separated - the same store the
// settings screen writes). Read at Destroy time to tell a real deactivation
// from a routine module-cache rebuild.
func moduleEnabled() bool {
	names := fyne.CurrentApp().Preferences().String("modulenames")
	for _, n := range strings.Split(names, "|") {
		if n == ModuleName {
			return true
		}
	}
	return false
}

// LaunchSuggestions offers an "Ask AI" item whenever the launcher has input and
// an API key is configured - without a key the assistant can't answer, so the
// suggestion is hidden until it's set up.
func (a *assistant) LaunchSuggestions(input string) []tyde.LaunchSuggestion {
	if strings.TrimSpace(input) == "" {
		return nil
	}
	if !loadConfig().ready() {
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
		win.SetCloseIntercept(win.Hide) // reuse the window (hide, don't destroy)
		a.chat.win = win
		a.win = win
	}

	a.chat.syncControls() // reflect current provider and reasoning setting
	a.win.Show()
	a.win.RequestFocus()
	if strings.TrimSpace(prompt) != "" {
		a.chat.reset() // a new search term starts a fresh conversation
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
	key      string // API key for a cloud provider
	endpoint string // server base URL for Local AI
	thinking bool   // Local AI: let a reasoning model think before answering
}

// loadConfig reads the current AI configuration from app preferences.
func loadConfig() config {
	p := fyne.CurrentApp().Preferences()
	prov := p.StringWithFallback(prefProvider, ProviderClaude)

	c := config{provider: prov, model: strings.TrimSpace(p.String(prefModel))}
	switch prov {
	case ProviderOpenAI:
		c.key = p.String(prefOpenAIKey)
	case ProviderLocal:
		// A managed kronk server lives at a fixed address we own; only a
		// user-run server needs a configurable URL.
		if kronkAvailable() {
			c.endpoint = kronkEndpoint
		} else {
			c.endpoint = strings.TrimSpace(p.StringWithFallback(prefLocalEndpoint, ollamaEndpoint))
		}
		c.thinking = p.Bool(prefLocalThinking)
	default:
		c.key = p.String(prefClaudeKey)
	}
	return c
}

// ready reports whether the configured provider can answer: cloud providers
// need an API key, Local AI needs a server URL (which defaults, so it is
// effectively always ready). It gates the "Ask AI" launcher suggestion.
func (c config) ready() bool {
	if c.provider == ProviderLocal {
		return c.endpoint != ""
	}
	return strings.TrimSpace(c.key) != ""
}

// modelName resolves the model to use, falling back to the provider default.
func (c config) modelName() string {
	if c.model != "" {
		return c.model
	}
	switch c.provider {
	case ProviderOpenAI:
		return defaultOpenAIModel
	case ProviderLocal:
		return defaultLocalModel()
	default:
		return defaultClaudeModel
	}
}

// newLLM builds a langchaingo model for the configured provider, or an error
// describing what the user still needs to set up.
func (c config) newLLM() (llms.Model, error) {
	if !c.ready() {
		if c.provider == ProviderLocal {
			return nil, fmt.Errorf("no Local AI server URL set - add one in the AI settings")
		}
		return nil, fmt.Errorf("no %s API key set - add one in the AI settings", c.provider)
	}

	switch c.provider {
	case ProviderOpenAI:
		return openai.New(openai.WithToken(c.key), openai.WithModel(c.modelName()))
	case ProviderLocal:
		// A local OpenAI-compatible server (ollama, Kronk, ...). It ignores the
		// token but langchaingo requires a non-empty one, so pass a placeholder.
		// The injectDoer sets "enable_thinking" per the user's Reasoning toggle:
		// off (default) makes a reasoning model answer directly and fast; on lets
		// it think first - slower but more accurate. Harmless to servers that
		// don't use the field.
		llm, err := openai.New(
			openai.WithBaseURL(c.endpoint),
			openai.WithToken("local"),
			openai.WithModel(c.modelName()),
			openai.WithHTTPClient(injectDoer{
				fields: map[string]any{"enable_thinking": c.thinking},
				next:   http.DefaultClient,
			}),
		)
		if err != nil {
			return nil, err
		}
		// Wrap so a server that isn't up / crashed gives an actionable error.
		return localModel{Model: llm}, nil
	default:
		return anthropic.New(anthropic.WithToken(c.key), anthropic.WithModel(c.modelName()))
	}
}
