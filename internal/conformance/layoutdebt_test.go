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
// **Every remaining entry is a floor, and that is a finding rather than a shortfall.** A site stays
// hand-written when absorbing it would make the primitive accept something it must not: a CSS class string
// (007 §12.3's "arbitrary properties", §15.2's framework classes), a second gap ladder for two sites, a
// choice of HTML element the vocabulary has no word for, or an `aria-label` that makes a `<section>` a
// landmark. Each entry below says which, site by site. Reading the floor as debt would push exactly the
// wrong change. `grid` and `columns` have reached zero: `grid`'s key is *absent* rather than empty, because
// an entry naming a kind with no sites fails too.
//
// Measured 2026-10-03 across every `.templ`, after two earlier measurements got the population wrong --
// the first counted frequency and not uniformity, the second counted uniformity and not parameterisability
// (see `domain.LayoutKind`'s retraction). 45 sites in 20 files then; **read the current numbers out of the
// map below**, which is what the 2026-10-07 slices (section 14 -> 8, columns 4 -> 0, row re-read) moved.
var handWrittenLayoutSites = map[string]map[string]int{
	// `row`: horizontal flow that wraps. **24 of 33 sites migrated; the nine left are read one by one below
	// (2026-10-07), and the earlier "eleven, two of them needing a capability" was a count of kinds, not of
	// sites.** Re-read: four are not a `<div>`, but only **one** of those four is a bare element swap -- the
	// others carry typography, an asymmetric gap or a form binding as well, so an element enum would fix one site
	// of nine and be shape-before-need for the rest. None is debt:
	//   - `appsettings` (a `<span>`, `text-sm text-slate-400`): element and typography together.
	//   - `inference` (`border-b border-slate-100 pb-1.5`): a heading underline, which
	//     `TestHandWrittenDividersOnlyShrink` already reads as a region rule rather than a row separator.
	//   - `installapplication` (a `<form method action>`): element plus 007 §11.3 Binding, which has no primitive.
	//   - `machine` (a `<span>`, `gap-x-2 gap-y-1`, `text-xs`): element, an asymmetric gap and typography.
	//   - `reviewdocument` x2: line ~223 is a `<span>` and the **only pure element swap** among the nine; the
	//     signature legend carries `text-3xs text-slate-500`.
	//   - `rolematrix` (`gap-x-3 gap-y-1`): an asymmetric gap; `machine`'s is `gap-x-2`, so the two are not even
	//     one case, and a second Gap ladder for two different pairs is the thing §12.3 stops.
	//   - `workspacemembers` x1 (`sm:w-32`; the role cell no longer wraps two badges and is a plain width): a cell's width in a column the header row above fixes;
	//     sizing, which is not layout.
	// Building nothing is the result, and it is the boundary working: each site stays hand-written because
	// absorbing it would make the primitive accept a CSS class string, a second gap ladder, or an element choice
	// the vocabulary has no word for (§12.5 Slot / §12.3 Component, Stage 2).
	"row": {
		"appsettings.templ": 1, "inference.templ": 1, "machine.templ": 1,
		"reviewdocument.templ": 2, "rolematrix.templ": 1,
	},
	// `split`: a main area beside a fixed-width aside. **§12.2 lists `split`, and an earlier measurement of
	// mine claimed it had zero uses.** It had five, all migrated on 2026-10-03; what is left is the two
	// one-offs named above, which is this kind's floor rather than debt.
	"split": {
		"rolematrix.templ": 1,
	},
	// `columns` has no entry: its last four sites left on 2026-10-07 and an entry naming a kind with no sites
	// fails. Three were one divided-list row (`dividedRowClasses`, a class reader -- `border-b` and `px-4` are not
	// layout, so a `columnsLayout` taking them would accept arbitrary classes, 007 §12.3), and one was a panel that
	// becomes columns (`panelLayout{ columnsLayout }`). `TestNoHandWrittenDividedRow` holds the first shape at zero.
	// `section`: a bordered, padded surface that is also a `<section>` (§12.2). **Built (`sectionLayout`,
	// `panelLayout`) and migrated 2026-10-07: 14 -> 8**, and the 8 are a floor, in three kinds:
	//   - `layout.templ` (2): the two renderers themselves, which the pattern points *at*.
	//   - four **named regions** -- `calendar` (a day cell whose border swaps by state), `newapplication`'s
	//     conversation pane (`min-h-96`), and two on `workspacehome` (a clipped link list; the add-application
	//     card). Each carries `aria-label`/`aria-labelledby`, which makes the `<section>` a landmark, and a
	//     Layout accepting arbitrary attributes is what 007 §12.3 forbids by name -- the same reason the
	//     `row` filter groups keep `role="group"` on a wrapper. **These four were invisible to the old
	//     pattern** (`<section class=` cannot match `<section aria-label=... class=`), so the count that read
	//     14 was an undercount of the same kind this gate has had before.
	//   - two **not a stacking surface**: `account`'s Two-factor row (a non-wrapping row; `rowLayout` always
	//     wraps) and `workspacemembers`' Remove-from-workspace zone (a `border-red-200` edge -- red is the
	//     tone palette's family, not the slate-shade ladder `border.*` roles climb; `workspacesettings`' own
	//     Danger zone is the second case and, being a `<div>`, is outside this pattern).
	// `sectionHeaderRow`'s five callers were counted here until 2026-10-07 and are not any more: it is the
	// heading a section *holds*, not a layout, and every caller now sits inside `stackLayout`.
	"section": {
		"account.templ": 1, "layout.templ": 2, "newapplication.templ": 1,
		"workspacehome.templ": 2,
	},
}

