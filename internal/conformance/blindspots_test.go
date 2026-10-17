package conformance

import (
	"regexp"
	"sort"
	"testing"
)

// The four ratchets below close blind spots the 2026-10-14 design-system audit measured, in the shape this repo uses for a
// population it cannot yet migrate (gap plan P7, 2026-10-15): freeze what is there per file, so a new one fails and a
// removed one fails until the entry drops. They count `.templ` sources with comments stripped, token by token, and are
// frozen at what the audit measured. None says the tokens are wrong; each says nothing new may arrive unnoticed.

// handWrittenVariantTokens counts a size, gap, corner, line-height or padding written under a breakpoint (`sm:gap-4`,
// `sm:rounded-lg`). The Theme's readers answer one class per role and a `sm:` override is not a role, so a Workspace's theme
// moves the small-screen value and not the large-screen one at these sites. 43 across 17 files; most are the pre-auth
// screens (no Theme reaches them) and the app shell's header and sheet, where a responsive change of size is the layout.
var handWrittenVariantTokens = map[string]int{
	"acceptinvite.templ": 1, "account.templ": 5, "appshell.templ": 13, "authshell.templ": 4, "checkyouremail.templ": 1,
	"controls.templ": 1, "createworkspace.templ": 1, "deleteaccount.templ": 1, "forgotpassword.templ": 1, "layout.templ": 1,
	"login.templ": 4, "register.templ": 1, "resendverification.templ": 1, "resetpassword.templ": 1, "rolematrix.templ": 1,
	"workspacemembers.templ": 1,
}

// handWrittenLeading counts a `leading-*` line-height written beside a text role. A role's size comes with its own line height
// (the artifact's type styles pair them), so each is a deliberate override: a roomier description (`leading-6`), a tight
// single line (`leading-4`), a measure for a sentence (`leading-5`). 31 across 12 files, `rolematrix` the largest.
var handWrittenLeading = map[string]int{
	"appsettings.templ": 3, "appshell.templ": 2, "authshell.templ": 1, "chooseworkspace.templ": 1,
	"machine.templ": 1, "reviewdocument.templ": 2, "rolematrix.templ": 7, "signatureplacement.templ": 1,
	"workspacefiles.templ": 3,
}

// handWrittenTracking counts `tracking-wide`, the artifact's 0.025em on eyebrows and table heads. It is not a Theme role (the
// type styles name it beside their size), so every uppercase label writes it. 16 across 10 `.templ` files.
var handWrittenTracking = map[string]int{
	"approvalinbox.templ": 1, "appsettings.templ": 1, "appshell.templ": 3, "attachments.templ": 1, "controls.templ": 1,
	"detail.templ": 2, "inference.templ": 1, "layout.templ": 2, "rolematrix.templ": 1, "workspacesettings.templ": 1,
}

// allowedShadows is the closed set of shadow utilities per file. The artifact has four shadows: `shadow-popover` (Tailwind's
// `shadow-lg`: a menu, a dialog, a toast, the desktop "More" panel), `shadow-hover` (`shadow-sm` on hover) and two written as
// arbitrary values (`shadow-card-auth`, `shadow-sheet`), whose numbers `TestUnmappedShadowTokensMatchTheirCode` compares. One
// use is not the artifact's: `open:shadow-md`, the board card's quick edit lifting the card it expands.
var allowedShadows = map[string][]string{
	"appshell.templ":  {"shadow-[0_-8px_24px_rgba(15,23,42,0.16)]", "sm:shadow-lg"},
	"authshell.templ": {"shadow-[0_1px_2px_rgba(15,23,42,0.04)]"},
	"layout.templ":    {"shadow-lg"},
	"machine.templ":   {"hover:shadow-sm", "open:shadow-md"},
	"popover.templ":   {"shadow-lg"},
}

var (
	variantTokenRe = regexp.MustCompile("(?:^|[\\s\"`])((?:sm|md|lg|xl):(?:text|gap|rounded|leading|p|px|py|pt|pb)[a-z0-9.\\[\\]/-]*)(?:[\\s\"`]|$)")
	leadingRe      = regexp.MustCompile("(?:^|[\\s\"`])(leading-[a-z0-9.\\[\\]]+)(?:[\\s\"`]|$)")
	trackingRe     = regexp.MustCompile("(?:^|[\\s\"`])(tracking-[a-z0-9.\\[\\]-]+)(?:[\\s\"`]|$)")
	shadowRe       = regexp.MustCompile("(?:^|[\\s\"`])((?:[a-z0-9\\[\\]&:-]+:)*shadow-[^\\s\"`{}]+)(?:[\\s\"`]|$)")
)

// countTokens counts the matches of re per .templ file.
func countTokens(t *testing.T, re *regexp.Regexp) map[string]int {
	t.Helper()
	out := map[string]int{}
	for name, raw := range renderingTemplSources(t) {
		if n := len(re.FindAllStringSubmatch(stripTemplComments(raw), -1)); n > 0 {
			out[name] = n
		}
	}
	return out
}

// checkFrozen fails for a count above its entry, below it, a new file and a stale entry.
func checkFrozen(t *testing.T, what, advice string, got, frozen map[string]int) {
	t.Helper()
	names := map[string]bool{}
	for n := range got {
		names[n] = true
	}
	for n := range frozen {
		names[n] = true
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)
	for _, n := range sorted {
		switch g, f := got[n], frozen[n]; {
		case g > f:
			t.Errorf("%s: %d %s, frozen at %d -- %s", n, g, what, f, advice)
		case g < f:
			t.Errorf("%s: %d %s, down from %d -- lower the entry to lock the migration in", n, g, what, f)
		}
	}
}

