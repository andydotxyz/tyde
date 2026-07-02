package ai

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// TestWithCopyableCode confirms that code-block segments are swapped for the
// copyable variant and that the swapped segment builds a visual without panic.
func TestWithCopyableCode(t *testing.T) {
	test.NewApp()

	rt := widget.NewRichTextFromMarkdown("Here is code:\n\n```\nfmt.Println(\"hi\")\n```\n")
	withCopyableCode(rt)

	var found *copyableCodeSegment
	for _, seg := range rt.Segments {
		if _, ok := seg.(*widget.CodeBlockSegment); ok {
			t.Fatalf("a plain CodeBlockSegment survived the swap")
		}
		if c, ok := seg.(*copyableCodeSegment); ok {
			found = c
		}
	}
	if found == nil {
		t.Fatal("no copyable code segment produced")
	}
	if found.Text != "fmt.Println(\"hi\")" {
		t.Fatalf("unexpected code text %q", found.Text)
	}

	// Visual and a subsequent Update must not panic.
	vis := found.Visual()
	if vis == nil {
		t.Fatal("nil visual")
	}
	found.Update(vis)
}

// TestWithCopyableCodeNoBlocks leaves prose untouched.
func TestWithCopyableCodeNoBlocks(t *testing.T) {
	test.NewApp()

	rt := widget.NewRichTextFromMarkdown("Just some **prose** with no code.")
	withCopyableCode(rt)

	for _, seg := range rt.Segments {
		if _, ok := seg.(*copyableCodeSegment); ok {
			t.Fatal("prose should not gain a code segment")
		}
	}
}
