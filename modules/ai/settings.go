package ai

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// SettingsContent builds the provider/token configuration panel for the AI
// assistant. Changes are persisted immediately to app preferences so they take
// effect the next time the assistant is opened. The panel is embedded both in
// the settings window's AI tab and in the first-run "Set up AI" step.
func SettingsContent() fyne.CanvasObject {
	p := fyne.CurrentApp().Preferences()

	provider := widget.NewSelect([]string{ProviderClaude, ProviderOpenAI}, nil)
	provider.SetSelected(p.StringWithFallback(prefProvider, ProviderClaude))

	key := widget.NewPasswordEntry()
	key.SetPlaceHolder("API key / token")

	model := widget.NewEntry()
	model.SetText(p.String(prefModel))

	// syncProvider reloads the key field and the model placeholder for the
	// currently selected provider.
	syncProvider := func() {
		if provider.Selected == ProviderOpenAI {
			key.SetText(p.String(prefOpenAIKey))
			model.SetPlaceHolder(defaultOpenAIModel)
		} else {
			key.SetText(p.String(prefClaudeKey))
			model.SetPlaceHolder(defaultClaudeModel)
		}
	}
	syncProvider()

	provider.OnChanged = func(string) {
		p.SetString(prefProvider, provider.Selected)
		syncProvider()
	}
	key.OnChanged = func(s string) {
		if provider.Selected == ProviderOpenAI {
			p.SetString(prefOpenAIKey, s)
		} else {
			p.SetString(prefClaudeKey, s)
		}
	}
	model.OnChanged = func(s string) {
		p.SetString(prefModel, s)
	}

	form := widget.NewForm(
		widget.NewFormItem("Provider", provider),
		widget.NewFormItem("API Key", key),
		widget.NewFormItem("Model", model),
	)

	help := widget.NewLabel("Your key is stored on this device and sent only to the chosen provider.\n" +
		"Leave Model blank to use the provider's default.")
	help.Wrapping = fyne.TextWrapWord

	return container.NewVBox(form, help)
}
