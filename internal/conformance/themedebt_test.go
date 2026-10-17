package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// themeDebt is one file's count of hand-written class tokens that a Theme role already names.
type themeDebt struct {
	size   int // text-3xs..text-2xl             -> textClass(ctx, domain.Role*) or staticText
	ink    int // text-slate-400/500/700/900      -> inkClass(ctx, domain.Ink*)
	weight int // font-medium, font-normal        -> weightClass(ctx, domain.Weight*)
	fill   int // bg-white                        -> backgroundClass(ctx, domain.BackgroundRaised) or surfaceClasses(ctx)
	// offLadder is text-[Npx]: a size **no role names**, so it is not debt against a reader. It is counted apart and
	// frozen in handWrittenOffLadderSizes, never in handWrittenThemeTokens.
	offLadder int
}

// handWrittenThemeTokens counts, per file, the class tokens a `theme:` block can already move but a `.templ` wrote
// as a literal -- the tenth directive ratchet, **shrink-only** on the same terms as its siblings (a higher number
// fails, a *lower* one fails until the entry drops, a new file fails, an entry left behind fails).
//
// **What it is for.** Every ratchet before it counts one *shape* (a chip, a muted paragraph, a surface) and each
// is blind to the next: a `<p>` migrated to `textClass`/`inkClass` left `TestHandWrittenMutedParagraphsOnlyShrink`
// by editing an attribute, and a screen written fresh with `text-sm text-slate-500` in a `<span>` was seen by
// none of them. This one counts the *tokens* regardless of element, so a screen that is written with the design
// system stays at zero and a screen that is not cannot appear unnoticed. A file absent from the map is a file
// with none, and the first literal in it fails.
//
// **Counting method, stated beside the number.** Syntactic: a `class="..."` value, or a `class={ ... }`
// expression reduced to its string literals by `joinClassExpressions` (without which a migration that splits a
// literal to splice a reader in would make a site vanish while staying hand-written), split on whitespace, with
// `//` comment lines removed. A token counts only if it is the *whole* utility and carries **no variant prefix**.
//
// **What it cannot see, on purpose or not:**
//   - a variant-prefixed token (`sm:text-sm`, `hover:text-slate-700`): interaction state and responsive overrides
//     are not roles (design-system-decisions, "Excluded on purpose"), and a gate counting them would call the
//     boundary working a miss;
//   - a class arriving from a Go function (`return "text-xs"` in a `.templ` helper, `tagClasses`, `appIconClasses`);
//   - `text-slate-300` and `text-slate-600`, `text-blue-*`, `bg-slate-50`/`bg-slate-100`: no ink role maps to
//     them here. The accent has its own gate since DS-D10 (TestHandWrittenAccentInksOnlyShrink), and the slate-50/100
//     wells theirs since W1 (TestHandWrittenWellFillsOnlyShrink); the comment that once said they had no role
//     without moving screens. A token with no role is a missing capability, not debt, and gating it would ask
//     for a Theme key nobody has decided;
//   - whether the *right* role was chosen. That a reader exists is what is checked, never that it is correct.
//
// **Since the gap plan's P5 (2026-10-14) the map holds only floors, 52 tokens in 18 files, each with its reason on its
// line** (read the counts out of the map): a link look whose size owner decision T-L keeps literal, a glyph box whose size
// is paired with a fixed circle (step numbers, initials, count badges), the 10px legends on a document marker, and one
// monospace block. 392 tokens in 31 files moved to `textClass`/`weightClass` by meaning, mechanically and then checked:
// `text-sm` is body, `text-xs` is meta, `text-2xs` is caption (label in a pill or an uppercase heading), `text-3xs` is
// the eyebrow, `font-medium` is the emphasis weight. Computed style over 103 pages at 1280 and 360px: 0 differences.
// **A role chosen by meaning is not checkable by a gate**, so the choice is recorded here; a different one is a decision.
//
// **Each remaining floor is a measurement of 2026-10-14, not a verdict.** The comment below is the 2026-10-11 original: They are what the tree held the day this was
// written, mostly pre-auth screens (no Workspace on ctx, so no theme can reach them) and sites that predate
// every Theme role. Read the entries out of the map; lower them as screens are touched.
var handWrittenThemeTokens = map[string]themeDebt{
	"account.templ":            {size: 1, ink: 0, weight: 0, fill: 0}, // floors: 1 10px marker legend (no role)
	"approvalinbox.templ":      {size: 2, ink: 0, weight: 0, fill: 0}, // floors: 1 link look (T-L), 1 glyph box (size paired with a fixed circle)
	"approvalstepper.templ":    {size: 3, ink: 0, weight: 0, fill: 0}, // floors: 3 glyph box (size paired with a fixed circle)
	"appshell.templ":           {size: 4, ink: 0, weight: 0, fill: 0}, // floors: 2 glyph box (size paired with a fixed circle), 2 link look (T-L)
	"attachments.templ":        {size: 2, ink: 0, weight: 0, fill: 0}, // floors: 1 glyph box (size paired with a fixed circle), 1 link look (T-L)
	"controls.templ":           {size: 2, ink: 0, weight: 0, fill: 0}, // floors: 2 link look (T-L)
	"detail.templ":             {size: 2, ink: 0, weight: 0, fill: 0}, // floors: 2 link look (T-L)
	"documentsubmit.templ":     {size: 3, ink: 0, weight: 0, fill: 0}, // floors: 2 link look (T-L), 1 glyph box (size paired with a fixed circle)
	"login.templ":              {size: 1, ink: 0, weight: 0, fill: 0}, // floors: 1 link look (T-L)
	"newapplication.templ":     {size: 4, ink: 0, weight: 0, fill: 0}, // floors: 4 link look (T-L)
	"reviewdocument.templ":     {size: 9, ink: 0, weight: 0, fill: 0}, // floors: 2 link look (T-L), 3 glyph box (size paired with a fixed circle), 4 10px marker legend (no role)
	"signatureplacement.templ": {size: 6, ink: 0, weight: 0, fill: 0}, // floors: 2 link look (T-L), 2 glyph box (size paired with a fixed circle), 2 10px marker legend (no role)
}

