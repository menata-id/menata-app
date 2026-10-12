package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// handWrittenTonePairs freezes, per file, the string literals in `internal/rendering` that write a tone hue's tint and
// the same hue's ink by hand (2026-10-12, rencana 0.2c). A semantic tone reads `toneClass`/`toneFillClass`/
// `toneInkClass`, so a Workspace's `theme.tone` moves it. Seven sites moved byte-identically:
// - three of the approver chip's states;
// - the pending nav badge;
// - the archived-workspace banner;
// - Delete account's blocker list;
// - the truncation notice.
//
// **Read by meaning (menjaga-design-system R2).** Every entry below is a coloured pair whose meaning is not a tone,
// or whose value is not the tone's. Moving it would recolour a screen, and that is a decision (R1).
//
// It reads **every** string literal, not only class attributes, so a pair that a `.templ` helper returns from Go
// (`appIconClasses`, `diffLineClass`) is counted too. `layout.templ` holds the readers and is skipped. Blind spots: a
// pair split across two literals or two elements, a hue outside the five tone hues, and grey pairs (slate has no
// tone with the same steps, recorded in `domain.BackgroundRole`).
var handWrittenTonePairs = map[string]struct {
	n   int
	why string
}{
	"appshell.templ":       {1, "the error toast is a class string inside a <script>, built in the browser where no ctx reaches"},
	"reviewdocument.templ": {1, "the PDF file badge: red-100 is the file type's colour, not a bad state"},
	"workspacefiles.templ": {2, "added and removed lines of a diff (green and red -800): a diff's own colours, not tones"},
	"workspacehome.templ":  {3, "an Application's icon colour: a palette an author picks (domain.KnownApplicationColors), like Tag, not a tone"},
}

var (
	tonePairLiteral = regexp.MustCompile(`"([^"\n]*)"`)
	tonePairTint    = regexp.MustCompile(`^bg-(red|emerald|green|amber|blue)-(?:50|100)$`)
	tonePairInk     = regexp.MustCompile(`^text-(red|emerald|green|amber|blue)-[6-9]00$`)
)

func TestHandWrittenTonePairsOnlyShrink(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	actual := map[string]int{}
	readerPairs := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		n, half := countTonePairs(stripTemplComments(string(b)))
		for _, h := range half {
			t.Errorf("%s: %q reads a tone through the Theme and writes the other half of the pair by hand -- a Workspace "+
				"remapping the tone moves one half only; read both (toneClass, or toneFillClass and toneInkClass)", e.Name(), h)
		}
		if e.Name() == "layout.templ" {
			readerPairs = n
			continue
		}
		if n > 0 {
			actual[e.Name()] = n
		}
	}
	// The readers return the fill and the ink separately, so layout.templ holds no pair. This is a check on the
	// readers, not on the scan; the scan's own check is the len(actual) fatal below.
	if readerPairs != 0 {
		t.Errorf("layout.templ holds %d tint+ink pair(s) in one literal -- toneFillClass and toneInkClass answer each half", readerPairs)
	}
	var files []string
	seen := map[string]bool{}
	for f := range actual {
		seen[f] = true
	}
	for f := range handWrittenTonePairs {
		seen[f] = true
	}
	for f := range seen {
		files = append(files, f)
	}
	sort.Strings(files)
	if len(actual) == 0 {
		t.Fatal("no hand-written tone pair found anywhere -- either every one reads the Theme (keep this gate, empty) or the pattern stopped matching")
	}
	for _, f := range files {
		got, want := actual[f], handWrittenTonePairs[f].n
		switch {
		case got > want:
			t.Errorf("%s: %d hand-written tone pair(s), frozen at %d -- a semantic tone reads toneClass/toneFillClass/toneInkClass(ctx, domain.Tone*), "+
				"so theme.tone moves it; a colour pair that is not a tone goes in handWrittenTonePairs with what it is", f, got, want)
		case got < want:
			t.Errorf("%s: %d hand-written tone pair(s), down from %d -- lower handWrittenTonePairs to lock it in", f, got, want)
		}
	}
}

