package conformance

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// themeCategoryImplRatchets is G1 of rencana 0.4 (2026-10-12): for every Theme category, the gate that holds the
// literals which already have a reader (the `impl` bucket), and the maps it freezes them in.
//
// **Why this shape, and not the planned `dsImplDebt` map.** The plan (written 2026-10-11) asked for one new map
// per file per category. By the time it was built, 0.2 had already put a ratchet beside each category it migrated, and
// `themedebt` held four more. A second map would count the same literals twice, which is 001 #8 at the test level.
// So G1 is a **coverage** gate over the ratchets that exist:
//   - every `domain.Theme` field (read from source) must name at least one gate here;
//   - every named gate must exist in this package;
//   - a new Theme category without an impl ratchet fails.
//
// Run with `-v` for the per-category floor totals.
//
// Measuring coverage found two categories with no impl ratchet. Each was migrated, then gated:
//   - padding: 14 hand-written `p-5` on surfaces now read `paddingClass`;
//   - border.surface: six loose `border-slate-200` and two state-swapped card edges now read `borderClass`.
var themeCategoryImplRatchets = map[string][]string{
	"Radius":     {"TestHandWrittenRadiiOnlyShrink"},
	"Weight":     {"TestHandWrittenThemeTokensOnlyShrink"},
	"Text":       {"TestHandWrittenThemeTokensOnlyShrink", "TestNoHandWrittenSubheading", "TestNoHandWrittenHeadingOrEyebrow"},
	"Gap":        {"TestHandWrittenLadderGapsOnlyShrink", "TestHandWrittenHalfStepGapsOnlyShrink", "TestHandWrittenRegionGapsOnlyShrink"},
	"Border":     {"TestHandWrittenControlBordersOnlyShrink", "TestHandWrittenSurfaceEdgesOnlyShrink", "TestHandWrittenDividersOnlyShrink", "TestNoHandWrittenRegionRule"},
	"Tone":       {"TestHandWrittenTonePairsOnlyShrink"},
	"Ink":        {"TestHandWrittenThemeTokensOnlyShrink"},
	"Background": {"TestHandWrittenThemeTokensOnlyShrink"},
	"Padding":    {"TestNoHandWrittenPanelPadding"},
}

func TestEveryThemeCategoryHasAnImplRatchet(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filepath.Join(repoRoot(), "internal", "domain", "theme.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var fields []string
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name.Name != "Theme" {
			return true
		}
		for _, fl := range ts.Type.(*ast.StructType).Fields.List {
			if _, isMap := fl.Type.(*ast.MapType); isMap {
				for _, nm := range fl.Names {
					fields = append(fields, nm.Name)
				}
			}
		}
		return false
	})
	if len(fields) == 0 {
		t.Fatal("no map field found on domain.Theme -- the scan is reading the wrong text")
	}
	tests := map[string]bool{}
	entries, _ := os.ReadDir(".")
	testFunc := regexp.MustCompile(`(?m)^func (Test\w+)\(t \*testing\.T\)`)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), "_test.go") {
			b, _ := os.ReadFile(e.Name())
			for _, m := range testFunc.FindAllStringSubmatch(string(b), -1) {
				tests[m[1]] = true
			}
		}
	}
	for _, fld := range fields {
		gates, ok := themeCategoryImplRatchets[fld]
		if !ok || len(gates) == 0 {
			t.Errorf("Theme category %q has no impl ratchet -- a literal equal to its default can be written by hand and nothing counts it; "+
				"migrate the sites that have a reader, gate the rest, and name the gate in themeCategoryImplRatchets", fld)
			continue
		}
		for _, g := range gates {
			if !tests[g] {
				t.Errorf("themeCategoryImplRatchets[%q] names %s, which is not a test in internal/conformance", fld, g)
			}
		}
	}
	for c := range themeCategoryImplRatchets {
		found := false
		for _, fld := range fields {
			found = found || fld == c
		}
		if !found {
			t.Errorf("themeCategoryImplRatchets names %q, which is not a field of domain.Theme -- a stale entry", c)
		}
	}
	if testing.Verbose() {
		var td themeDebt
		for _, d := range handWrittenThemeTokens {
			td.size, td.ink, td.weight, td.fill = td.size+d.size, td.ink+d.ink, td.weight+d.weight, td.fill+d.fill
		}
		sum := func(m map[string]int) (n int) {
			for _, v := range m {
				n += v
			}
			return
		}
		radii, ladder, tones, controls, edges, off := 0, 0, 0, 0, 0, 0
		for _, v := range handWrittenRadii {
			radii += v.n
		}
		for _, v := range handWrittenLadderGaps {
			ladder += v.n
		}
		for _, v := range handWrittenTonePairs {
			tones += v.n
		}
		for _, v := range handWrittenControlBorders {
			controls += v.n
		}
		for _, v := range handWrittenSurfaceEdges {
			edges += v.n
		}
		for _, v := range handWrittenOffLadderSizes {
			off += v.n
		}
		lines := []string{
			fmt.Sprintf("Radius: %d corners", radii),
			fmt.Sprintf("Weight: %d", td.weight),
			fmt.Sprintf("Text: %d sizes, plus %d off-ladder (DS-D13)", td.size, off),
			fmt.Sprintf("Gap: %d ladder, %d half-step, %d region", ladder, sum(handWrittenHalfStepGaps), sum(handWrittenRegionGaps)),
			fmt.Sprintf("Border: %d control, %d surface edge, %d divider", controls, edges, sum(handWrittenDividerFloors)),
			fmt.Sprintf("Tone: %d pairs", tones),
			fmt.Sprintf("Ink: %d", td.ink),
			fmt.Sprintf("Background: %d white fills", td.fill),
			"Padding: 0 (zero-floor gate)",
		}
		sort.Strings(lines)
		t.Logf("impl floors per Theme category (each with its reasons in its own map):\n  %s", strings.Join(lines, "\n  "))
	}
}