func TestHandWrittenVariantTokensOnlyShrink(t *testing.T) {
	got := countTokens(t, variantTokenRe)
	if len(got) < 10 {
		t.Fatalf("only %d files hold a breakpoint-prefixed size, gap, corner or padding -- the scan is measuring less than it appears to", len(got))
	}
	checkFrozen(t, "breakpoint-prefixed design tokens", "a `sm:` override is not a Theme role, so a Workspace's theme moves the small-screen value and not this one; say why in this map", got, handWrittenVariantTokens)
}

func TestHandWrittenLeadingOnlyShrink(t *testing.T) {
	got := countTokens(t, leadingRe)
	if len(got) < 8 {
		t.Fatalf("only %d files hold a `leading-*`, the scan is measuring less than it appears to", len(got))
	}
	checkFrozen(t, "`leading-*` overrides", "a text role brings its own line height (the artifact pairs them); an override is a decision, say why in this map", got, handWrittenLeading)
}

func TestHandWrittenTrackingOnlyShrink(t *testing.T) {
	got := countTokens(t, trackingRe)
	if len(got) < 6 {
		t.Fatalf("only %d files hold a `tracking-*`, the scan is measuring less than it appears to", len(got))
	}
	checkFrozen(t, "`tracking-*` tokens", "the artifact's eyebrow and table head carry 0.025em beside their size and no Theme role names it; a new uppercase label is a new entry", got, handWrittenTracking)
}

func TestShadowsAreOnlyTheArtifactsFour(t *testing.T) {
	seen := map[string]map[string]bool{}
	total := 0
	for name, raw := range renderingTemplSources(t) {
		for _, m := range shadowRe.FindAllStringSubmatch(stripTemplComments(raw), -1) {
			if seen[name] == nil {
				seen[name] = map[string]bool{}
			}
			seen[name][m[1]] = true
			total++
		}
	}
	if total < 6 {
		t.Fatalf("only %d shadow utilities were found -- the scan is measuring less than it appears to", total)
	}
	for name, set := range seen {
		allowed := map[string]bool{}
		for _, a := range allowedShadows[name] {
			allowed[a] = true
		}
		for tok := range set {
			if !allowed[tok] {
				t.Errorf("%s writes `%s`, which is not one of the artifact's four shadows (shadow-popover, shadow-hover, shadow-card-auth, shadow-sheet) -- use one, or add it to allowedShadows with a reason", name, tok)
			}
		}
	}
	for name, list := range allowedShadows {
		for _, a := range list {
			if !seen[name][a] {
				t.Errorf("allowedShadows lists `%s` in %s, which no longer writes it -- remove the entry", a, name)
			}
		}
	}
}

// handWrittenSVGHex is the closed list of `stroke`/`fill` written as a hex colour in an SVG attribute, which every class-reading
// colour gate is blind to (the audit found `#16a34a`, a green that was in no palette). Three remain: the white check mark
// inside a filled marker on the paper layer, which is fixed in both modes (DK9).
var handWrittenSVGHex = map[string]int{"reviewdocument.templ": 3}

func TestSVGColoursAreNotHexLiterals(t *testing.T) {
	re := regexp.MustCompile(`\b(?:stroke|fill)="#[0-9a-fA-F]{3,8}"`)
	got := countTokens(t, re)
	checkFrozen(t, "hex colours in an SVG attribute", "draw it with currentColor over a class that reads a colour role (or mark-good), so a gate can see it", got, handWrittenSVGHex)
	if len(handWrittenSVGHex) == 0 {
		t.Fatal("handWrittenSVGHex is empty -- delete this check's sanity floor, not the gate")
	}
}

// handWrittenControlHeights counts a control height written as a literal (`h-9`, `h-10.5`, `h-11`, with `sm:` and `pointer-coarse:`
// variants). The artifact's `control-height` (36px) has no Theme role and no reader, and the audit found two mechanisms for the
// touch size (`pointer-coarse:h-11` and a viewport pair, `h-10.5 sm:h-9`). 24 across 10 files: the fields and buttons at 36px, the
// 42px mobile fields on the account and sign-in screens, and the 44px reveal buttons. A new control height is a new entry.
var handWrittenControlHeights = map[string]int{
	"account.templ": 8, "authshell.templ": 4, "chooseworkspace.templ": 1, "controls.templ": 2, "layout.templ": 1,
	"login.templ": 2, "newapplication.templ": 1, "reviewdocument.templ": 1, "workspacemembers.templ": 1,
}

func TestHandWrittenControlHeightsOnlyShrink(t *testing.T) {
	re := regexp.MustCompile("(?:^|[\\s\"`])((?:sm:|pointer-coarse:)?h-(?:9|10|10\\.5|11))(?:[\\s\"`]|$)")
	got := countTokens(t, re)
	if len(got) < 6 {
		t.Fatalf("only %d files hold a control height, the scan is measuring less than it appears to", len(got))
	}
	checkFrozen(t, "literal control heights", "the artifact's control-height is 36px and 44px on touch for the compact control; use controlHeightCompact() or say why in this map", got, handWrittenControlHeights)
}
