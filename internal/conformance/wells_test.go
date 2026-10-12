package conformance

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The background roles' defaults below `raised`, shared with G1 so its probe tests what this gate counts.
const (
	baseFillToken   = "bg-slate-50"
	sunkenFillToken = "bg-slate-100"
)

// handWrittenWellFills freezes the literal `bg-slate-50` / `bg-slate-100` left after owner decision W1 (2026-10-12):
// a well follows its colour, so a slate-50 well reads `backgroundClass(ctx, domain.BackgroundBase)` and a slate-100
// one `BackgroundSunken`. 13 moved byte-identically (106 pages and component renders): table header bands, info wells,
// a file card, an image frame, menu icon wells, and two progress tracks. What stays is the same grey doing another
// job (R2). Counted in every string literal outside layout.templ.
var handWrittenWellFills = map[string]struct {
	n   int
	why string
}{
	"appshell.templ":        {7, "seven h-px menu dividers: a rule drawn as a 1px fill, not a surface (the divider role is an edge)"},
	"account.templ":         {2, "the read-only email box and a disabled button: a disabled control's look"},
	"approvalinbox.templ":   {1, "the waiting approver chip's fill (approverChipTone): a chip, with no tone of its steps"},
	"approvalstepper.templ": {1, "a waiting step's circle: a marker, not a surface"},
	"authshell.templ":       {1, "the pre-auth <body>: no Workspace on ctx, no theme reaches it"},
	"machine.templ":         {1, "the board's selected view tab: a state, not a surface"},
	"newapplication.templ":  {1, "the assistant's chat bubble: its own shape, not a well"},
	"reviewdocument.templ":  {2, "the reference <code> chip and a waiting signature marker"},
	"workspacehome.templ":   {2, "an Application's default icon colour (a palette) and a suggestion chip"},
}

func TestHandWrittenWellFillsOnlyShrink(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	actual := map[string]int{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") || e.Name() == "layout.templ" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range tonePairLiteral.FindAllStringSubmatch(stripTemplComments(string(b)), -1) {
			for _, tok := range strings.Fields(m[1]) {
				if tok == baseFillToken || tok == sunkenFillToken {
					actual[e.Name()]++
				}
			}
		}
	}
	if len(actual) == 0 {
		t.Fatal("no literal well fill found -- every well reads the Theme (keep this gate, empty) or the scan stopped matching")
	}
	seen := map[string]bool{}
	for f := range actual {
		seen[f] = true
	}
	for f := range handWrittenWellFills {
		seen[f] = true
	}
	var files []string
	for f := range seen {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		switch got, want := actual[f], handWrittenWellFills[f].n; {
		case got > want:
			t.Errorf("%s: %d literal %s/%s, frozen at %d -- a well reads backgroundClass(ctx, domain.BackgroundBase|BackgroundSunken) by its colour (owner decision W1); the same grey doing another job goes in handWrittenWellFills with what it is", f, got, baseFillToken, sunkenFillToken, want)
		case got < want:
			t.Errorf("%s: %d literal well fill(s), down from %d -- lower handWrittenWellFills to lock it in", f, got, want)
		}
	}
}
