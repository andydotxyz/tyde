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

	history   []llms.MessageContent
	log       *fyne.Container
	scroll    *container.Scroll
	entry     *widget.Entry
	send      *widget.Button
	reasoning *widget.Check // Local AI: quick per-window speed/accuracy toggle
	busy      bool

	// gen identifies the current conversation; reset bumps it so a reply still
	// in flight from a previous one can't write into the new one. cancel stops
	// that in-flight request. Both are only touched on the UI thread.
	gen    int
	cancel context.CancelFunc
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

	// A compact reasoning toggle above the input makes it easy to trade speed for
	// accuracy per question. It shares the Local AI setting, and only applies to
	// that provider - syncControls shows/hides it and keeps it in step.
	c.reasoning = widget.NewCheck("Reasoning", nil)
	c.reasoning.SetChecked(fyne.CurrentApp().Preferences().Bool(prefLocalThinking))
	c.reasoning.OnChanged = func(on bool) {
		fyne.CurrentApp().Preferences().SetBool(prefLocalThinking, on)
	}

	input := container.NewBorder(nil, nil, nil, c.send, c.entry)
	controls := container.NewHBox(layout.NewSpacer(), c.reasoning)
	bottom := container.NewVBox(controls, container.NewPadded(input))
	body := container.NewStack(container.NewCenter(watermark), c.scroll)
	return container.NewBorder(nil, bottom, nil, nil, body)
}

// ask seeds the input with text and sends it, used for the launcher's initial
// question.
func (c *chatUI) ask(text string) {
	c.entry.SetText(text)
	c.submit()
}

// reset clears the transcript and conversation history back to a fresh start, so
// a question launched from a new search term is not appended onto a previous
// conversation. Any reply still streaming from the old conversation is cancelled
// and its late writes are ignored (see the gen check in submit). Runs on the UI
// thread (called from open, via the launcher tap).
func (c *chatUI) reset() {
	c.gen++
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	c.setBusy(false) // a superseded reply must not leave the input disabled

	c.history = []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, systemPrompt),
	}
	if c.log != nil {
		c.log.RemoveAll()
		c.log.Refresh()
	}
}

// syncControls refreshes the reasoning toggle from preferences and shows it only
// for Local AI (the only provider enable_thinking applies to). Called each time
// the window opens so it tracks changes made in settings or to the provider.
func (c *chatUI) syncControls() {
	if c.reasoning == nil {
		return
	}
	p := fyne.CurrentApp().Preferences()
	c.reasoning.SetChecked(p.Bool(prefLocalThinking))
	if p.StringWithFallback(prefProvider, ProviderClaude) == ProviderLocal {
		c.reasoning.Show()
	} else {
		c.reasoning.Hide()
	}
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

	reply, stopSpinner := c.addPendingReply()
	c.setBusy(true)

	// Snapshot the conversation and its generation, and make the request
	// cancellable, so a reset (new search term) can abandon this reply cleanly:
	// the generation check drops any late UI/history writes, and cancel stops the
	// model mid-flight instead of leaving it to run on into the next question.
	hist := c.history
	gen := c.gen
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel

	// The model call performs network IO - keep it off the render thread and
	// marshal only the UI updates back with fyne.Do. The whole thing is guarded:
	// this module runs inside the compositor process, so a panic here (a bad
	// stream from a local server, a render fault) must not crash the desktop.
	go func() {
		defer recoverAI("chat goroutine")
		defer cancel()
		var sb strings.Builder
		resp, genErr := llm.GenerateContent(
			ctx, hist,
			// Without an explicit limit langchaingo defaults max_tokens to 2048,
			// which truncates longer replies mid-sentence (stop_reason max_tokens).
			llms.WithMaxTokens(8192),
			llms.WithStreamingFunc(func(_ context.Context, chunk []byte) error {
				sb.Write(chunk)
				text := sb.String()
				// A local server can emit empty leading chunks (a role-only delta,
				// keep-alives) before any tokens - and the model may take a while to
				// load and start generating. Keep the activity indicator up until
				// real text actually arrives, or it vanishes into a blank pause.
				if text == "" {
					return nil
				}
				fyne.Do(func() {
					if c.gen != gen { // a reset superseded this reply
						return
					}
					defer recoverAI("chat stream render")
					stopSpinner() // first real reply text has arrived
					// Plain markdown while streaming: don't swap in copyable code
					// blocks on every chunk - re-parsing rebuilds the segments each
					// time, and churning their types under Fyne's renderer is both
					// wasteful and crash-prone. The copy buttons are added once the
					// reply is complete (final render below).
					reply.ParseMarkdown(text)
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
			if c.gen != gen { // a reset superseded this reply - leave the new one be
				return
			}
			defer recoverAI("chat final render")
			stopSpinner() // ensure it stops even if nothing streamed
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

			// Record the exchange on the UI thread (same goroutine as reset and
			// the human-turn append) so history has a single writer.
			if genErr == nil {
				c.history = append(c.history, llms.TextParts(llms.ChatMessageTypeAI, final))
			}
		})
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

// addPendingReply appends the assistant's reply row with a running activity
// indicator beside the empty body, and returns the body to stream into plus a
// stop func that removes the indicator.
func (c *chatUI) addPendingReply() (*widget.RichText, func()) {
	body := widget.NewRichTextFromMarkdown("")
	body.Wrapping = fyne.TextWrapWord

	spinner := widget.NewActivity()
	spinner.Start()

	content := container.NewVBox(container.NewHBox(spinner), body)
	row := container.NewBorder(nil, nil, container.NewVBox(widget.NewIcon(Icon), layout.NewSpacer()), nil, content)
	c.log.Add(container.NewPadded(row))
	c.log.Refresh()
	c.scroll.ScrollToBottom()

	stop := func() {
		spinner.Stop()
		spinner.Hide()
	}
	return body, stop
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
