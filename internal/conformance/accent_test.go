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
	"login.templ":             {3, "pre-auth: no Workspace on ctx, no theme reaches it"},
	"register.templ":          {1, "pre-auth: no Workspace on ctx, no theme reaches it"},
	"machine.templ":           {1, "the Tag palette's blue entry (a colour an author picks, not the accent)"},
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
				if tok == accentInkToken {
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
			t.Errorf("%s: %d literal %s, frozen at %d -- a link or accent text reads inkClass(ctx, domain.InkAccent) (and accentHoverClass for its hover), so theme.ink.accent moves it", f, got, accentInkToken, want)
		case got < want:
			t.Errorf("%s: %d literal %s, down from %d -- lower handWrittenAccentInks to lock it in", f, got, accentInkToken, want)
		}
	}
}
