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
// **The floor is not zero for `split`.** Two of its seven matches are genuine one-offs that will stay
// hand-written: `automation.templ`'s `[6rem_1fr]` label/value definition list, and `rolematrix.templ`'s
// bordered row grid. A one-off is what a primitive is *not* for.
//
// Measured 2026-10-03 across every `.templ`, after two earlier measurements got the population wrong --
// the first counted frequency and not uniformity, the second counted uniformity and not
// parameterisability (see `domain.LayoutKind`'s retraction). 45 sites in 20 files, of which **`grid`'s five
// are now zero**: `domain.LayoutGrid` and `rendering.gridLayout` landed with all five callers the same day,
// and `grid` is absent below rather than set to zero because an entry for a kind with no sites fails too.
var handWrittenLayoutSites = map[string]map[string]int{
	// `row`: horizontal flow that wraps. Alignment is a closed set of three in this corpus --
	// items-center (11), justify-between (5), items-baseline (2) -- which is exactly what Bootstrap,
	// Tailwind and every component library expose as named options.
	"row": {
		"approvalinbox.templ": 6, "appsettings.templ": 1, "appshell.templ": 1, "boardsettings.templ": 1,
		"detail.templ": 2, "documentsubmit.templ": 1, "groups.templ": 1, "inference.templ": 3,
		"installapplication.templ": 1, "machine.templ": 1, "mytasks.templ": 1, "newapplication.templ": 2,
		"reviewdocument.templ": 5, "rolematrix.templ": 1, "signatureplacement.templ": 2,
		"workspacehome.templ": 1, "workspacemembers.templ": 3,
	},
	// `split`: a main area beside a fixed-width aside, five of these differing only in a pixel width and
	// which side. **§12.2 lists `split`, and an earlier measurement of mine claimed it had zero uses.**
	// Two of the seven are the one-offs named above and will not migrate.
	"split": {
		"automation.templ": 1, "documentsubmit.templ": 1, "newapplication.templ": 2,
		"reviewdocument.templ": 1, "rolematrix.templ": 1, "signatureplacement.templ": 1,
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
		"row":   "build or use the `row` primitive (alignment is a closed set of three here: centre, spread, baseline)",
		"grid":  "build or use the `grid` primitive with a `columns` enum",
		"split": "build or use §12.2's `split` primitive (main area plus a fixed-width aside)",
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
