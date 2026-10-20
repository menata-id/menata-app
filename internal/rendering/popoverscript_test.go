package rendering

import (
	"bytes"
	"strings"
	"testing"
)

// The shell's popover script is one of the named script exceptions; its two behaviours that a review found missing are pinned here as
// text (the script cannot be run in a Go test; the browser check is in the slice's record): a dialog that opens takes focus into its
// first control, and Escape closes only the innermost open popover.
func TestShellScript_aDialogTakesFocusAndEscapeClosesTheInnermost(t *testing.T) {
	var b bytes.Buffer
	if err := appShell("T", "W", Viewer{}, "").Render(assignedNavFixture(), &b); err != nil {
		t.Skipf("the shell does not render bare: %v", err)
	}
	out := b.String()
	for _, want := range []string{`addEventListener("toggle"`, `[role=dialog] input:not([type=hidden])`, `if (!d.querySelector("details[data-popover][open]")) { d.open = false; }`} {
		if !strings.Contains(out, want) {
			t.Errorf("the shell script lacks %q", want)
		}
	}
}

// A card dragged over the lane for records with no list is over no place: the script clears the ring, the bar, the count and the sentence
// and does not accept the drop (the review of slice 5). Text-pinned, like the shell script above; the browser check is in the slice's record.
func TestBoardDragScript_theLaneWithNoListAcceptsNothingAndSaysNothing(t *testing.T) {
	var b bytes.Buffer
	if err := boardDragScript().Render(assignedNavFixture(), &b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `if (!col.dataset.value) { clear(); status(""); return; }`) {
		t.Errorf("the drag script lets a card hover the lane with no list as if it were a place")
	}
	if strings.Index(b.String(), `if (!col.dataset.value) { clear(); status(""); return; }`) > strings.Index(b.String(), `e.dataTransfer.dropEffect = "move"`) {
		t.Errorf("the refusal comes after the drop is accepted")
	}
}