// layoutSitePatterns are the syntactic shapes counted. Kept beside the population so a reader can
// reproduce the count with the same definition the gate uses.
var layoutSitePatterns = map[string]*regexp.Regexp{
	"row":   regexp.MustCompile(`class="flex flex-wrap[^"]*"`),
	"split": regexp.MustCompile(`class="grid[^"]*grid-cols-\[`),
	"grid":  regexp.MustCompile(`class="grid[^"]*\bgrid-cols-\d`),
	// **These two were missing until 2026-10-03, and their absence is the finding, not the fix.** This gate
	// exists to point the next session at the remaining work, and it was reporting 13 sites while the real
	// unmigrated population was 51 -- because its patterns were written with the same method that wrongly
	// rejected `row` and `grid`: searching for a class string instead of for the meaning. `columns` was
	// recorded twice as having "zero measured uses" and has nine; `section` was dismissed as overlapping
	// `panel` and has 34. A directive gate that under-reports by four times directs nothing.
	"columns": regexp.MustCompile(`class="[^"]*(?:sm|md|lg):flex-row`),
	"section": regexp.MustCompile(`<section\b[^>]*\bclass=`),
}

// TestHandWrittenLayoutSitesOnlyShrink is the directive half of the Experience Plane work.
func TestHandWrittenLayoutSitesOnlyShrink(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}

	actual := map[string]map[string]int{"row": {}, "grid": {}, "split": {}, "columns": {}, "section": {}}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		s := string(b)
		if e.Name() != "layout.templ" { // the primitives' own renderers are what the patterns point *at*
			// String literals only: these patterns anchor at the start of the class value (`class="flex flex-wrap…`),
			// and the radius rewrite in joinClassExpressions would put a corner in front of a site whose expression
			// opens with radiusClass, hiding it (found by the 2026-10-12 independent review).
			s = joinClassLiterals(s)
		}
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
		"row":     "use `rowLayout` (domain.RowAlign x RowJustify x Gap, all closed sets) -- unless this site is one of the nine read in the entry above, in which case say which in the entry above rather than widening the primitive",
		"grid":    "use `gridLayout` (domain.GridCols x Gap)",
		"split":   "use §12.2's `split` via `splitLayout` (domain.SplitSide x AsideWidth x Gap)",
		"columns": "use §12.2's `columns` via `columnsLayout` (domain.ColumnsAlign x Gap); a divided list row is `dividedRowClasses`",
		"section": "use §12.2's `section` via `sectionLayout(Gap)` (stacks) or `panelLayout()` (does not), or `stackLayout` when there is no surface -- unless this site is one of the floor kinds named in the entry above",
	}

	for _, kind := range []string{"grid", "row", "split", "columns", "section"} {
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

var (
	classExpression = regexp.MustCompile(`class=\{([^}]*)\}`)
	stringLiteral   = regexp.MustCompile(`"([^"]*)"`)
)

// joinClassExpressions rewrites `class={ "a", themeReader(ctx), "b" }` as `class="a b"`, so that a site whose
// literal was split to splice a Theme reader into it is still seen by the patterns above, which read the
// `class="..."` form. Without it a migration to `borderClass`/`weightClass` made a hand-written layout site
// vanish from the count while staying hand-written -- a ratchet that fell by three and measured an edit to the
// attribute, not a migration to a primitive. Only string literals are kept; the readers' own output is not
// layout vocabulary.
//
// It also reads a radius reader called inline as the corner it answers by default (`radiusReadersAsLiterals`).
// Shape gates recognise a chip, a surface or a tile partly by its literal corner, and rencana 0.2b moved 57 corners
// to `radiusClass`: without this, five gates lost floors that were still hand-written in every other respect
// (menjaga-design-system L13). `TestHandWrittenRadiiOnlyShrink`, which counts the corners themselves, reads the raw
// form through `joinClassLiterals`.
func joinClassExpressions(src string) string {
	return joinClassLiterals(radiusReadersAsLiterals(src))
}

// typeReadersAsLiterals rewrites `textClass(ctx, domain.Role*)` and `weightClass(ctx, domain.Weight*)` as the quoted class
// each answers under the default Theme (gap-plan P5). A shape gate that recognises a chip, a muted paragraph or a caption
// partly by its literal size loses its floors when that size moves to a reader while the element stays hand-written in
// every other respect (L13, a fifth time); such a gate applies this where it falls. It is **opt-in, not part of
// `joinClassExpressions`**, because the Static kinds and the Components are implemented in `layout.templ` with these very
// readers, and a gate that read them as literals would count the primitive as the hand-written copy it replaces.
// `TestTypeRewriteMatchesTheDefaultTheme` holds `typeDefaults` to the Theme.
func typeReadersAsLiterals(src string) string {
	return typeReaderCall.ReplaceAllStringFunc(src, func(m string) string {
		if v, ok := typeDefaults[typeReaderCall.FindStringSubmatch(m)[2]]; ok {
			return `"` + v + `"`
		}
		return m
	})
}

var (
	typeReaderCall = regexp.MustCompile(`(textClass|weightClass)\(ctx, domain\.((?:Role|Weight)\w+)\)`)
	typeDefaults   = map[string]string{
		"RoleDisplay": "text-2xl", "RoleHeading": "text-xl", "RoleSubheading": "text-base", "RoleItem": "text-item",
		"RoleBody": "text-sm", "RoleMeta": "text-xs", "RoleLabel": "text-2xs", "RoleCaption": "text-2xs", "RoleEyebrow": "text-3xs",
		"WeightBody": "font-normal", "WeightEmphasis": "font-medium",
	}
)

// radiusReadersAsLiterals rewrites `radiusClass(ctx, domain.Radius*)` as the quoted class it answers under the
// default Theme.
func radiusReadersAsLiterals(src string) string {
	return radiusReaderCall.ReplaceAllStringFunc(src, func(m string) string {
		return `"` + radiusDefaults[radiusReaderCall.FindStringSubmatch(m)[1]] + `"`
	})
}

var (
	radiusReaderCall = regexp.MustCompile(`radiusClass\(ctx, domain\.(Radius\w+)\)`)
	radiusDefaults   = map[string]string{"RadiusControl": "rounded-md", "RadiusSurface": "rounded-lg", "RadiusPill": "rounded-full"}
)

// borderReadersAsLiterals rewrites `borderClass(ctx, domain.Border*)` as the quoted class it answers under the default
// Theme. Only the **shape** gates (surface, tile) apply it: they recognise a hand-written surface partly by its edge, and
// an edge read through the Theme is still a hand-written surface's edge (L13). The **value** gates (control border,
// region rule, field, button) must not, because for them a reader is exactly the migration they count.
func borderReadersAsLiterals(src string) string {
	src = stateEdgeHelperCall.ReplaceAllString(src, `"border-slate-200"`)
	return borderReaderCall.ReplaceAllStringFunc(src, func(m string) string {
		return `"` + borderDefaults[borderReaderCall.FindStringSubmatch(m)[1]] + `"`
	})
}

// stateEdgeHelperCall names the Go helpers that choose a surface's edge by state (chosen in Go so two templ.KV keys
// cannot collide, L12). Their rest state is the surface edge, so for the shape gates they read as that literal:
// without this, moving the overdue approval card's edge into approvalCardEdge (DS-D16) dropped the card from the
// surface ratchet while it stayed hand-written (L13).
var stateEdgeHelperCall = regexp.MustCompile(`approvalCardEdge\(ctx, [^)]*\)`)

var (
	borderReaderCall = regexp.MustCompile(`borderClass\(ctx, domain\.(Border\w+)\)`)
	borderDefaults   = map[string]string{"BorderSurface": "border-slate-200", "BorderControl": "border-slate-300", "BorderDivider": "border-slate-100"}
)

// inkReadersAsLiterals rewrites `inkClass(ctx, domain.Ink*)` as the palette class it draws under the default Theme. Only
// the shape gates that recognise a hand-written element partly by its ink apply it (the muted paragraph and the
// caption): rencana 3.2d moved 217 ink literals to inkClass, and without this both lost every floor while the paragraphs
// stayed hand-written in every other respect (L13 again). The value gates must not, because for them a reader is the
// migration. `TestInkRewriteMatchesTheDefaultTheme` holds inkDefaults to the defaults derived from the code.
func inkReadersAsLiterals(src string) string {
	return inkReaderCall.ReplaceAllStringFunc(src, func(m string) string {
		return `"` + inkDefaults[inkReaderCall.FindStringSubmatch(m)[1]] + `"`
	})
}

var (
	inkReaderCall = regexp.MustCompile(`inkClass\(ctx, domain\.(Ink\w+)\)`)
	inkDefaults   = map[string]string{"InkStrong": "text-slate-900", "InkBody": "text-slate-700", "InkSecondary": "text-slate-500", "InkFaint": "text-slate-400", "InkAccent": "text-blue-600"}
)

// colourReadersAsLiterals rewrites every colour reader call -- the role readers and 3.2d's reader forms -- as the
// palette class it draws under the default Theme. Rencana 3.2d moved 143 more colour literals to readers, and four
// shape gates (chips, h-9 buttons, fields, panel titles) lost floors that were still hand-written in every other
// respect: L13 a fourth time. Only shape gates apply it; a value gate counts the reader as the migration it is.
// `TestColourRewriteMatchesTheDefaultTheme` holds every entry to the reader's own return, resolved through input.css.
// The hover, variant and translucent forms (inkHoverClass, toneInkHoverClass, inkFillHoverClass, accent*,
// inkClassFromSm, surfaceFromSm, openSurfaceClasses, raisedTintClass, unreadTintClass, baseTintClass,
// controlAccentClass, checkedFillClass, sunkenHoverClass, toneInkFillClass) are deliberately not rewritten: no shape
// gate recognises a shape by a hover or a tint, so they stay opaque calls there.
func colourReadersAsLiterals(src string) string {
	src = inkReadersAsLiterals(borderReadersAsLiterals(src))
	return colourReaderCall.ReplaceAllStringFunc(src, func(m string) string {
		if v, ok := colourDefaults[m]; ok {
			return `"` + v + `"`
		}
		return m
	})
}

var colourDefaults = map[string]string{
	"backgroundClass(ctx, domain.BackgroundRaised)": "bg-white", "backgroundClass(ctx, domain.BackgroundBase)": "bg-slate-50",
	"backgroundClass(ctx, domain.BackgroundSunken)": "bg-slate-100",
	"toneFillClass(ctx, domain.ToneNeutral)":        "bg-slate-100", "toneInkClass(ctx, domain.ToneNeutral)": "text-slate-600",
	"toneFillClass(ctx, domain.ToneInfo)": "bg-blue-50", "toneInkClass(ctx, domain.ToneInfo)": "text-blue-700",
	"toneFillClass(ctx, domain.ToneGood)": "bg-emerald-50", "toneInkClass(ctx, domain.ToneGood)": "text-emerald-700",
	"toneFillClass(ctx, domain.ToneBad)": "bg-red-50", "toneInkClass(ctx, domain.ToneBad)": "text-red-700",
	"toneFillClass(ctx, domain.ToneWarn)": "bg-amber-50", "toneInkClass(ctx, domain.ToneWarn)": "text-amber-800",
	"toneFillClass(ctx, domain.ToneMuted)": "bg-slate-50", "toneInkClass(ctx, domain.ToneMuted)": "text-slate-400",
	"onFillClass(ctx)": "text-white", "inkFillClass(ctx, domain.InkStrong)": "bg-slate-900",
	"inkFillClass(ctx, domain.InkAccent)": "bg-blue-600", "inkEdgeClass(ctx)": "border-slate-900",
	"borderFillClass(ctx, domain.BorderSurface)": "bg-slate-200", "borderFillClass(ctx, domain.BorderDivider)": "bg-slate-100",
	"accentActiveClass(ctx)": "text-blue-700",
}

var colourReaderCall = regexp.MustCompile(`\b(?:backgroundClass|toneFillClass|toneInkClass|onFillClass|inkFillClass|inkEdgeClass|borderFillClass|accentActiveClass)\(ctx(?:, [\w.]+)?\)`)

// joinClassLiterals is joinClassExpressions without the reader rewrite: string literals only.
func joinClassLiterals(src string) string {
	return classExpression.ReplaceAllStringFunc(src, func(m string) string {
		var parts []string
		for _, lit := range stringLiteral.FindAllStringSubmatch(m, -1) {
			parts = append(parts, lit[1])
		}
		return `class="` + strings.Join(parts, " ") + `"`
	})
}

// handWrittenHalfStepGaps freezes, per file, the `gap-1.5` literals left after `domain.GapXtight` arrived
// (2026-10-11). Thirteen inline sites moved to `layoutGap(ctx, domain.GapXtight)` byte-identically; these nine stay,
// each for one reason. They are spacing **between** components (vertical `flex-col` stacks, and one Save/Cancel
// button row), and the Design System artifact scopes `gap-xtight` to an icon or dot with its label inside a compact
// component, "never the space between components". They are a measured floor, not debt to clear blindly.
// Shrink-only: a new literal fails and should read GapXtight if it is an inline label pair, or be argued here; a
// lower count fails until the entry drops.
var handWrittenHalfStepGaps = map[string]int{
	"approvalinbox.templ":  1, // a card's stacked title and meta
	"machine.templ":        4, // record card stack; card quick-edit form and its Save/Cancel row; move form
	"newapplication.templ": 1, // stacked label and control
	"workspacehome.templ":  1, // stacked heading block
}

func TestHandWrittenHalfStepGapsOnlyShrink(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	comment := regexp.MustCompile(`//[^\n]*`)
	got := map[string]int{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") || e.Name() == "layout.templ" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if n := len(halfStepGapToken.FindAllString(comment.ReplaceAllString(string(b), ""), -1)); n > 0 {
			got[e.Name()] = n
		}
	}
	for file, n := range got {
		want := handWrittenHalfStepGaps[file]
		switch {
		case n > want:
			t.Errorf("%s: %d hand-written gap-1.5, frozen at %d -- an icon/dot with its label in a compact component reads layoutGap(ctx, domain.GapXtight); spacing between components is not what gap-xtight is for", file, n, want)
		case n < want:
			t.Errorf("%s: %d hand-written gap-1.5, down from %d -- lower the entry to lock the improvement in", file, n, want)
		}
	}
	for file := range handWrittenHalfStepGaps {
		if got[file] == 0 {
			t.Errorf("handWrittenHalfStepGaps names %s, which has no gap-1.5 left -- remove the entry", file)
		}
	}
}

