package ai

import (
	"context"
	"net/url"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// cloudHelp sits beneath the cloud-provider form.
const cloudHelp = "Your key is stored on this device and sent only to the chosen provider.\n" +
	"Leave Model blank to use the provider's default."

// SettingsContent builds the provider configuration panel for the AI assistant.
// Changes are persisted immediately to app preferences so they take effect the
// next time the assistant is opened. The panel is embedded both in the settings
// window's AI tab and in the first-run "Set up AI" step.
//
// Each provider needs different setup - a cloud key, or a local server URL - so
// the area below the provider selector is rebuilt for the chosen provider
// rather than relabelled in place (Fyne forms don't update item labels reliably
// on refresh).
func SettingsContent() fyne.CanvasObject {
	p := fyne.CurrentApp().Preferences()

	provider := widget.NewSelect([]string{ProviderClaude, ProviderOpenAI, ProviderLocal}, nil)
	provider.SetSelected(p.StringWithFallback(prefProvider, ProviderClaude))

	body := container.NewStack()

	rebuild := func() {
		switch provider.Selected {
		case ProviderOpenAI:
			body.Objects = []fyne.CanvasObject{cloudSettings(p, prefOpenAIKey, defaultOpenAIModel)}
		case ProviderLocal:
			body.Objects = []fyne.CanvasObject{localSettings(p)}
		default:
			body.Objects = []fyne.CanvasObject{cloudSettings(p, prefClaudeKey, defaultClaudeModel)}
		}
		body.Refresh()
	}
	rebuild()

	provider.OnChanged = func(string) {
		p.SetString(prefProvider, provider.Selected)
		if provider.Selected == ProviderLocal {
			// Switching to Local while the module is loaded: get the managed
			// server coming up so it's ready by the time they ask something.
			serverMgr.ensure()
		}
		rebuild()
	}

	head := widget.NewForm(widget.NewFormItem("Provider", provider))
	return container.NewVBox(head, body)
}

// cloudSettings builds the API-key + model form for a cloud provider.
func cloudSettings(p fyne.Preferences, keyPref, defaultModel string) fyne.CanvasObject {
	key := widget.NewPasswordEntry()
	key.SetPlaceHolder("API key / token")
	key.SetText(p.String(keyPref))
	key.OnChanged = func(s string) { p.SetString(keyPref, s) }

	model := widget.NewEntry()
	model.SetText(p.String(prefModel))
	model.SetPlaceHolder(defaultModel)
	model.OnChanged = func(s string) { p.SetString(prefModel, s) }

	help := widget.NewLabel(cloudHelp)
	help.Wrapping = fyne.TextWrapWord

	return container.NewVBox(widget.NewForm(
		widget.NewFormItem("API Key", key),
		widget.NewFormItem("Model", model),
	), help)
}

// localSettings builds the Local AI panel. When kronk is installed tyde owns
// the server, so the panel is as simple as the old embedded setup: just a Model
// field (leave blank for the default) plus a Test button - no URL to configure.
// Without kronk the user runs their own server, so a Base URL is needed and the
// guidance points them at ollama.
func localSettings(p fyne.Preferences) fyne.CanvasObject {
	model := widget.NewEntry()
	model.SetText(p.String(prefModel))
	model.SetPlaceHolder(defaultLocalModel())
	model.OnChanged = func(s string) { p.SetString(prefModel, s) }

	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord

	// Reasoning toggle: off (default) makes a reasoning model answer directly and
	// fast; on lets it think first - slower but more accurate. Applies to the
	// next message, no restart needed.
	reasoning := widget.NewCheck("Reasoning: slower, but more accurate", nil)
	reasoning.SetChecked(p.Bool(prefLocalThinking))
	reasoning.OnChanged = func(on bool) { p.SetBool(prefLocalThinking, on) }

	if kronkAvailable() {
		// Managed server: tyde knows the address, so the user only picks a model.
		note := widget.NewLabel("Kronk is installed - tyde runs a local server for you and stops it when the " +
			"assistant is turned off. Just choose a Model (or leave blank for the default); it downloads on " +
			"first use, which can take a while.")
		note.Wrapping = fyne.TextWrapWord

		test := probeButton(func() string { return kronkEndpoint }, model, status)
		return container.NewVBox(
			widget.NewForm(widget.NewFormItem("Model", model)),
			reasoning,
			container.NewHBox(test), status,
			widget.NewSeparator(), note,
		)
	}

	// User-run server: needs an address.
	endpoint := widget.NewEntry()
	endpoint.SetText(p.StringWithFallback(prefLocalEndpoint, ollamaEndpoint))
	endpoint.SetPlaceHolder(ollamaEndpoint)
	endpoint.OnChanged = func(s string) { p.SetString(prefLocalEndpoint, s) }

	test := probeButton(func() string {
		if ep := strings.TrimSpace(endpoint.Text); ep != "" {
			return ep
		}
		return ollamaEndpoint
	}, model, status)

	return container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("Base URL", endpoint),
			widget.NewFormItem("Model", model),
		),
		reasoning,
		container.NewHBox(test), status,
		widget.NewSeparator(), ollamaGuide(),
	)
}

// probeButton makes a "Test connection" button that checks the server at
// endpointFn() and reports whether it (and the chosen model) is ready.
func probeButton(endpointFn func() string, model *widget.Entry, status *widget.Label) *widget.Button {
	return widget.NewButton("Test connection", func() {
		ep := endpointFn()
		want := strings.TrimSpace(model.Text)
		if want == "" {
			want = defaultLocalModel()
		}
		status.SetText("Testing " + ep + " …")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			defer cancel()
			res := probeEndpoint(ctx, ep)
			msg := describeProbe(res, want)
			fyne.Do(func() { status.SetText(msg) })
		}()
	})
}

// ollamaGuide explains how to stand up a server when tyde can't manage one, and
// offers a shortcut to the ollama download page.
func ollamaGuide() fyne.CanvasObject {
	lbl := widget.NewLabel("Local AI needs an OpenAI-compatible server on this machine. The easiest is ollama: " +
		"install it, then run  ollama pull " + defaultLocalModel() + "  and press Test. Any such server works - " +
		"set the Base URL to match (e.g. the Kronk model server, LM Studio, llama.cpp).")
	lbl.Wrapping = fyne.TextWrapWord

	getOllama := widget.NewButton("Get ollama", func() {
		if u, err := url.Parse("https://ollama.com/download"); err == nil {
			_ = fyne.CurrentApp().OpenURL(u)
		}
	})

	return container.NewVBox(lbl, container.NewHBox(getOllama))
}

// describeProbe turns a connection test into a one-line status a beginner can act on.
func describeProbe(res probeResult, model string) string {
	if res.err != nil {
		return "No server responded (" + res.err.Error() + ").\nStart a local server, then Test again."
	}
	if res.has(model) {
		return "Connected. Model '" + model + "' is ready."
	}
	if len(res.models) == 0 {
		return "Connected, but no models are loaded yet. Pull '" + model + "' on the server, " +
			"or wait for its first-use download."
	}
	return "Connected, but '" + model + "' isn't available. The server offers: " +
		strings.Join(res.models, ", ") + ".\nSet Model to one of those."
}
