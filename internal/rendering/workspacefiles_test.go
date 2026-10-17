package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func renderDiff(t *testing.T, label, text string) string {
	t.Helper()
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{}, "Tallyville", true)
	var buf bytes.Buffer
	if err := diffView(label, text).Render(ctx, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

// A diff line is attacker-shaped text as far as the page is concerned: pasted YAML comes back inside it.
func TestDiff_escapesLinesAndLabel(t *testing.T) {
	html := renderDiff(t, "<i>a.yaml</i>", "-old\n+<b>new</b><script>alert(1)</script>\n")
	if strings.Contains(html, "<b>new</b>") || strings.Contains(html, "<script>") || strings.Contains(html, "<i>a.yaml</i>") {
		t.Error("a diff line or the label reached the page unescaped")
	}
}

// Added and removed lines draw in the good and bad tone; a hunk header in neither.
func TestDiff_colorsLinesByTheirFirstCharacter(t *testing.T) {
	html := renderDiff(t, "a.yaml", "@@ -1 +1 @@\n-old\n+new\n same\n")
	for _, want := range []string{"tone-good-bg", "tone-bad-bg"} {
		if !strings.Contains(html, want) {
			t.Errorf("no %s line in %s", want, html)
		}
	}
}

// A file that would not change is a sentence, not an empty box.
func TestDiff_aBlankTextSaysNothingWouldChange(t *testing.T) {
	html := renderDiff(t, "a.yaml", "  \n")
	if strings.Contains(html, "<pre") || !strings.Contains(html, "Nothing would change") {
		t.Errorf("a blank diff was not drawn as the sentence: %s", html)
	}
}