// handWrittenRegionGaps freezes the `gap-6` literals left after `domain.GapRegion` arrived (2026-10-12), the
// Design System's `gap-region` (24px): the space between a screen's large regions. Its one measured site, the
// pre-auth shell's brand block over its card, moved byte-identically. What is left is a **column gutter**, the
// two-column header of the role matrix. It has the same 24px and a different meaning, and rule R2 of the governance
// guide says a value is not folded because it happens to match. The row below it uses `sm:gap-6`, a breakpoint
// variant this count does not read. Shrink-only, both ways.
var handWrittenRegionGaps = map[string]int{
	"rolematrix.templ": 1, // the two-column header band's gutter
}

func TestHandWrittenRegionGapsOnlyShrink(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	comment := regexp.MustCompile(`//[^\n]*`)
	got := map[string]int{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") || e.Name() == "layout.templ" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if n := len(regionGapToken.FindAllString(comment.ReplaceAllString(string(b), ""), -1)); n > 0 {
			got[e.Name()] = n
		}
	}
	for file, n := range got {
		if want := handWrittenRegionGaps[file]; n != want {
			t.Errorf("%s: %d hand-written gap-6, frozen at %d -- the space between a screen's large regions reads layoutGap(ctx, domain.GapRegion); a column gutter is argued here, and a lower count lowers the entry", file, n, want)
		}
	}
	for file := range handWrittenRegionGaps {
		if got[file] == 0 {
			t.Errorf("handWrittenRegionGaps names %s, which has no gap-6 left -- remove the entry", file)
		}
	}
}

