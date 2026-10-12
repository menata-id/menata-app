package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// TestEveryThemeCategoryHasAnImplRatchet is G1 of rencana 0.4 (2026-10-12): **every Theme role's default class is
// counted by a gate, or is listed in themeRolesWithoutImplRatchet with the reason it is not.**
//
// **Why per role, by probe.** The first version mapped each `domain.Theme` category to a gate *name* and checked only
// that the name existed. The independent review satisfied it with `"Padding": {"TestAppManifestLoads"}`, and found
// three categories whose role defaults nothing counted (background base/sunken, badge paddings, neutral/muted tone).
// The hard-coded "Padding: 0" line hid it. So the gate now does three things:
//   - it takes every role of every category from the `domain.Known*` sets;
//   - it derives the role's default class **from the code**: the step from `domain.DefaultTheme()`, then the class
//     from the reader's own `case … return` arms in layout.templ;
//   - it feeds `<div class="…">` holding that class to the **same counting logic** the category's gate uses, and
//     requires a count.
//
// Why a planned per-file `dsImplDebt` map was not built: 0.2 had already put a ratchet beside each category, so a
// second map would count the same literals twice (001 #8).
//
// Blind spot: the probe proves the gate's pattern matches the default class. It cannot prove that the gate is wired to
// read every file; each gate's own sanity check does that.
func TestEveryThemeCategoryHasAnImplRatchet(t *testing.T) {
	consts := domainStringConsts(t)
	layout, err := os.ReadFile(filepath.Join(repoRoot(), "internal", "rendering", "layout.templ"))
	if err != nil {
		t.Fatal(err)
	}
	arms := func(fn string) map[string]string { return readerArms(t, string(layout), fn, consts) }
	th := domain.DefaultTheme()
	probe := func(class string) string { return `<div class="` + class + `"></div>` }
	literal := func(tok string) func(string) bool {
		return func(class string) bool { return strings.Contains(" "+class+" ", " "+tok+" ") }
	}
	type role struct{ category, name, class string }
	var roles []role
	add := func(cat, name, class string) { roles = append(roles, role{cat, name, class}) }
	for r := range domain.KnownRadiusRoles {
		add("Radius", string(r), armFor(arms("radiusClass"), string(th.RadiusFor(r))))
	}
	for r := range domain.KnownWeightRoles {
		add("Weight", string(r), armFor(arms("weightClass"), string(th.WeightFor(r))))
	}
	for r := range domain.KnownTextRoles {
		add("Text", string(r), armFor(arms("textClass"), string(th.TextFor(r))))
	}
	for g := range domain.KnownGaps {
		add("Gap", string(g), armFor(arms("layoutGap"), string(th.GapFor(g))))
	}
	for r := range domain.KnownBorderRoles {
		add("Border", string(r), armFor(arms("borderClass"), string(th.BorderFor(r))))
	}
	for r := range domain.KnownInkRoles {
		add("Ink", string(r), armFor(arms("inkClass"), string(th.InkFor(r))))
	}
	for r := range domain.KnownBackgroundRoles {
		add("Background", string(r), armFor(arms("backgroundClass"), string(th.BackgroundFor(r))))
	}
	for tone := range domain.KnownBadgeTones {
		p := string(th.ToneFor(tone))
		add("Tone", string(tone), armFor(arms("toneFillClass"), p)+" "+armFor(arms("toneInkClass"), p))
	}
	for r := range domain.KnownPaddingRoles {
		add("Padding", string(r), arms("paddingClass")[string(r.Axis())+"/"+string(th.PaddingFor(r))])
	}
	counted := map[string]func(class string) bool{
		"Radius": func(c string) bool {
			for _, tok := range strings.Fields(c) {
				if radiusToken.MatchString(tok) {
					return true
				}
			}
			return false
		},
		"Weight": func(c string) bool {
			return countThemeTokens(probe(c)).weight > 0 || dividerToken.MatchString(probe(c))
		},
		"Text":       func(c string) bool { return countThemeTokens(probe(c)).size > 0 },
		"Ink":        func(c string) bool { return countThemeTokens(probe(c)).ink > 0 },
		"Background": func(c string) bool { return countThemeTokens(probe(c)).fill > 0 },
		"Gap": func(c string) bool {
			return ladderGapToken.MatchString(c) || halfStepGapToken.MatchString(probe(c)) || regionGapToken.MatchString(probe(c))
		},
		"Border": func(c string) bool {
			return literal(controlBorderToken)(c) || literal(surfaceEdgeToken)(c) || dividerToken.MatchString(probe(c))
		},
		"Tone":    func(c string) bool { n, _ := countTonePairs(probe(c)); return n > 0 },
		"Padding": literal(panelPaddingToken),
	}
	exemptSeen := map[string]bool{}
	var lines []string
	for _, r := range roles {
		key := r.category + "/" + r.name
		if r.class == "" || strings.TrimSpace(r.class) == "" {
			t.Errorf("%s: no default class derived from the reader's arms -- the parse is reading the wrong text", key)
			continue
		}
		count, ok := counted[r.category]
		if !ok {
			t.Errorf("Theme category %q has no counting logic in G1 -- name the gate that counts its defaults", r.category)
			continue
		}
		why, exempt := themeRolesWithoutImplRatchet[key]
		switch hit := count(r.class); {
		case hit && exempt:
			t.Errorf("%s (%q) is counted by its gate but listed in themeRolesWithoutImplRatchet -- drop the stale entry", key, r.class)
		case !hit && !exempt:
			t.Errorf("%s: its default %q is counted by no gate -- a hand-written copy would never be seen; gate it, or list it in "+
				"themeRolesWithoutImplRatchet with the reason it cannot be", key, r.class)
		case exempt:
			exemptSeen[key] = true
			lines = append(lines, fmt.Sprintf("%-28s %-28s not counted: %s", key, r.class, why))
		default:
			lines = append(lines, fmt.Sprintf("%-28s %-28s counted", key, r.class))
		}
	}
	for k := range themeRolesWithoutImplRatchet {
		if !exemptSeen[k] {
			found := false
			for _, r := range roles {
				found = found || r.category+"/"+r.name == k
			}
			if !found {
				t.Errorf("themeRolesWithoutImplRatchet names %q, which is not a Theme role -- a stale entry", k)
			}
		}
	}
	if len(roles) < 30 {
		t.Fatalf("only %d Theme roles enumerated -- the Known* sets are not being read", len(roles))
	}
	sort.Strings(lines)
	t.Logf("G1, one line per Theme role:\n  %s", strings.Join(lines, "\n  "))
}

