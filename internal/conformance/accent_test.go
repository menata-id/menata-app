package conformance

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// accentInkToken is `ink.accent`'s default class, shared with G1 so its probe tests what this gate counts.
const accentInkToken = "text-blue-600"

// accentHoverToken is the accent's hover default. A literal one beside a read accent hovers blue under any theme, a
// shape the 2026-10-12 review spliced in unseen while the gate counted only the ink.
const accentHoverToken = "hover:text-blue-700"

// handWrittenAccentInks freezes, per file, the literal `text-blue-600` left after owner decision DS-D10 (2026-10-12)
// gave links and accent text the `ink.accent` role. 42 sites moved, and they read `inkClass(ctx, domain.InkAccent)`
// and `accentHoverClass(ctx)`. The diff over 106 pages was identical except `&` → `&amp;` inside one arbitrary-variant
// class on two pages, which is the same DOM. Counted in every string literal, so a Go helper's class is seen.
// layout.templ holds the reader and is skipped.
var handWrittenAccentInks = map[string]struct {
	n   int
	why string
}{
	"appmembers.templ":        {1, "hovers to blue-800, two steps darker than accent-hover; folding the two link hovers is a decision not taken (recorded as an open finding)"},
	"appsettings.templ":       {2, "hovers to blue-800; see appmembers"},
	"rolematrix.templ":        {2, "hovers to blue-800; see appmembers"},
	"workspacefiles.templ":    {3, "hovers to blue-800; see appmembers"},
	"workspacesettings.templ": {1, "hovers to blue-800; see appmembers"},
	"chooseworkspace.templ":   {1, "pre-auth: no Workspace on ctx, no theme reaches it"},
	"login.templ":             {6, "pre-auth: no Workspace on ctx, no theme reaches it"},
	"register.templ":          {2, "pre-auth: no Workspace on ctx, no theme reaches it"},
	"machine.templ":           {1, "the Tag palette's blue entry (a colour an author picks, not the accent)"},
	"appshell.templ":          {2, "state markers kept literal on purpose: the active bottom tab (desktop nav marks the active item text-blue-700, a different step) and the current-Workspace check; moving one without the other made the two navs disagree under a theme (2026-10-12 review), so active-nav colour is decided together, later"},
	"reviewdocument.templ":    {1, "the signature marker's \"yours\" state colour, paired with border-blue-500"},
}

func TestHandWrittenAccentInksOnlyShrink(t *testing.T) {
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
				if tok == accentInkToken || tok == accentHoverToken {
					actual[e.Name()]++
				}
			}
		}
	}
	if len(actual) == 0 {
		t.Fatal("no literal accent found anywhere -- either every link reads the Theme (keep this gate, empty) or the scan stopped matching")
	}
	seen := map[string]bool{}
	for f := range actual {
		seen[f] = true
	}
	for f := range handWrittenAccentInks {
		seen[f] = true
	}
	var files []string
	for f := range seen {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		switch got, want := actual[f], handWrittenAccentInks[f].n; {
		case got > want:
			t.Errorf("%s: %d literal accent ink or hover, frozen at %d -- a link or accent text reads inkClass(ctx, domain.InkAccent) (and accentHoverClass for its hover), so theme.ink.accent moves it", f, got, want)
		case got < want:
			t.Errorf("%s: %d literal accent ink or hover, down from %d -- lower handWrittenAccentInks to lock it in", f, got, want)
		}
	}
}
