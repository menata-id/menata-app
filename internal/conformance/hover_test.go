package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// handWrittenHoverWashes freezes the literal hover fills left after owner decision DS-D11 (2026-10-12): hover lives in
// recipes. A row, menu row or secondary button reads `hoverWashClass(ctx)`, the Design System's `hover-wash`, an
// alias of `bg-base`, so it follows `background.base`. A danger control reads `toneHoverFillClass(ctx, domain.ToneBad)`.
// 38 hovers moved byte-identically (35 washes and 3 danger fills; 106 pages). `hover:bg-slate-100` is a darker hover with another job and is not
// counted here (recorded as an open finding). Counted in every string literal outside layout.templ.
var handWrittenHoverWashes = map[string]struct {
	n   int
	why string
}{}

// hoverWashToken is a hover fill the recipes own: the wash (slate-50) and every tone's -50 fill, with any variant
// prefix (sm:, group-) and opacity. Blind spots: a class in a backtick or single-quoted string.
var hoverWashToken = regexp.MustCompile(`^(?:[\w-]+:)*(?:group-)?hover:bg-(?:slate|red|amber|emerald|blue)-50(?:/\d+)?$`)

func TestHandWrittenHoverWashesOnlyShrink(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	actual := map[string]int{}
	readers := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") || e.Name() == "layout.templ" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		src := stripTemplComments(string(b))
		readers += strings.Count(src, "hoverWashClass(ctx)") + strings.Count(src, "toneHoverFillClass(ctx,")
		for _, m := range tonePairLiteral.FindAllStringSubmatch(src, -1) {
			for _, tok := range strings.Fields(m[1]) {
				if hoverWashToken.MatchString(tok) {
					actual[e.Name()]++
				}
			}
		}
	}
	// The sanity check counts the readers, not the forbidden token, so the gate can reach zero (the trap
	// TestNoHandWrittenDividedRow records: a check on the very token being forbidden fails the day the gate succeeds).
	if readers == 0 {
		t.Fatal("no hoverWashClass/toneHoverFillClass call found -- the scan stopped matching")
	}
	seen := map[string]bool{}
	for f := range actual {
		seen[f] = true
	}
	for f := range handWrittenHoverWashes {
		seen[f] = true
	}
	var files []string
	for f := range seen {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		switch got, want := actual[f], handWrittenHoverWashes[f].n; {
		case got > want:
			t.Errorf("%s: %d literal hover wash(es), frozen at %d -- a row or secondary button hovers with hoverWashClass(ctx), a danger control with toneHoverFillClass(ctx, domain.ToneBad) (owner decision DS-D11)", f, got, want)
		case got < want:
			t.Errorf("%s: %d literal hover wash(es), down from %d -- lower handWrittenHoverWashes to lock it in", f, got, want)
		}
	}
}