// handWrittenCompactHeights freezes the `h-8` literals left after `controlHeightCompact` arrived (2026-10-12), the Design
// System's `control-height-compact`: 32px for a small control inside a row, **mouse only** (44px on touch). Its two
// sites, review's View PDF and Download links, moved, and on a touch screen they grew from 32 to 44px, measured in
// Chromium with touch emulation. What is left is the attachment kind tile: an aria-hidden label box, not a control.
// Shrink-only, both ways.
var handWrittenCompactHeights = map[string]int{
	"attachments.templ": 1, // the file-kind tile beside an attachment -- not a control
}

func TestHandWrittenCompactHeightsOnlyShrink(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	comment := regexp.MustCompile(`//[^\n]*`)
	tok := regexp.MustCompile(`(?:^|[\s"{])h-8(?:$|[\s"}])`)
	got := map[string]int{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") || e.Name() == "layout.templ" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if n := len(tok.FindAllString(comment.ReplaceAllString(string(b), ""), -1)); n > 0 {
			got[e.Name()] = n
		}
	}
	for file, n := range got {
		if want := handWrittenCompactHeights[file]; n != want {
			t.Errorf("%s: %d hand-written h-8, frozen at %d -- a small control in a row reads controlHeightCompact() (which keeps 44px on touch); a non-control is argued here, and a lower count lowers the entry", file, n, want)
		}
	}
	for file := range handWrittenCompactHeights {
		if got[file] == 0 {
			t.Errorf("handWrittenCompactHeights names %s, which has no h-8 left -- remove the entry", file)
		}
	}
}

// halfStepGapToken and regionGapToken are the two gap-value gates' patterns, at package level so G1
// (TestEveryThemeCategoryHasAnImplRatchet) probes the same pattern the gate counts with.
var (
	halfStepGapToken = regexp.MustCompile(`(?:^|[\s"{])gap-1\.5(?:$|[\s"}])`)
	regionGapToken   = regexp.MustCompile(`(?:^|[\s"{])gap-6(?:$|[\s"}])`)
)