// handWrittenSurfaceEdges freezes the literal `border-slate-200` left in `internal/rendering` (2026-10-12, rencana
// 0.4). It is `border.surface`'s default, the edge of a surface, and it reads `borderClass(ctx, domain.BorderSurface)`.
// The edges moved byte-identically: a dialog, a lane, three image frames, a suggestion chip, and the rest state of two
// cards whose edge swaps by state. Counted from string literals only (`joinClassLiterals`), unprefixed, so `sm:` and
// `open:` variants are out. Blind spot: an edge returned by a Go function.
var handWrittenSurfaceEdges = map[string]struct {
	n   int
	why string
}{
	"account.templ": {1, "the read-only email box: slate-200 on slate-50 is a disabled control's look, not a surface edge"},
}

func TestHandWrittenSurfaceEdgesOnlyShrink(t *testing.T) {
	counts := countLiteralTokens(t, "border-slate-200")
	checkFloors(t, counts, func(f string) int { return handWrittenSurfaceEdges[f].n }, keysOf(handWrittenSurfaceEdges),
		"literal border-slate-200", "a surface's edge reads borderClass(ctx, domain.BorderSurface), so theme.border.surface moves it; an edge that is not a surface's goes in handWrittenSurfaceEdges with what it is")
}

// TestNoHandWrittenPanelPadding is a zero-floor gate (2026-10-12, rencana 0.4). `p-5` is the default of
// `padding.panel` and `padding.section`. Every hand-written one sat on a surface, and all 14 moved byte-identically:
// a `<section>` reads `PaddingSection` (the `sectionLayout` role), and any other card reads `PaddingPanel`.
func TestNoHandWrittenPanelPadding(t *testing.T) {
	counts := countLiteralTokens(t, "p-5")
	checkFloors(t, counts, func(string) int { return 0 }, nil,
		"literal p-5", "a panel or section's padding reads paddingClass(ctx, domain.PaddingPanel|PaddingSection)")
}

// countLiteralTokens counts one unprefixed token in the string literals of every class attribute outside layout.templ.
func countLiteralTokens(t *testing.T, tok string) map[string]int {
	t.Helper()
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	classAttr := regexp.MustCompile(`class="([^"]*)"`)
	out := map[string]int{}
	scanned := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") || e.Name() == "layout.templ" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		for _, m := range classAttr.FindAllStringSubmatch(joinClassLiterals(stripTemplComments(string(b))), -1) {
			scanned++
			for _, x := range strings.Fields(m[1]) {
				if x == tok {
					out[e.Name()]++
				}
			}
		}
	}
	if scanned == 0 {
		t.Fatal("no class attribute was scanned -- the gate is reading the wrong text")
	}
	return out
}

func checkFloors(t *testing.T, actual map[string]int, want func(string) int, floored []string, what, use string) {
	t.Helper()
	seen := map[string]bool{}
	for f := range actual {
		seen[f] = true
	}
	for _, f := range floored {
		seen[f] = true
	}
	var files []string
	for f := range seen {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		switch got, w := actual[f], want(f); {
		case got > w:
			t.Errorf("%s: %d %s, frozen at %d -- %s", f, got, what, w, use)
		case got < w:
			t.Errorf("%s: %d %s, down from %d -- lower the floor to lock it in", f, got, what, w)
		}
	}
}

func keysOf[V any](m map[string]V) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
