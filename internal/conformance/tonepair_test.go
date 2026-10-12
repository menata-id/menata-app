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
	"approvalinbox.templ":   {2, "the urgent SLA pill: overdue is red (a bad pair), due-today is amber-700 where warn's ink is amber-800; the pill stays whole until the amber is decided (the artifact folded amber ink to tone-warn-ink in the mockups)"},
	"approvalstepper.templ": {1, "a finished step's check circle is blue; reading `info` (in progress) would mislabel it and reading `good` would recolour it"},
	"appshell.templ":        {1, "the error toast is a class string inside a <script>, built in the browser where no ctx reaches"},
	"reviewdocument.templ":  {1, "the PDF file badge: red-100 is the file type's colour, not a bad state"},
	"workspacefiles.templ":  {2, "added and removed lines of a diff (green and red -800): a diff's own colours, not tones"},
	"workspacehome.templ":   {3, "an Application's icon colour: a palette an author picks (domain.KnownApplicationColors), like Tag, not a tone"},
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
	comment := regexp.MustCompile(`(?m)^\s*//.*$`)
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
		src := comment.ReplaceAllString(string(b), "")
		for _, m := range tonePairLiteral.FindAllStringSubmatch(src, -1) {
			if !holdsTonePair(m[1]) {
				continue
			}
			if e.Name() == "layout.templ" {
				readerPairs++
				continue
			}
			actual[e.Name()]++
		}
	}
	// The readers return the fill and the ink separately, so layout.templ holds no pair in one literal; a pair
	// appearing there would be a reader that stopped splitting them. Zero is the sanity check that the scan reads text.
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
