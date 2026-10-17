package conformance

import (
	"strings"
	"testing"
)

// TestQuietDestructiveActionsShareOneLook is a zero-floor gate for owner decision T-D (2026-10-14). A text action that
// destroys something but is rare and quiet (Delete this group, Discard this conversation, Revoke) rests in the
// secondary ink and turns to the danger ink on hover, at the caption size. Before the decision four of them differed:
// Delete this group rested red and underlined on hover, and Revoke was one size larger. A `<button>` therefore may not
// rest in the danger ink with an underline on hover, and one that turns to the danger ink on hover must not set a
// literal `text-sm`. Read on the opening tag's own text, so a class from a Go function is not seen (stated, not hidden).
func TestQuietDestructiveActionsShareOneLook(t *testing.T) {
	seen := 0
	sources := renderingTemplSources(t)
	for _, name := range sortedFileNames(sources) {
		for _, tag := range buttonOpenTags(sources[name]) {
			if !strings.Contains(tag, "toneInkHoverClass(ctx, domain.ToneBad)") {
				if strings.Contains(tag, "toneInkClass(ctx, domain.ToneBad)") && strings.Contains(tag, "hover:underline") {
					t.Errorf("%s: a button resting in the danger ink with an underline on hover -- a quiet destructive action rests secondary and turns red on hover (T-D): %.100q", name, strings.Join(strings.Fields(tag), " "))
				}
				continue
			}
			seen++
			if strings.Contains(tag, "text-sm") {
				t.Errorf("%s: a quiet destructive action sets text-sm -- the caption size is text-xs (T-D): %.100q", name, strings.Join(strings.Fields(tag), " "))
			}
		}
	}
	if seen < 3 {
		t.Fatalf("only %d buttons turning to the danger ink on hover were found -- the scan is measuring less than it appears to", seen)
	}
}