var (
	themeClassValue = regexp.MustCompile(`class="([^"]*)"`)
	themeSizeToken  = regexp.MustCompile(`^text-(?:3xs|2xs|xs|sm|item|base|xl|2xl)$`)
	themeOffLadder  = regexp.MustCompile(`^text-\[[\d.]+(?:px|rem|em)\]$`)
	themeInkToken   = regexp.MustCompile(`^text-slate-(?:400|500|700|900)$`)
	themeWeightTok  = regexp.MustCompile(`^font-(?:medium|normal)$`)
)

func countThemeTokens(src string) themeDebt {
	src = chipNoLine.ReplaceAllString(src, "")
	src = joinClassLiterals(src) // the raw form: this ratchet counts the size and weight literals themselves
	var d themeDebt
	for _, m := range themeClassValue.FindAllStringSubmatch(src, -1) {
		for _, tok := range strings.Fields(m[1]) {
			switch {
			case themeOffLadder.MatchString(tok):
				d.offLadder++
			case themeSizeToken.MatchString(tok):
				d.size++
			case themeInkToken.MatchString(tok):
				d.ink++
			case themeWeightTok.MatchString(tok):
				d.weight++
			case tok == "bg-white":
				d.fill++
			}
		}
	}
	return d
}

func TestHandWrittenThemeTokensOnlyShrink(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}

	actual := map[string]themeDebt{}
	offLadder := map[string]int{}
	var total themeDebt
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		d := countThemeTokens(string(b))
		if d.offLadder > 0 {
			offLadder[e.Name()] = d.offLadder
		}
		d.offLadder = 0
		if d != (themeDebt{}) {
			actual[e.Name()] = d
			total.size += d.size
			total.ink += d.ink
			total.weight += d.weight
			total.fill += d.fill
		}
	}

	kinds := []struct {
		name, use string
		get       func(themeDebt) int
	}{
		{"text size", "`textClass(ctx, domain.Role*)`, or `staticText(domain.Static*, ...)` where the text is a leaf", func(d themeDebt) int { return d.size }},
		{"text ink", "`inkClass(ctx, domain.Ink*)`", func(d themeDebt) int { return d.ink }},
		{"font weight", "`weightClass(ctx, domain.Weight*)`", func(d themeDebt) int { return d.weight }},
		{"white fill", "`surfaceClasses(ctx)` for a card, else `backgroundClass(ctx, domain.BackgroundRaised)`", func(d themeDebt) int { return d.fill }},
	}
	for _, f := range sortedThemeFiles(actual, handWrittenThemeTokens) {
		a, w := actual[f], handWrittenThemeTokens[f]
		for _, k := range kinds {
			switch got, want := k.get(a), k.get(w); {
			case got > want:
				t.Errorf("%s: %d hand-written %s token(s), frozen at %d -- read the Theme instead: %s. A Workspace's `theme:` block moves the reader and not the literal (006). If this site genuinely has no role, say which in handWrittenThemeTokens rather than widening it", f, got, k.name, want, k.use)
			case got < want:
				t.Errorf("%s: %d hand-written %s token(s), down from %d -- lower the entry in this file to lock the migration in", f, got, k.name, want)
			}
		}
	}

	checkOffLadderSizes(t, offLadder)

	var frozen themeDebt
	for _, d := range handWrittenThemeTokens {
		frozen.size += d.size
		frozen.ink += d.ink
		frozen.weight += d.weight
		frozen.fill += d.fill
	}
	if total != frozen {
		t.Logf("totals now: %+v; frozen: %+v", total, frozen)
	}
	if len(actual) == 0 {
		t.Fatal("no hand-written theme token found anywhere -- either every screen reads the Theme (say so in capabilities.md and keep this gate as an ordinary one) or the pattern has stopped matching")
	}
	if os.Getenv("PRINT_THEME_DEBT") != "" {
		for _, f := range sortedThemeFiles(actual, nil) {
			d := actual[f]
			fmt.Printf("\t%q: {size: %d, ink: %d, weight: %d, fill: %d},\n", f, d.size, d.ink, d.weight, d.fill)
		}
	}
}

