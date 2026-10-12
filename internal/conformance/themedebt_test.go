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
	size   int // text-3xs..text-2xl, text-[Npx]  -> textClass(ctx, domain.Role*) or staticText
	ink    int // text-slate-400/500/700/900      -> inkClass(ctx, domain.Ink*)
	weight int // font-medium, font-normal        -> weightClass(ctx, domain.Weight*)
	fill   int // bg-white                        -> backgroundClass(ctx, domain.BackgroundRaised) or surfaceClasses(ctx)
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
//     them, and the slate-50/100 wells are recorded in domain.BackgroundRole as having no role that draws them
//     without moving screens. A token with no role is a missing capability, not debt, and gating it would ask
//     for a Theme key nobody has decided;
//   - whether the *right* role was chosen. That a reader exists is what is checked, never that it is correct.
//
// **Each floor is a measurement of 2026-10-11, not a verdict.** They are what the tree held the day this was
// written, mostly pre-auth screens (no Workspace on ctx, so no theme can reach them) and sites that predate
// every Theme role. Read the entries out of the map; lower them as screens are touched.
var handWrittenThemeTokens = map[string]themeDebt{
	"account.templ":            {size: 18, ink: 13, weight: 7, fill: 5},
	"appmembers.templ":         {size: 8, ink: 4, weight: 0, fill: 0},
	"approvalinbox.templ":      {size: 23, ink: 19, weight: 5, fill: 2},
	"approvalstepper.templ":    {size: 9, ink: 7, weight: 6, fill: 0},
	"appshell.templ":           {size: 50, ink: 51, weight: 22, fill: 0},
	"attachments.templ":        {size: 4, ink: 2, weight: 1, fill: 0},
	"authshell.templ":          {size: 9, ink: 6, weight: 3, fill: 2},
	"checklist.templ":          {size: 2, ink: 2, weight: 0, fill: 0},
	"checkyouremail.templ":     {size: 2, ink: 2, weight: 1, fill: 0},
	"chooseworkspace.templ":    {size: 8, ink: 8, weight: 2, fill: 1},
	"comments.templ":           {size: 4, ink: 3, weight: 1, fill: 0},
	"controls.templ":           {size: 5, ink: 3, weight: 0, fill: 0},
	"deleteaccount.templ":      {size: 4, ink: 3, weight: 4, fill: 0},
	"detail.templ":             {size: 22, ink: 14, weight: 7, fill: 0},
	"documentsubmit.templ":     {size: 12, ink: 8, weight: 5, fill: 0},
	"forgotpassword.templ":     {size: 1, ink: 2, weight: 0, fill: 0},
	"groups.templ":             {size: 15, ink: 9, weight: 2, fill: 1},
	"inference.templ":          {size: 8, ink: 16, weight: 0, fill: 0},
	"installapplication.templ": {size: 11, ink: 8, weight: 3, fill: 0},
	"layout.templ":             {size: 4, ink: 0, weight: 0, fill: 0},
	"login.templ":              {size: 5, ink: 2, weight: 2, fill: 2},
	"machine.templ":            {size: 23, ink: 19, weight: 8, fill: 1},
	"newapplication.templ":     {size: 25, ink: 19, weight: 0, fill: 1},
	"notifications.templ":      {size: 5, ink: 5, weight: 2, fill: 2},
	"progress.templ":           {size: 2, ink: 1, weight: 0, fill: 0},
	"resendverification.templ": {size: 1, ink: 2, weight: 0, fill: 0},
	"reviewdocument.templ":     {size: 26, ink: 16, weight: 4, fill: 2},
	"signatureplacement.templ": {size: 14, ink: 8, weight: 3, fill: 0},
	"table.templ":              {size: 1, ink: 0, weight: 0, fill: 0},
	"workspacefiles.templ":     {size: 8, ink: 3, weight: 0, fill: 0},
	"workspacehome.templ":      {size: 25, ink: 22, weight: 8, fill: 5},
	"workspacemembers.templ":   {size: 32, ink: 18, weight: 6, fill: 3},
	"workspacesettings.templ":  {size: 7, ink: 4, weight: 3, fill: 0},
}

var (
	themeClassValue = regexp.MustCompile(`class="([^"]*)"`)
	themeSizeToken  = regexp.MustCompile(`^text-(?:3xs|2xs|xs|sm|base|xl|2xl|\[\d+px\])$`)
	themeInkToken   = regexp.MustCompile(`^text-slate-(?:400|500|700|900)$`)
	themeWeightTok  = regexp.MustCompile(`^font-(?:medium|normal)$`)
)

func countThemeTokens(src string) themeDebt {
	src = chipNoLine.ReplaceAllString(src, "")
	src = joinClassExpressions(src)
	var d themeDebt
	for _, m := range themeClassValue.FindAllStringSubmatch(src, -1) {
		for _, tok := range strings.Fields(m[1]) {
			switch {
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
	var total themeDebt
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if d := countThemeTokens(string(b)); d != (themeDebt{}) {
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