// themeRolesWithoutImplRatchet is the closed list of Theme roles whose default class no gate counts, with the reason.
// It fails both ways: an entry whose default is now counted is stale.
var themeRolesWithoutImplRatchet = map[string]string{
	"Background/base":         "bg-slate-50 wells: whether a well is `base` or `sunken` is an open owner decision (DSD D3, DH 2026-10-11), so its hand-written lines (about 17 on 2026-10-12) are not debt yet",
	"Background/sunken":       "bg-slate-100 wells and hovers: same open decision as base",
	"Tone/neutral":            "bg-slate-100 text-slate-600 is also a well with body text and an app icon's grey; a pair count would be mostly false findings (tone-pair gate counts colour hues only)",
	"Tone/muted":              "bg-slate-50 text-slate-400: same collision as neutral",
	"Padding/badge-x":         "px-2.5 is generic spacing; a hand-written badge is held by the chip ratchet and StatusBadge, not by its padding token",
	"Padding/badge-y":         "py-1 is generic spacing; see badge-x",
	"Padding/badge-compact-x": "px-2 is generic spacing; see badge-x",
	"Padding/badge-compact-y": "py-0.5 is generic spacing; see badge-x",
}

// domainStringConsts maps every string-typed constant in internal/domain to its value.
func domainStringConsts(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	re := regexp.MustCompile(`(?m)^\s*(\w+)\s+\w+\s*=\s*"([^"]*)"`)
	files, _ := filepath.Glob(filepath.Join(repoRoot(), "internal", "domain", "*.go"))
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, _ := os.ReadFile(f)
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			out[m[1]] = m[2]
		}
	}
	return out
}

// readerDefaultArm keys a reader's `default:` arm, which answers every value no `case` names (tone `neutral`'s palette).
const readerDefaultArm = "<default>"

// armFor is the class a reader answers for a value: its own arm, or the default arm.
func armFor(arms map[string]string, v string) string {
	if c, ok := arms[v]; ok {
		return c
	}
	return arms[readerDefaultArm]
}

