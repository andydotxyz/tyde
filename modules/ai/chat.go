package ai

import (
	"context"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	wmtheme "fyshos.com/tyde/theme"

	"github.com/tmc/langchaingo/llms"
)

const systemPrompt = "You are a helpful AI assistant built into the FyshOS desktop. " +
	"Answer concisely and format replies in Markdown when it helps readability."

// chatUI is the conversation view: a scrolling transcript above an input row.
// history holds the running conversation passed to the model on each turn.
type chatUI struct {
	win fyne.Window

	history []llms.MessageContent
	log     *fyne.Container
	scroll  *container.Scroll
	entry   *widget.Entry
	send    *widget.Button
	busy    bool
}

func newChatUI() *chatUI {
	return &chatUI{
		history: []llms.MessageContent{
			llms.TextParts(llms.ChatMessageTypeSystem, systemPrompt),
		},
	}
}

// build assembles the chat window content.
func (c *chatUI) build() fyne.CanvasObject {
	c.log = container.NewVBox()
	c.scroll = container.NewVScroll(c.log)

	c.entry = widget.NewMultiLineEntry()
	c.entry.SetPlaceHolder("Ask the assistant...")
	c.entry.Wrapping = fyne.TextWrapWord
	c.entry.OnSubmitted = func(string) { c.submit() }

	c.send = &widget.Button{
		Icon: theme.MailSendIcon(), Importance: widget.HighImportance,
		OnTapped: c.submit,
	}

	// A large, faint assistant glyph sits behind the transcript for style and to
	// make the window's purpose clear at a glance.
	watermark := canvas.NewImageFromResource(Icon)
	watermark.FillMode = canvas.ImageFillContain
	watermark.Translucency = 0.9
	watermark.SetMinSize(fyne.NewSquareSize(240))

	input := container.NewBorder(nil, nil, nil, c.send, c.entry)
	body := container.NewStack(container.NewCenter(watermark), c.scroll)
	return container.NewBorder(nil, container.NewPadded(input), nil, nil, body)
}

// ask seeds the input with text and sends it, used for the launcher's initial
// question.
func (c *chatUI) ask(text string) {
	c.entry.SetText(text)
	c.submit()
}

// submit sends the current input to the model and streams the reply back.
func (c *chatUI) submit() {
	text := strings.TrimSpace(c.entry.Text)
	if text == "" || c.busy {
		return
	}

	c.entry.SetText("")
	c.addMessage(wmtheme.UserIcon, text)
	c.history = append(c.history, llms.TextParts(llms.ChatMessageTypeHuman, text))

	cfg := loadConfig()
	llm, err := cfg.newLLM()
	if err != nil {
		c.addMessage(theme.WarningIcon(), err.Error())
		return
	}

	reply := c.addMessage(Icon, "")
	c.setBusy(true)

	// The model call performs network IO - keep it off the render thread and
	// marshal only the UI updates back with fyne.Do.
	go func() {
		var sb strings.Builder
		resp, genErr := llm.GenerateContent(
			context.Background(), c.history,
			// Without an explicit limit langchaingo defaults max_tokens to 2048,
			// which truncates longer replies mid-sentence (stop_reason max_tokens).
			llms.WithMaxTokens(8192),
			llms.WithStreamingFunc(func(_ context.Context, chunk []byte) error {
				sb.Write(chunk)
				text := sb.String()
				fyne.Do(func() {
					renderMarkdown(reply, text)
					c.scroll.ScrollToBottom()
				})
				return nil
			}),
		)

		final := sb.String()
		if genErr == nil && final == "" && resp != nil && len(resp.Choices) > 0 {
			final = resp.Choices[0].Content // provider streamed nothing; use the full reply
		}

		// A "max_tokens" stop reason means the model hit the output cap and the
		// reply is incomplete - flag it so the reader knows to ask for more.
		truncated := genErr == nil && resp != nil && len(resp.Choices) > 0 &&
			resp.Choices[0].StopReason == "max_tokens"

		fyne.Do(func() {
			switch {
			case genErr != nil:
				reply.ParseMarkdown("⚠️ " + genErr.Error())
			case final != "":
				shown := final
				if truncated {
					shown += " …\n\n*(reply truncated - ask to continue)*"
				}
				renderMarkdown(reply, shown)
			}
			c.scroll.ScrollToBottom()
			c.setBusy(false)
			c.win.Canvas().Focus(c.entry)
		})

		if genErr == nil {
			c.history = append(c.history, llms.TextParts(llms.ChatMessageTypeAI, final))
		}
	}()
}

// renderMarkdown parses Markdown into the reply body and upgrades its code
// blocks to copyable ones, keeping the two steps together at every render site.
func renderMarkdown(body *widget.RichText, md string) {
	body.ParseMarkdown(md)
	withCopyableCode(body)
}

// addMessage appends a transcript row (an icon beside a Markdown body) and
// returns the body so a streaming reply can update it in place.
func (c *chatUI) addMessage(icon fyne.Resource, text string) *widget.RichText {
	body := widget.NewRichTextFromMarkdown(text)
	body.Wrapping = fyne.TextWrapWord

	row := container.NewBorder(nil, nil, container.NewVBox(widget.NewIcon(icon), layout.NewSpacer()), nil, body)
	c.log.Add(container.NewPadded(row))
	c.log.Refresh()
	c.scroll.ScrollToBottom()
	return body
}

// setBusy toggles the input while a reply is in flight.
func (c *chatUI) setBusy(busy bool) {
	c.busy = busy
	if busy {
		c.send.Disable()
	} else {
		c.send.Enable()
	}
}