// countTonePairs counts hand-written pairs in one file. A class attribute counts the literals that each hold a pair, or
// one if the pair is only split across its literals (`"bg-red-50 p-3", "text-red-700"`, or two `templ.KV`s). Every
// other string literal, such as a Go helper's return, counts on its own. It also returns each class expression that
// pairs a tone reader with a literal tint or ink, which is a half migration. Splits and half migrations were
// surviving mutations in the 2026-10-12 independent review.
func countTonePairs(src string) (int, []string) {
	n := 0
	var half []string
	rest := classAnyAttr.ReplaceAllStringFunc(src, func(attr string) string {
		lits := tonePairLiteral.FindAllStringSubmatch(attr, -1)
		each, joined := 0, make([]string, 0, len(lits))
		for _, l := range lits {
			joined = append(joined, l[1])
			if holdsTonePair(l[1]) {
				each++
			}
		}
		switch {
		case each > 0:
			n += each
		case holdsTonePair(strings.Join(joined, " ")):
			n++
		}
		if toneReaderCall.MatchString(attr) {
			for _, tok := range strings.Fields(strings.Join(joined, " ")) {
				if tonePairTint.MatchString(tok) || tonePairInk.MatchString(tok) {
					half = append(half, strings.Join(strings.Fields(attr), " "))
					break
				}
			}
		}
		return ""
	})
	for _, m := range tonePairLiteral.FindAllStringSubmatch(rest, -1) {
		if holdsTonePair(m[1]) {
			n++
		}
	}
	return n, half
}

var classAnyAttr = regexp.MustCompile(`class=(?:"[^"]*"|\{[^}]*\})`)

// holdsTonePair reports whether one string holds a tint and an ink of the same tone hue. Variant-prefixed tokens are
// state and breakpoint, not the pair.
func holdsTonePair(s string) bool {
	tints, inks := map[string]bool{}, map[string]bool{}
	for _, tok := range strings.Fields(s) {
		if m := tonePairTint.FindStringSubmatch(tok); m != nil {
			tints[m[1]] = true
		}
		if m := tonePairInk.FindStringSubmatch(tok); m != nil {
			inks[m[1]] = true
		}
	}
	for h := range tints {
		if inks[h] {
			return true
		}
	}
	return false
}

// handWrittenToneEdges freezes the literal tone edges (`border-<hue>-200`) left after owner decision DS-D16
// (2026-10-12) made the edge part of the tone palette: `toneEdgeClass(ctx, domain.Tone*)`. Six edges moved
// byte-identically (the archived banner, the truncation notice, the leave guard's warning, the overdue approval card,
// two danger zones). Counted in every string literal outside layout.templ.
var handWrittenToneEdges = map[string]struct {
	n   int
	why string
}{
	"appshell.templ":       {1, "the error toast is a class string inside a <script>"},
	"notifications.templ":  {1, "an unread row's blue edge beside a /40 tint: an unread highlight, not a tone"},
	"reviewdocument.templ": {1, "the signed signature marker's emerald-300 edge: a marker state beside mark-good, counted since the 2026-10-12 review widened the gate to -300"},
}

var toneEdgeToken = regexp.MustCompile(`^border-(?:red|emerald|green|amber|blue)-[23]00$`)

func TestHandWrittenToneEdgesOnlyShrink(t *testing.T) {
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
				if toneEdgeToken.MatchString(tok) {
					actual[e.Name()]++
				}
			}
		}
	}
	files := map[string]bool{}
	for f := range actual {
		files[f] = true
	}
	for f := range handWrittenToneEdges {
		files[f] = true
	}
	if len(actual) == 0 {
		t.Fatal("no literal tone edge found -- every edge reads the Theme (keep this gate, empty) or the scan stopped matching")
	}
	for f := range files {
		switch got, want := actual[f], handWrittenToneEdges[f].n; {
		case got > want:
			t.Errorf("%s: %d literal tone edge(s), frozen at %d -- a tone's edge reads toneEdgeClass(ctx, domain.Tone*), so theme.tone moves it with its fill and ink", f, got, want)
		case got < want:
			t.Errorf("%s: %d literal tone edge(s), down from %d -- lower handWrittenToneEdges to lock it in", f, got, want)
		}
	}
}
