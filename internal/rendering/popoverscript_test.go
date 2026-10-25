package rendering

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
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

// Opening one popover closes the others (2026-10-25): the shell's `toggle` listener closes every other open `details[data-popover]` that is
// neither the menu the new one came from nor something opened from it (a dialog inside a menu leaves the menu open). Text-pinned like the
// scripts above; the browser check is in the slice's record. `name=` on <details> is not used for this, since it would close the parent.
func TestShellScript_openingAPopoverClosesTheOthersButNotItsParent(t *testing.T) {
	var b bytes.Buffer
	if err := appShell("T", "W", Viewer{}, "").Render(assignedNavFixture(), &b); err != nil {
		t.Skipf("the shell does not render bare: %v", err)
	}
	out := b.String()
	for _, want := range []string{`if (o !== d && !o.contains(d) && !d.contains(o)) { o.open = false; }`, `document.querySelectorAll("details[data-popover][open]").forEach(function (o)`} {
		if !strings.Contains(out, want) {
			t.Errorf("the shell script no longer closes the other open popovers: it lacks %q", want)
		}
	}
}

// unnamedDisclosures is every hand-written <details> that belongs to no exclusive group, with why that is right: each is a disclosure that
// reveals more of one thing in place, and two of them open side by side is correct. Anything else that opens over or beside the page is a
// Popover (the shell closes the others) or shares a `name=` with its siblings (one editor open at a time). A count that falls fails too.
var unnamedDisclosures = map[string]int{
	"chooseworkspace.templ": 1, // the archived Workspaces list, collapsed by default
	"controls.templ":        1, // foldedRest: the tail of a long list
	"detail.templ":          1, // longText: "Read more"
	"disclosure.templ":      1, // the Disclosure Component: a panel the page declared
	"popover.templ":         1, // the Popover itself; the shell script is its group
}

func TestEveryDetailsClosesItsSiblingsOrIsADisclosure(t *testing.T) {
	files, err := filepath.Glob("*.templ")
	if err != nil || len(files) == 0 {
		t.Fatalf("no templates found: %v", err)
	}
	open := regexp.MustCompile(`(?m)^\s*<details\b[^>]*>?`)
	got := map[string]int{}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range open.FindAllString(string(src), -1) {
			if strings.Contains(m, `name="`) || (strings.Contains(m, "data-popover") && f != "popover.templ") {
				continue
			}
			got[f]++
		}
	}
	for f, n := range got {
		if unnamedDisclosures[f] != n {
			t.Errorf("%s draws %d <details> in no exclusive group, the list expects %d: give it name=\"...\" (an editor or a menu row) or a Popover, or list it as a disclosure with a reason", f, n, unnamedDisclosures[f])
		}
	}
	for f, n := range unnamedDisclosures {
		if got[f] != n {
			t.Errorf("%s is listed with %d unnamed <details> but draws %d: lower the entry", f, n, got[f])
		}
	}
}
