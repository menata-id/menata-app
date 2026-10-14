package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// emptyStateExempt is every hand-written dashed, centred box that is not the EmptyState Component, with its reason and
// count. Closed both ways: a count that drifts either way fails, and an entry for a file that has none fails too.
var emptyStateExempt = map[string]struct {
	n   int
	why string
}{
	// Workspace Home's two cards: a title and a description inside the box, roomier (`px-6 py-12`) than the one-message
	// card's `p-8`. Both sit on one screen, so a second input and a second padding would be a shape for one case. They
	// wait for another screen that needs a title and a description; folding the padding into `p-8` is a visible change.
	"workspacehome.templ": {2, "title and description in a roomier box, on one screen only"},
}

// TestNoHandWrittenEmptyState is a zero-floor gate beside `EmptyState`, the twelfth registered Component (rencana 2.3c,
// 2026-10-13; build, migrate, then gate). A list that holds nothing says so with `@emptyState(text)`, so the corner, the
// edge, the fill, the size and the ink follow one contract and a Workspace's `theme:` moves every one at once.
//
// It counts a class attribute holding `border-dashed` **and** `text-center`, read after `joinClassExpressions` so a box
// whose corner is a Theme reader is still seen. The renderer (`emptystate.templ`) is the one file not scanned.
//
// **What it cannot see, stated:** an empty state drawn without a dashed edge (a plain `<p>` or a `staticText`, which is
// how a declared `Collection` says `empty:`), a box whose class arrives from a Go function, and a dashed centred box
// that is not an empty state (none exists today). The population this replaces was counted by *reading* each of the 13
// `border-dashed` class sets: only four were "nothing here"; the rest were the review screen's signature boxes, a signature canvas and
// markers, an "Add workspace" card, a pending avatar and draft tiles.
func TestNoHandWrittenEmptyState(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	classAttr := regexp.MustCompile(`class="([^"]*)"`)
	has := func(set, tok string) bool {
		return regexp.MustCompile(`(?:^|\s)` + regexp.QuoteMeta(tok) + `(?:\s|$)`).MatchString(set)
	}
	scanned := 0
	seen := map[string]int{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") || e.Name() == "emptystate.templ" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		for _, m := range classAttr.FindAllStringSubmatch(joinClassExpressions(stripTemplComments(string(b))), -1) {
			scanned++
			if has(m[1], "border-dashed") && has(m[1], "text-center") {
				seen[e.Name()]++
			}
		}
	}
	if scanned == 0 {
		t.Fatal("no class attribute was scanned -- the gate is reading the wrong text")
	}
	files := make([]string, 0, len(seen))
	for f := range seen {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		if want := emptyStateExempt[f].n; seen[f] != want {
			t.Errorf("%s: %d hand-written dashed empty-state box(es), exempt %d -- a list that holds nothing reads @emptyState(text), so its "+
				"corner, edge, fill, size and ink follow one contract; a box an EmptyState must not absorb goes in emptyStateExempt with its reason", f, seen[f], want)
		}
	}
	for f, ex := range emptyStateExempt {
		if seen[f] == 0 {
			t.Errorf("emptyStateExempt[%q] = %d but the file has none -- a closed exemption must match exactly; drop it", f, ex.n)
		}
		if strings.TrimSpace(ex.why) == "" {
			t.Errorf("emptyStateExempt names %s with no reason", f)
		}
	}
}