// handWrittenOffLadderSizes freezes, per file, the `text-[Npx]` sizes (2026-10-12, rencana 0.1). They sat in the
// `size` bucket until then, which told a reader to "read the Theme instead" when **no Theme role names them**: the
// ladder went 14 (`body`) to 16 (`subheading`). Owner decision **DS-D13** (2026-10-12) added `item` (15px) for one
// item's title, and the ten authenticated titles moved to it.
//
// What stays has a different meaning: a **pre-auth** screen renders with no Workspace on ctx, so no role could reach
// it (it would need the pre-auth `auth-*` sizes mapped first), and an authenticated 15px left here is a control's
// mobile touch size, not an item title. Shrink-only both ways, on the terms of handWrittenThemeTokens.
var handWrittenOffLadderSizes = map[string]struct {
	n   int
	why string
}{
	"account.templ":          {5, "a field's or button's mobile size (15px below `sm:`, `sm:text-sm` above): a control's touch size, not an item title, so DS-D13's `item` role does not apply"},
	"appshell.templ":         {1, "the workspace monogram's 13px glyph in its 36px (`size-9`) tile; GS2's rejectedDesignExempt"},
	"authshell.templ":        {6, "pre-auth (4x 15px, the 21px and 26px brand/heading) -- no Workspace on ctx, no Theme reaches it"},
	"checkyouremail.templ":   {1, "pre-auth; 15px -- no Theme reaches it"},
	"chooseworkspace.templ":  {1, "pre-auth; 15px -- no Theme reaches it"},
	"login.templ":            {2, "pre-auth; 15px -- no Theme reaches it"},
	"workspacemembers.templ": {1, "the invite field's mobile size; see account.templ"},
}

func checkOffLadderSizes(t *testing.T, actual map[string]int) {
	t.Helper()
	files := map[string]themeDebt{}
	for f := range actual {
		files[f] = themeDebt{}
	}
	for f := range handWrittenOffLadderSizes {
		files[f] = themeDebt{}
	}
	for _, f := range sortedThemeFiles(files, nil) {
		got, want := actual[f], handWrittenOffLadderSizes[f].n
		switch {
		case got > want:
			t.Errorf("%s: %d off-ladder text size(s) (text-[Npx]), frozen at %d -- no Theme role names this size, so "+
				"pick a step of the ladder (textClass/staticText; an item's 15px title is RoleItem, DS-D13); a new "+
				"arbitrary size is a design decision, not a literal (menjaga-design-system R1)", f, got, want)
		case got < want:
			t.Errorf("%s: %d off-ladder text size(s), down from %d -- lower handWrittenOffLadderSizes to lock it in", f, got, want)
		}
	}
}

func sortedThemeFiles(a, b map[string]themeDebt) []string {
	union := map[string]themeDebt{}
	for k, v := range a {
		union[k] = v
	}
	for k, v := range b {
		union[k] = v
	}
	return sortedFileNames(union)
}
