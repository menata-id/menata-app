package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// handWrittenLayoutSites is the Experience Plane's remaining debt, counted per file, **shrink-only**.
//
// **This gate exists to direct work, not only to prevent regression**, which makes it the odd one out in
// this package. The other ratchets freeze something already correct; this one freezes something known to
// be wrong, so that the next session finds it from a failing test rather than from remembering to read a
// plan. Two directions, both useful:
//
//   - a **new** hand-written layout site fails, and the message names the primitive it should have used.
//     That is the half that stops the debt growing silently -- 007 §12.4's normative rule is already
//     violated by 46 of 49 navigation routes, and every bespoke screen added makes it worse.
//   - a **lower** count fails until the entry is updated, which is how a migration gets locked in rather
//     than left as room to regress.
//
// Counted syntactically, on purpose: "is this really a row" is a judgement, and a gate that made it would
// be the ~50%-false-finding shape this repo has deleted twice. What the syntax cannot tell apart is noted
// per entry instead.
//
// **Neither remaining floor is zero, and that is a finding rather than a shortfall.** A site stays
// hand-written when absorbing it would make the primitive accept something it must not: a CSS class string
// (007 §12.3's "arbitrary properties", §15.2's framework classes), a second gap ladder for two sites, or a
// choice of HTML element the vocabulary has no word for. Each entry below says which. Reading the floor as
// debt would push exactly the wrong change.
//
// Measured 2026-10-03 across every `.templ`, after two earlier measurements got the population wrong --
// the first counted frequency and not uniformity, the second counted uniformity and not parameterisability
// (see `domain.LayoutKind`'s retraction). **45 sites in 20 files -> 13 in 10**, over three slices the same
// day: `grid` (5), `split` (5) and `row` (22). `grid`'s key is *absent* rather than zero, because an entry
// naming a kind with no sites fails too.
var handWrittenLayoutSites = map[string]map[string]int{
	// `row`: horizontal flow that wraps. **22 of 33 sites migrated on 2026-10-03; these eleven are the
	// floor.** Three kinds, none of them debt: seven carry typography, decoration or sizing on the same
	// element (absorbing them would make the primitive take a CSS class string -- 007 §12.3, §15.2); two want
	// an asymmetric column/row gap the single Gap ladder cannot express; and two are not a `<div>` at all (a
	// `<form>` and a `<span>`), which is the one genuine missing capability here -- choosing the element needs
	// §12.5 Slot or a Component, i.e. Stage 2. `rendering.rowLayout`'s comment carries the per-site reasons.
	"row": {
		"appsettings.templ": 1, "appshell.templ": 1, "inference.templ": 1, "installapplication.templ": 1,
		"machine.templ": 1, "mytasks.templ": 1, "reviewdocument.templ": 2, "rolematrix.templ": 1,
		"workspacemembers.templ": 2,
	},
	// `split`: a main area beside a fixed-width aside. **§12.2 lists `split`, and an earlier measurement of
	// mine claimed it had zero uses.** It had five, all migrated on 2026-10-03; what is left is the two
	// one-offs named above, which is this kind's floor rather than debt.
	"split": {
		"automation.templ": 1, "rolematrix.templ": 1,
	},
}

// layoutSitePatterns are the syntactic shapes counted. Kept beside the population so a reader can
// reproduce the count with the same definition the gate uses.
var layoutSitePatterns = map[string]*regexp.Regexp{
	"row":   regexp.MustCompile(`class="flex flex-wrap[^"]*"`),
	"split": regexp.MustCompile(`class="grid[^"]*grid-cols-\[`),
	"grid":  regexp.MustCompile(`class="grid[^"]*\bgrid-cols-\d`),
}

// TestHandWrittenLayoutSitesOnlyShrink is the directive half of the Experience Plane work.
func TestHandWrittenLayoutSitesOnlyShrink(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}

	actual := map[string]map[string]int{"row": {}, "grid": {}, "split": {}}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		s := string(b)
		splits := len(layoutSitePatterns["split"].FindAllString(s, -1))
		for kind, pat := range layoutSitePatterns {
			n := len(pat.FindAllString(s, -1))
			// A split matches the grid pattern too; count it once, as the more specific kind.
			if kind == "grid" {
				n -= splits
			}
			if n > 0 {
				actual[kind][e.Name()] = n
			}
		}
	}

	advice := map[string]string{
		"row":   "use `rowLayout` (domain.RowAlign x RowJustify x Gap, all closed sets) -- unless this site is one of the eleven kinds of floor, in which case say which in the entry above rather than widening the primitive",
		"grid":  "use `gridLayout` (domain.GridCols x Gap)",
		"split": "use §12.2's `split` via `splitLayout` (domain.SplitSide x AsideWidth x Gap)",
	}

	for _, kind := range []string{"grid", "row", "split"} {
		want, got := handWrittenLayoutSites[kind], actual[kind]
		for _, f := range sortedFileNames(got) {
			switch w := want[f]; {
			case got[f] > w:
				t.Errorf("%s: %d hand-written %q layout site(s), frozen at %d -- %s. See domain.LayoutKind for the measured shapes, and menata-app-document's Experience Plane kajian §6 for the plan", f, got[f], kind, w, advice[kind])
			case got[f] < w:
				t.Errorf("%s: %d hand-written %q site(s), down from %d -- lower the entry in this file to lock the migration in", f, got[f], kind, w)
			}
		}
		for _, f := range sortedFileNames(want) {
			if _, still := got[f]; !still {
				t.Errorf("handWrittenLayoutSites still lists %s under %q, which no longer has one -- remove the entry", f, kind)
			}
		}
	}

	total := 0
	for _, m := range actual {
		for _, n := range m {
			total += n
		}
	}
	if total == 0 {
		t.Fatal("no hand-written layout site found anywhere -- either the Experience Plane migration is complete (delete this gate and say so in capabilities.md) or the patterns have stopped matching")
	}
}

func sortedFileNames[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