// readerArms maps a reader's `case domain.X:` labels (by the constant's value) to the class its arm returns. A
// two-level switch (paddingClass: axis, then amount) keys the inner arms as "<axis>/<amount>", and an outer `default:`
// stands for the axis the switch did not name ("all").
func readerArms(t *testing.T, src, fn string, consts map[string]string) map[string]string {
	t.Helper()
	body := regexp.MustCompile(`(?s)\nfunc ` + fn + `\(.*?\n\}`).FindString(src)
	if body == "" {
		t.Fatalf("reader %s not found in layout.templ", fn)
	}
	out := map[string]string{}
	label := regexp.MustCompile(`^(\t+)case (.*):$`)
	ret := regexp.MustCompile(`^\t+return "([^"]*)"$`)
	ident := regexp.MustCompile(`domain\.(\w+)`)
	var outer string
	var pending []string
	for _, line := range strings.Split(body, "\n") {
		if m := label.FindStringSubmatch(line); m != nil {
			var vals []string
			for _, id := range ident.FindAllStringSubmatch(m[2], -1) {
				vals = append(vals, consts[id[1]])
			}
			if len(m[1]) == 1 && fn == "paddingClass" {
				outer, pending = vals[0], nil
				continue
			}
			pending = vals
			continue
		}
		if strings.TrimSpace(line) == "default:" {
			if fn == "paddingClass" && !strings.HasPrefix(line, "\t\t") {
				outer, pending = consts["PaddingAxisAll"], nil
			} else {
				pending = []string{readerDefaultArm}
			}
			continue
		}
		if m := ret.FindStringSubmatch(line); m != nil && pending != nil {
			for _, v := range pending {
				if fn == "paddingClass" {
					out[outer+"/"+v] = m[1]
				} else {
					out[v] = m[1]
				}
			}
			pending = nil
		}
	}
	if len(out) == 0 {
		t.Fatalf("no arms parsed from %s", fn)
	}
	return out
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
	counts := countLiteralTokens(t, surfaceEdgeToken)
	checkFloors(t, counts, func(f string) int { return handWrittenSurfaceEdges[f].n }, keysOf(handWrittenSurfaceEdges),
		"literal border-slate-200", "a surface's edge reads borderClass(ctx, domain.BorderSurface), so theme.border.surface moves it; an edge that is not a surface's goes in handWrittenSurfaceEdges with what it is")
}

// TestNoHandWrittenPanelPadding is a zero-floor gate (2026-10-12, rencana 0.4). `p-5` is the default of
// `padding.panel` and `padding.section`. Every hand-written one sat on a surface, and all 14 moved byte-identically:
// a `<section>` reads `PaddingSection` (the `sectionLayout` role), and any other card reads `PaddingPanel`.
// Blind spots: `p-5` returned by a Go function, a variant (`sm:p-5`), and `px-5 py-5` (equal to `p-5`; none in the
// tree is an exact pair today). Zero-floor with no floor map, so a `p-5` that is not a panel or section (a toast, a
// pre-auth card) must first argue for a floor map here rather than borrow a role.
func TestNoHandWrittenPanelPadding(t *testing.T) {
	counts := countLiteralTokens(t, panelPaddingToken)
	checkFloors(t, counts, func(string) int { return 0 }, nil,
		"literal p-5", "a panel or section's padding reads paddingClass(ctx, domain.PaddingPanel|PaddingSection)")
}

// The tokens the value gates count, shared with G1 so its probe tests what the gates count, not a copy of it.
const (
	panelPaddingToken  = "p-5"
	surfaceEdgeToken   = "border-slate-200"
	controlBorderToken = "border-slate-300"
)

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
		// layout.templ is read too: its readers are `return "..."` statements, which this never reads, and skipping the
		// file only let a new primitive written with literals through (2026-10-12 review).
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
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

// TestPanelAndSectionPaddingMatchTheirElement holds the rule rencana 0.4 migrated by: a hand-written `<section>` reads
// `PaddingSection` (the `sectionLayout` role) and any other element reads `PaddingPanel`. The two roles share the default
// `five`, so a byte-diff cannot tell them apart, and the 2026-10-12 review swapped five of them unseen. layout.templ is
// skipped: its primitives choose their own role.
func TestPanelAndSectionPaddingMatchTheirElement(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	site := regexp.MustCompile(`<(\w+)\b[^<>]*?class=\{[^}]*paddingClass\(ctx, domain\.Padding(Panel|Section)\)`)
	seen := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") || e.Name() == "layout.templ" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		src := stripTemplComments(string(b))
		for _, m := range site.FindAllStringSubmatchIndex(src, -1) {
			seen++
			tag, role := src[m[2]:m[3]], src[m[4]:m[5]]
			line := strings.Count(src[:m[0]], "\n") + 1
			if (tag == "section") != (role == "Section") {
				t.Errorf("%s:%d: a <%s> reads Padding%s -- a <section> reads PaddingSection and any other element PaddingPanel", e.Name(), line, tag, role)
			}
		}
	}
	if seen < 14 {
		t.Fatalf("found %d panel/section padding readers, want at least the 14 rencana 0.4 migrated -- the scan is reading the wrong text", seen)
	}
}
