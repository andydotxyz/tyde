package ai

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// copyableCodeSegment wraps Fyne's CodeBlockSegment so the rendered code panel
// carries a copy button in its top-right corner. Fyne draws code blocks as
// non-selectable monospace panels, so overlaying a copy button is the only way
// to let the user lift the code back out.
type copyableCodeSegment struct {
	widget.CodeBlockSegment
}

// Visual builds the code panel with the copy button floating over it.
func (s *copyableCodeSegment) Visual() fyne.CanvasObject {
	return newCopyableCodeBlock(&s.CodeBlockSegment)
}

// Update refreshes the panel from the segment's current text - used as the
// streaming reply grows the block in place. The object handed back by Fyne can
// be one created for a different segment type at this position (the reply is
// re-parsed on every streamed chunk, so a slot can change type between paints);
// in that case skip the in-place update rather than crash on a bad type
// assertion - Fyne rebuilds it from Visual on the next cycle.
func (s *copyableCodeSegment) Update(o fyne.CanvasObject) {
	if cb, ok := o.(*copyableCodeBlock); ok {
		cb.update(&s.CodeBlockSegment)
	}
}

// copyableCodeBlock stacks Fyne's own code-block visual under a small copy
// button anchored to the top-right.
type copyableCodeBlock struct {
	widget.BaseWidget

	seg  *widget.CodeBlockSegment // source of the current text (for the clipboard)
	code fyne.CanvasObject        // Fyne's monospace code panel
}

func newCopyableCodeBlock(seg *widget.CodeBlockSegment) *copyableCodeBlock {
	c := &copyableCodeBlock{seg: seg, code: seg.Visual()}
	c.ExtendBaseWidget(c)
	return c
}

// update points the block at the latest segment and refreshes the code text in
// place, reusing the existing monospace panel.
func (c *copyableCodeBlock) update(seg *widget.CodeBlockSegment) {
	c.seg = seg
	seg.Update(c.code)
	c.Refresh()
}

func (c *copyableCodeBlock) CreateRenderer() fyne.WidgetRenderer {
	copyBtn := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		fyne.CurrentApp().Clipboard().SetContent(c.seg.Text)
	})
	copyBtn.Importance = widget.LowImportance

	// Pin the button to the top-right: leading spacer pushes it right, the
	// trailing spacer keeps it at the top, and it floats over the code panel.
	corner := container.NewVBox(
		container.NewHBox(layout.NewSpacer(), container.NewPadded(copyBtn)),
		layout.NewSpacer(),
	)
	return widget.NewSimpleRenderer(container.NewStack(c.code, corner))
}

// withCopyableCode replaces every plain code-block segment produced by
// ParseMarkdown with a copyable one, then refreshes if anything changed. Call
// it after each ParseMarkdown on the reply body.
func withCopyableCode(rt *widget.RichText) {
	changed := false
	for i, seg := range rt.Segments {
		if cb, ok := seg.(*widget.CodeBlockSegment); ok {
			repl := &copyableCodeSegment{}
			repl.Text = cb.Text
			rt.Segments[i] = repl
			changed = true
		}
	}
	if changed {
		rt.Refresh()
	}
}
