package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// handWrittenMutedParagraphs counts the muted sentences still drawn as a hand-written `<p>` instead of
// `staticText(domain.StaticMessage|StaticNote, ...)`, per file, **shrink-only** -- the third directive ratchet,
// built after eleven sites moved (2026-10-06; build, migrate, then gate).
//
// **Every floor here is a `<p>` carrying something `staticText` must not accept**, and none is debt -- the
// boundary working, on the same terms as `row`'s floors:
//
//   - `approvalinbox.templ` (2): a sentence holding an inline `<span>` and a conditional tail. Inline markup
//     inside a text run is exactly what `staticText` does not draw (007 §12.6 static content is a leaf).
//   - `appsettings.templ` (2), `installapplication.templ`, `workspacesettings.templ`: a page's description at
//     `leading-6`. Four sites sharing a line-height no text role names -- a **finding**, not a miss: if a role
//     for it is ever declared, these four are the first consumers.
//   - `dashboard.templ`, `rolematrix.templ` (3): the sentence carries its own spacing (`py-3`, `mt-1.5`, `px-4
//     py-4`) or width (`max-w-[760px]`). Spacing and width are Layout's (§12.2), and a Static kind accepting them
//     is §12.3's "arbitrary properties".
//   - `inference.templ` (3): two add `max-w-prose`; the third is a multi-line sentence whose surrounding
//     whitespace is part of today's output.
//   - `newapplication.templ` (1), `workspacemembers.templ` (1): an interpolated sentence carrying `'` or `"`.
//     templ writes a literal quote raw and an expression's quote escaped (`&#34;`), so migrating changes the
//     bytes -- equivalent HTML, not identical, and a migration whose diff is not empty is reported, not hidden.
//
// **The first draft of this gate caught its own author's count.** The census that chose these two kinds matched
// the exact literal `<p class="m-0 text-sm text-slate-500">` and found 11 migratable sites; the gate's pattern
// (a class *set*, any order, any extras) found 15 more that the literal could not see. Counting what is present
// may be syntactic, but the *shape* of the pattern decides what is counted.
//
// **Counting method, stated beside the number.** Syntactic and deliberately narrow: an opening `<p>` whose class
// holds `m-0`, `text-sm` or `text-xs`, and `text-slate-500`. It cannot see a muted sentence in a `<div>`/`<span>`,
// one at `text-2xs` (16 sites in 6 files by exact literal, a caption that no text role names -- see domain.StaticNote), or one
// using another ink shade. Concluding a muted sentence is absent still needs reading the screen.
var handWrittenMutedParagraphs = map[string]int{
	"appsettings.templ":        2,
	"approvalinbox.templ":      2,
	"dashboard.templ":          1,
	"inference.templ":          3,
	"installapplication.templ": 1,
	"newapplication.templ":     1,
	// 4th: board 06's closing sentence carries an inline link to the Members screen, which a Static kind must not accept.
	"rolematrix.templ":        4,
	"workspacemembers.templ":  1,
	"workspacesettings.templ": 1,
}

var (
	mutedParaTag  = regexp.MustCompile(`(?s)<p\b[^>]*\bclass="([^"]*)"`)
	mutedParaM0   = regexp.MustCompile(`(?:^|\s)m-0(?:\s|$)`)
	mutedParaSize = regexp.MustCompile(`(?:^|\s)text-(?:sm|xs)(?:\s|$)`)
	mutedParaInk  = regexp.MustCompile(`(?:^|\s)text-slate-500(?:\s|$)`)
	mutedParaNote = regexp.MustCompile(`(?m)^\s*//.*$`)
)

func TestHandWrittenMutedParagraphsOnlyShrink(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}

	actual := map[string]int{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		src := mutedParaNote.ReplaceAllString(string(b), "")
		for _, m := range mutedParaTag.FindAllStringSubmatch(src, -1) {
			if mutedParaM0.MatchString(m[1]) && mutedParaSize.MatchString(m[1]) && mutedParaInk.MatchString(m[1]) {
				actual[e.Name()]++
			}
		}
	}

	for _, f := range sortedFileNames(actual) {
		switch w := handWrittenMutedParagraphs[f]; {
		case actual[f] > w:
			t.Errorf("%s: %d hand-written muted paragraph(s), frozen at %d -- use `@staticText(domain.StaticMessage, text)` (a sentence standing in for content) or `domain.StaticNote` (a footnote about it). If this one carries inline markup or a width the Static kinds must not accept, say which in handWrittenMutedParagraphs rather than widening it", f, actual[f], w)
		case actual[f] < w:
			t.Errorf("%s: %d hand-written muted paragraph(s), down from %d -- lower the entry in this file to lock the migration in", f, actual[f], w)
		}
	}
	for _, f := range sortedFileNames(handWrittenMutedParagraphs) {
		if _, still := actual[f]; !still {
			t.Errorf("handWrittenMutedParagraphs still lists %s, which no longer has one -- remove the entry", f)
		}
	}
	if len(actual) == 0 {
		t.Fatal("no hand-written muted paragraph found anywhere -- either every one uses staticText (say so in capabilities.md and keep this gate as an ordinary one) or the pattern has stopped matching")
	}
}

var subheadingTag = regexp.MustCompile(`(?s)<h[1-6]\b((?:[^>{]|\{[^}]*\})*)>`)

// TestNoHandWrittenSubheading holds the floor at **zero**: a heading element at `text-base` is a section's own
// title and goes through `@staticText(domain.StaticSubheading, ...)`, so a Workspace declaring
// `theme.text.subheading` moves all of them together. Counted syntactically over `<h1>`..`<h6>` with `//`
// comments stripped; what it cannot see is a section title drawn as a `<span>` or `<div>` (two `text-base`
// non-headings exist and are not this role: a menu label and a monogram), and the 23 body-sized `<h2>` panel
// titles are a different job it deliberately does not claim.
func TestNoHandWrittenSubheading(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		src := mutedParaNote.ReplaceAllString(string(b), "")
		for _, m := range subheadingTag.FindAllStringSubmatch(src, -1) {
			if regexp.MustCompile(`(?:^|[\s"])text-base(?:\s|")`).MatchString(m[1]) {
				t.Errorf("%s: a hand-written heading at text-base -- use `@staticText(domain.StaticSubheading, text)` (007 §12.6; theme.text.subheading)", e.Name())
			}
		}
	}
}

var eyebrowSpanTag = regexp.MustCompile(`(?s)<span\b((?:[^>{]|\{[^}]*\})*)>`)

// TestNoHandWrittenHeadingOrEyebrow holds four floors (the faint overline and the panel title joined 2026-10-06) at **zero**: a heading element at `text-xl` is a screen's
// own title and goes through `@staticText(domain.StaticHeading, ...)`, and the blue `text-3xs` uppercase span is
// `StaticEyebrow`, so `theme.text.heading` / `theme.text.eyebrow` move all of them together (both roles were
// declarable and read by nothing until 2026-10-06). Counted syntactically with `//` comments stripped.
// Deliberately **not** claimed: the two `<h1 text-2xl>` (a larger title, its own measurement), and appshell's
// `font-medium` "Workspace" label, which carries a weight the Eyebrow kind does not accept.
func TestNoHandWrittenHeadingOrEyebrow(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	hasToken := func(attrs, tok string) bool {
		return regexp.MustCompile(`(?:^|[\s"])` + regexp.QuoteMeta(tok) + `(?:\s|")`).MatchString(attrs)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		src := mutedParaNote.ReplaceAllString(string(b), "")
		for _, m := range subheadingTag.FindAllStringSubmatch(src, -1) {
			if hasToken(m[1], "text-xl") {
				t.Errorf("%s: a hand-written heading at text-xl -- use `@staticText(domain.StaticHeading, text)` (007 §12.6; theme.text.heading)", e.Name())
			}
		}
		for _, m := range subheadingTag.FindAllStringSubmatch(src, -1) {
			if strings.HasPrefix(strings.TrimSpace(m[0]), "<h2") && hasToken(m[1], "text-sm") && hasToken(m[1], "font-medium") &&
				hasToken(m[1], "m-0") && !hasToken(m[1], "text-red-700") {
				t.Errorf("%s: a hand-written panel title -- use `@staticText(domain.StaticPanelHeading, text)` (007 §12.6; theme.text.body + emphasis weight)", e.Name())
			}
		}
		for _, m := range eyebrowSpanTag.FindAllStringSubmatch(src, -1) {
			if hasToken(m[1], "text-3xs") && hasToken(m[1], "tracking-wide") && hasToken(m[1], "text-blue-600") &&
				hasToken(m[1], "uppercase") && !hasToken(m[1], "font-medium") {
				t.Errorf("%s: a hand-written blue eyebrow -- use `@staticText(domain.StaticEyebrow, text)` (007 §12.6; theme.text.eyebrow)", e.Name())
			}
			if hasToken(m[1], "text-3xs") && hasToken(m[1], "tracking-wide") && hasToken(m[1], "text-slate-400") &&
				hasToken(m[1], "uppercase") && !hasToken(m[1], "font-medium") {
				t.Errorf("%s: a hand-written faint overline -- use `@staticText(domain.StaticOverline, text)` (007 §12.6; theme.text.eyebrow, ink faint)", e.Name())
			}
		}
	}
}

// handWrittenCaptions counts the help lines still drawn as a hand-written `<p>` instead of
// `staticText(domain.StaticCaption, ...)`, per file, **shrink-only** -- built after eight sites moved
// (2026-10-06). The population is counted by class *set* (a `<p>` holding `m-0`, `text-2xs`, `text-slate-500`, any
// extras), not by the exact literal the census used: the literal found 16, the set finds more, and the
// difference is the lesson `handWrittenMutedParagraphs` already records.
//
// **Every floor carries something `StaticCaption` must not accept**, none is debt:
//   - plain-text sentences with an apostrophe (`account`, `groups`): `{ text }` escapes `'` to `&#39;`, equivalent
//     HTML and not identical bytes.
//   - multi-line or conditional text, or text with an inline `<span>`/expression run (`signatureplacement`,
//     `workspacemembers`, `documentsubmit`, `account`'s "Not enabled (planned)"): whitespace and markup inside the
//     run are part of today's output, and a Static kind is a leaf.
//   - extra classes (`mb-2`, `leading-5`, `rounded-md bg-slate-50 p-3`): spacing and surfaces are Layout's (§12.2).
//
// Counting method: syntactic over the opening `<p>` with `//` comments stripped. It cannot see a caption drawn as
// a `<span>`/`<div>`/`<label>` (20+ exist and several are not captions at all), so it states what is present
// and concludes nothing about absence.
var handWrittenCaptions = map[string]int{
	"account.templ":            3, // :78 `mb-2` spacing; :127 apostrophe escapes to &#39;; :161 inline <span>
	"documentsubmit.templ":     2, // :102 multi-line with an expression run; :190 multi-line text
	"groups.templ":             1, // apostrophe ("Group's")
	"reviewdocument.templ":     1, // `leading-5` plus an inline staticLink
	"signatureplacement.templ": 2, // :157 multi-line text; :168 bordered well (`rounded-md bg-slate-50 p-3 leading-5`)
	"workspacemembers.templ":   2, // :271 conditional branches; :337 apostrophe, multi-line
}

var captionPara = regexp.MustCompile(`(?s)<p\b[^>]*\bclass="([^"]*)"`)

func TestHandWrittenCaptionsOnlyShrink(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	has := func(attrs, tok string) bool {
		return regexp.MustCompile(`(?:^|\s)` + regexp.QuoteMeta(tok) + `(?:\s|$)`).MatchString(attrs)
	}
	actual := map[string]int{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		src := mutedParaNote.ReplaceAllString(string(b), "")
		for _, m := range captionPara.FindAllStringSubmatch(src, -1) {
			if has(m[1], "m-0") && has(m[1], "text-2xs") && has(m[1], "text-slate-500") {
				actual[e.Name()]++
			}
		}
	}
	for _, f := range sortedFileNames(actual) {
		switch w := handWrittenCaptions[f]; {
		case actual[f] > w:
			t.Errorf("%s: %d hand-written caption(s), frozen at %d -- use `@staticText(domain.StaticCaption, text)`. If this one carries markup, a conditional, a quote or spacing the kind must not accept, say which in handWrittenCaptions rather than widening it", f, actual[f], w)
		case actual[f] < w:
			t.Errorf("%s: %d hand-written caption(s), down from %d -- lower the entry in this file to lock the migration in", f, actual[f], w)
		}
	}
	for _, f := range sortedFileNames(handWrittenCaptions) {
		if _, still := actual[f]; !still {
			t.Errorf("handWrittenCaptions still lists %s, which no longer has one -- remove the entry", f)
		}
	}
}

// handWrittenButtonFloors are the `.templ` sites that still style a button-shaped element by calling
// buttonClasses directly instead of `@button(...)`. Each is a floor with its reason, not debt: the Button
// Component's contract accepts a label, a variant and a name/value pair, and these carry something it must not
// (007 §12.3 -- a Component may not accept arbitrary properties). Counted 2026-10-06 by `buttonClasses(` in
// each .templ outside controls.templ, `//` lines removed.
var handWrittenButtonFloors = map[string]int{
	"approvalinbox.templ":      1, // hx-* wiring on the button
	"attachments.templ":        1, // a <label> wrapping a file input, not a <button>
	"detail.templ":             5, // an <a> anchor (2), hx-* buttons (2), a <summary> disclosure trigger
	"machine.templ":            5, // the onclick Cancel, hx-* buttons, and per-site mr-1 spacing
	"newapplication.templ":     2, // id="send-btn" (a script hook) and an <a> with justify-center
	"reviewdocument.templ":     3, // sig-* hook classes and data-modal: control behaviour, not a name/value pair
	"signatureplacement.templ": 1, // hx-* wiring on the button
}

// handWrittenButtonLiterals are `<button class="h-9 ...">` written out by hand, with no buttonClasses call.
// They are NOT the Button Component's shape -- `px-3` against its `px-4`, and a slate-300 outline against its
// slate-200 -- so folding them in would shift pixels. **D8 was decided 2026-10-07 (outline = `border.surface`),
// and it did not move these four into `@button`**: three read `border.control` and `radius.control` now (a slate-300
// outline is a control's rim, kept as it was) and stay floors; the fourth is a red `Deactivate` that stays literal. They are counted after
// `joinClassExpressions`, because splicing a reader into the class made them invisible to the old pattern.
var handWrittenButtonLiterals = map[string]int{
	"chooseworkspace.templ":  1,
	"notifications.templ":    1,
	"workspacemembers.templ": 2,
}

var (
	buttonClassesCall = regexp.MustCompile(`buttonClasses\(`)
	buttonLiteralOpen = regexp.MustCompile(`(?s)<button\b[^>]*?\bclass="([^"]*)"`)
)

func TestHandWrittenButtonsOnlyShrink(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	has := func(attrs, tok string) bool {
		return regexp.MustCompile(`(?:^|\s)` + regexp.QuoteMeta(tok) + `(?:\s|$)`).MatchString(attrs)
	}
	floors, literals := map[string]int{}, map[string]int{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		src := mutedParaNote.ReplaceAllString(string(b), "")
		if e.Name() != "controls.templ" {
			if n := len(buttonClassesCall.FindAllString(src, -1)); n > 0 {
				floors[e.Name()] = n
			}
		}
		for _, m := range buttonLiteralOpen.FindAllStringSubmatch(joinClassExpressions(src), -1) {
			if has(m[1], "h-9") && (has(m[1], "bg-white") || has(m[1], "bg-slate-900")) {
				literals[e.Name()]++
			}
		}
	}
	check := func(label, use string, actual, frozen map[string]int) {
		for _, f := range sortedFileNames(actual) {
			switch w := frozen[f]; {
			case actual[f] > w:
				t.Errorf("%s: %d %s, frozen at %d -- %s", f, actual[f], label, w, use)
			case actual[f] < w:
				t.Errorf("%s: %d %s, down from %d -- lower the entry in this file to lock the migration in", f, actual[f], label, w)
			}
		}
		for _, f := range sortedFileNames(frozen) {
			if _, still := actual[f]; !still {
				t.Errorf("the %s list still names %s, which no longer has one -- remove the entry", label, f)
			}
		}
	}
	check("buttonClasses call(s)", "use `@button(label, variant, name, value)`. If this one carries hx-* wiring, an href, a hook class or spacing the Component must not accept, say so in handWrittenButtonFloors rather than widening the contract", floors, handWrittenButtonFloors)
	check("hand-written h-9 button(s)", "use `@button(...)`; a literal button is the shape the Component replaces", literals, handWrittenButtonLiterals)
}

// handWrittenFieldFloors are the `.templ` sites that draw an `<input>`/`<select>`/`<textarea>` with a literal
// `border-slate-300` instead of reading `border.control`. **The four that remain are all on pre-auth screens, and
// that is the reason rather than the 15px size** (2026-10-07): `currentWorkspace` is installed only inside the
// authenticated route group (`internal/web/router.go`), so sign in, register, password reset, accept-invite and choose-workspace
// render with no Workspace on ctx and no `theme:` block can reach them. The earlier note called all seven "a size is
// not a Theme role yet"; the three authenticated ones (account's name field, the invite-role select, the signature
// width box) never needed the size to be a role -- they kept `h-10.5`/`text-[15px]` as literals and now read
// `radius.control`, `border.control` and `ink.strong` around them. Counted by the tag's own class token set after
// `joinClassExpressions` (so a spliced site that still carries the literal is seen), `//` lines removed. **What the
// pattern cannot see**, stated rather than hidden: an input whose class arrives from a Go function
// (`approverUserClass` reads `fieldClasses`, so it is fine), and the read-only email box in account.templ, which is
// slate-200 on slate-50 -- a disabled look, not an input's own box.
//
// **These four are a boundary, not debt, and the entry must not be "fixed" by calling `fieldClasses(ctx)` here.**
// That call would compile and drop this count to zero -- a ctx with no Workspace falls back to `DefaultTheme()` --
// while no declaration could ever reach the page: a ratchet falling by an edit to an attribute, not by a
// capability. The way back is a runtime-level theme loaded before any Workspace exists (a new metadata key, loader
// support and the installer mirror), worth building only when a cross-Workspace branding need is real; there is
// no such case in `menata-app-document`'s `case-portfolio.md` (grepped for brand|theme|logo|login, 2026-10-07: one unrelated hit). Until then the four stay literal and say so.
var handWrittenFieldFloors = map[string]int{
	"authshell.templ": 2, // pre-auth kit (authField, authPasswordField); no Workspace on ctx, 007 §12.6 has no runtime-level Theme
	"login.templ":     2, // pre-auth sign-in; the same boundary, and a 15px mobile target the kit shares
}

var fieldTagOpen = regexp.MustCompile(`(?s)<(?:input|select|textarea)\b(?:[^>{]|\{[^}]*\})*>`)
var fieldTagClass = regexp.MustCompile(`\bclass="([^"]*)"`)

func TestHandWrittenFieldsOnlyShrink(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	has300 := regexp.MustCompile(`(?:^|\s)border-slate-300(?:\s|$)`)
	actual := map[string]int{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		src := joinClassExpressions(mutedParaNote.ReplaceAllString(string(b), ""))
		for _, tag := range fieldTagOpen.FindAllString(src, -1) {
			if m := fieldTagClass.FindStringSubmatch(tag); m != nil && has300.MatchString(m[1]) {
				actual[e.Name()]++
			}
		}
	}
	for _, f := range sortedFileNames(actual) {
		switch w := handWrittenFieldFloors[f]; {
		case actual[f] > w:
			t.Errorf("%s: %d hand-written field(s), frozen at %d -- use `fieldClasses(ctx)` so `theme.radius.control`, `theme.border.control` and the ink roles move it; if this one carries a size or spacing the reader must not accept, say so in handWrittenFieldFloors", f, actual[f], w)
		case actual[f] < w:
			t.Errorf("%s: %d hand-written field(s), down from %d -- lower the entry in this file to lock the migration in", f, actual[f], w)
		}
	}
	for _, f := range sortedFileNames(handWrittenFieldFloors) {
		if _, still := actual[f]; !still {
			t.Errorf("handWrittenFieldFloors still names %s, which no longer has one -- remove the entry", f)
		}
	}
}

// handWrittenDividerFloors are the `.templ` sites that still write the divider shade (`border-slate-100`,
// `divide-slate-100`) or a reset to normal weight (`font-normal`) as a literal instead of reading
// `borderClass(ctx, domain.BorderDivider)` / `weightClass(ctx, domain.WeightBody)`. Counted 2026-10-06 over each
// file's tokens with `//` lines removed, `layout.templ` excluded because it is where the readers live. Each is a
// floor with its reason, and none is a missing capability:
//   - appshell.templ, inference.templ, newapplication.templ: a rule between a *region* and the next (the nav strip
//     under the header, a heading's underline, a card body above its footer), not between rows of a list or table.
//     `BorderDivider`'s own comment says rows; reading it here would make the role mean two things.
//   - mytasks.templ: `divide-y divide-slate-100` is Tailwind's sibling-selector form, a different class family from
//     `border-*`; a second reader for one site is the "second ladder for two sites" boundary.
//
// **The measurement this slice made has a blind spot worth stating**: the other ratchets read `class="..."`, and
// splicing a Theme reader into a literal turns it into `class={ "a", reader(ctx), "b" }`. Three layout sites left
// `TestHandWrittenLayoutSitesOnlyShrink`'s count that way without being migrated, so it now joins class
// expressions first (`joinClassExpressions`). The chip, field, button and paragraph ratchets were not widened: none
// of their sites was touched here, and widening them is the same one-line change when one is.
var handWrittenDividerFloors = map[string]int{
	"appshell.templ":       1, // region rule under the header
	"inference.templ":      1, // heading underline
	"mytasks.templ":        1, // divide-y utility
	"newapplication.templ": 1, // card body / footer rule
}

var dividerToken = regexp.MustCompile(`(?:^|[\s"])(?:(?:border|divide)-slate-100|font-normal)(?:[\s"]|$)`)

func TestHandWrittenDividersOnlyShrink(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	comment := regexp.MustCompile(`//[^\n]*`)
	actual := map[string]int{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") || e.Name() == "layout.templ" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if n := len(dividerToken.FindAllString(comment.ReplaceAllString(string(b), ""), -1)); n > 0 {
			actual[e.Name()] = n
		}
	}
	for _, f := range sortedFileNames(actual) {
		switch w := handWrittenDividerFloors[f]; {
		case actual[f] > w:
			t.Errorf("%s: %d literal divider/normal-weight token(s), frozen at %d -- read `borderClass(ctx, domain.BorderDivider)` or `weightClass(ctx, domain.WeightBody)` so `theme.border.divider` / `theme.weight.body` move it; a site that is not a row separator goes in handWrittenDividerFloors with its reason", f, actual[f], w)
		case actual[f] < w:
			t.Errorf("%s: %d literal divider/normal-weight token(s), down from %d -- lower the entry in this file to lock the migration in", f, actual[f], w)
		}
	}
	for _, f := range sortedFileNames(handWrittenDividerFloors) {
		if _, still := actual[f]; !still {
			t.Errorf("handWrittenDividerFloors still names %s, which no longer has one -- remove the entry", f)
		}
	}
}

// handWrittenSurfaceFloors are the `.templ` sites that still write the surface recipe by hand -- a class set holding
// `rounded-lg`, `border` and the literal `border-slate-200` -- instead of reading `surfaceClasses(ctx)`. The
// eighth directive ratchet (2026-10-06; find the readers, migrate, then gate): 34 sites in 18 files moved, and
// until they did a Workspace's `theme.border.surface` recoloured only what `panelLayout`/`sectionLayout` drew.
//
// **What this does not count, stated so the next slice does not rediscover it:** the `rounded-md` +
// `border-slate-200` sites are not the surface role -- they were buttons, a read-only input and nested tiles, three
// jobs a widening to `RadiusSurface` would shift by 2px. The eleven tiles moved to `tileClasses(ctx)` and are counted by
// `TestHandWrittenTilesOnlyShrink` below; the rest wait on D8. Dashed empty states (`border-slate-300`) and red
// danger zones are other border colours.
var handWrittenSurfaceFloors = map[string]int{
	"approvalinbox.templ":  1, // border colour is conditional (red when overdue); a reader would have to take the state
	"calendar.templ":       1, // today's column swaps the border and fill; same shape
	"notifications.templ":  1, // an unread row swaps the border and fill; same shape
	"reviewdocument.templ": 1, // a <dialog> with no raised fill: the user agent paints its Canvas, and adding bg-white would be a change
}

var (
	surfaceClassAttr = regexp.MustCompile(`class="([^"]*)"`)
	surfaceLiteral   = regexp.MustCompile(`"([^"]*)"`)
)

func TestHandWrittenSurfacesOnlyShrink(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	comment := regexp.MustCompile(`//[^\n]*`)
	actual := map[string]int{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") || e.Name() == "layout.templ" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		src := joinClassExpressions(comment.ReplaceAllString(string(b), ""))
		for _, m := range surfaceClassAttr.FindAllStringSubmatch(src, -1) {
			set := map[string]bool{}
			for _, tok := range strings.Fields(m[1]) {
				set[tok] = true
			}
			if set["rounded-lg"] && set["border"] && set["border-slate-200"] {
				actual[e.Name()]++
			}
		}
	}
	for _, f := range sortedFileNames(actual) {
		switch w := handWrittenSurfaceFloors[f]; {
		case actual[f] > w:
			t.Errorf("%s: %d hand-written surface site(s), frozen at %d -- use `surfaceClasses(ctx)` so `theme.radius.surface`, `theme.border.surface` and `theme.background.raised` move it; a site whose border colour is conditional or whose fill is not the raised one goes in handWrittenSurfaceFloors with its reason", f, actual[f], w)
		case actual[f] < w:
			t.Errorf("%s: %d hand-written surface site(s), down from %d -- lower the entry in this file to lock the migration in", f, actual[f], w)
		}
	}
	for _, f := range sortedFileNames(handWrittenSurfaceFloors) {
		if _, still := actual[f]; !still {
			t.Errorf("handWrittenSurfaceFloors still names %s, which no longer has one -- remove the entry", f)
		}
	}
}

// TestNoHandWrittenRegionRule is a zero-floor gate (2026-10-06): no class set in `internal/rendering` may hold
// `border-t`, `border-b` or `border-y` beside a literal `border-slate-200`. A rule that bounds a region -- a card's
// header, a table's head, the page's top bar and bottom nav -- reads `borderClass(ctx, domain.BorderSurface)`, so
// `theme.border.surface` moves the frame's rules together with the box they sit in. Thirteen sites moved (twelve
// literals and `tableHeadCell`, whose seven callers now go through `tableHeadCellClasses`); none was a floor, because
// none swaps its colour by state. A row separator is a different job and reads `divider` (`tableCellClasses`).
//
// **No new role was added, and that is a decision with a way back.** A `region` role would default to the same
// slate-200 as `surface` and nothing declares a reason to separate them; if an author needs a faint box with firm
// rules (or the reverse), that is the case that earns the role. Blind spots, stated: a rule arriving from a Go
// function rather than a `class` attribute, and a variant-prefixed token (`sm:border-b`), which the census found none of.
func TestNoHandWrittenRegionRule(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	comment := regexp.MustCompile(`//[^\n]*`)
	seen := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") || e.Name() == "layout.templ" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		src := joinClassExpressions(comment.ReplaceAllString(string(b), ""))
		for _, m := range surfaceClassAttr.FindAllStringSubmatch(src, -1) {
			set := map[string]bool{}
			for _, tok := range strings.Fields(m[1]) {
				set[tok] = true
			}
			if set["border"] || set["border-slate-200"] {
				seen++
			}
			if (set["border-t"] || set["border-b"] || set["border-y"]) && set["border-slate-200"] {
				t.Errorf("%s: a hand-written region rule (%q) -- use `borderClass(ctx, domain.BorderSurface)` so `theme.border.surface` moves it", e.Name(), m[1])
			}
		}
	}
	if seen == 0 {
		t.Fatal("no class set holding a border was found anywhere -- the scan is looking at the wrong text")
	}
}

// TestNoHandWrittenDividedRow is a zero-floor gate (2026-10-07): no class set in `internal/rendering` may hold
// `last:border-b-0` beside `sm:flex-row`. That pair is a divided list's row -- stacked on a phone, cells side by side
// from `sm:`, a rule under each but the last -- and it reads `dividedRowClasses(ctx, gap)`, so `theme.border.divider`
// and `theme.gap` move every such list together. Three sites carried one literal (approvalinbox, workspacemembers
// twice); the `columns` pattern in `TestHandWrittenLayoutSitesOnlyShrink` could not hold them after the reader
// replaced the literal, so this keeps the shape from coming back unnoticed. Blind spots, stated: a row arriving from
// a Go function other than the reader, and one written without `last:border-b-0`.
func TestNoHandWrittenDividedRow(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	comment := regexp.MustCompile(`//[^\n]*`)
	seen := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") || e.Name() == "layout.templ" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		src := joinClassExpressions(comment.ReplaceAllString(string(b), ""))
		for _, m := range surfaceClassAttr.FindAllStringSubmatch(src, -1) {
			set := map[string]bool{}
			for _, tok := range strings.Fields(m[1]) {
				set[tok] = true
			}
			if set["flex"] {
				seen++
			}
			if set["last:border-b-0"] && set["sm:flex-row"] {
				t.Errorf("%s: a hand-written divided row (%q) -- use `dividedRowClasses(ctx, gap)` so the theme moves it", e.Name(), m[1])
			}
		}
	}
	if seen == 0 {
		t.Fatal("no class set holding flex was found anywhere -- the scan is looking at the wrong text")
	}
}

// handWrittenTileFloors are the `.templ` sites still writing `rounded-md border border-slate-200` by hand after the
// eleven tiles moved to `tileClasses(ctx)` (2026-10-07; read, migrate, then gate -- no primitive). **None of the ten is
// a tile**, which is the finding: the census that found 21 sites sorted them by reading into 11 tiles, 7 controls and
// 3 that are neither, and the reading is what the floors below record. The seven controls are outlines that wait on
// owner decision D8 (which border role a button's outline is): moving them would shift buttons. The other three are an
// image frame, an image edge and a segmented control's track, none of which is a box holding content.
//
// **Updated 2026-10-07 after D8 (a button's outline reads `border.surface`).** Seven sites left: the calendar's
// three controls, `reviewdocument`'s View PDF / Download, the disabled Enable button (all `tileClasses`) and the
// view switch's track (`tileClasses` + `background.raised`), byte-identical by default. **Three remain and none is
// button-shaped**: a read-only email box (a field on slate-50, a disabled look), a clipped image frame, and the
// edge of a rendered page image.
var handWrittenTileFloors = map[string]int{
	"account.templ":            1, // a read-only email box: a field on slate-50 as a disabled look, not a tile or a button
	"reviewdocument.templ":     1, // a clipped image frame on slate-50
	"signatureplacement.templ": 1, // the edge of a rendered page image, not a box holding content
}

// TestHandWrittenTilesOnlyShrink counts a class set holding `rounded-md`, `border` and the literal `border-slate-200`
// after `joinClassExpressions`, so a tile that reads `tileClasses(ctx)` leaves the count while a control does not.
// Blind spots, stated: a tile whose radius or border arrives from a Go function, and a `slate-200` edge on a
// `rounded-lg` box (that is `TestHandWrittenSurfacesOnlyShrink`'s population).
func TestHandWrittenTilesOnlyShrink(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	comment := regexp.MustCompile(`//[^\n]*`)
	actual := map[string]int{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") || e.Name() == "layout.templ" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		src := joinClassExpressions(comment.ReplaceAllString(string(b), ""))
		for _, m := range surfaceClassAttr.FindAllStringSubmatch(src, -1) {
			set := map[string]bool{}
			for _, tok := range strings.Fields(m[1]) {
				set[tok] = true
			}
			if set["rounded-md"] && set["border"] && set["border-slate-200"] {
				actual[e.Name()]++
			}
		}
	}
	for _, f := range sortedFileNames(actual) {
		switch w := handWrittenTileFloors[f]; {
		case actual[f] > w:
			t.Errorf("%s: %d hand-written small-box site(s), frozen at %d -- a tile (a box holding content) reads `tileClasses(ctx)` so `theme.radius.control` and `theme.border.surface` move it; a button-shaped outline reads `tileClasses(ctx)` too (D8, 2026-10-07); anything that is neither goes in handWrittenTileFloors with its reason", f, actual[f], w)
		case actual[f] < w:
			t.Errorf("%s: %d hand-written small-box site(s), down from %d -- lower the entry in this file to lock the migration in", f, actual[f], w)
		}
	}
	for _, f := range sortedFileNames(handWrittenTileFloors) {
		if _, still := actual[f]; !still {
			t.Errorf("handWrittenTileFloors still names %s, which no longer has one -- remove the entry", f)
		}
	}
}

// tagTint matches the translucent fill a tag chip is drawn with (`bg-blue-600/10`), which is the palette's
// signature: a status badge, a button and a surface use a solid -50/-100 tint or white, never a /10 one.
var tagTint = regexp.MustCompile(`^bg-[a-z]+-[0-9]+/10$`)

// TestNoHandWrittenTag is a zero-floor gate (2026-10-08): no class set in `internal/rendering` may hold
// `rounded-full` beside a `bg-<colour>-<step>/10` fill. That pair is a tag chip, and it reads
// `@tagChip(label, color)` -- the renderer of `domain.ComponentTag` -- so the closed palette stays the only
// place a tag's colour is decided. Five sites drew a tag, all through one function, so the migration moved no
// markup; the gate exists because the existing chip ratchet counts `-50`/`-100` tints and could not see this
// shape at all (measured: every `/10` fill in the tree sits in `tagClasses`, none in a class attribute).
// Blind spots, stated: a fill arriving from a Go function other than `tagClasses`, and a chip written without
// `rounded-full`.
func TestNoHandWrittenTag(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	comment := regexp.MustCompile(`//[^\n]*`)
	seen := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		src := joinClassExpressions(comment.ReplaceAllString(string(b), ""))
		for _, m := range surfaceClassAttr.FindAllStringSubmatch(src, -1) {
			rounded, tint := false, false
			for _, tok := range strings.Fields(m[1]) {
				rounded = rounded || tok == "rounded-full"
				tint = tint || tagTint.MatchString(tok)
			}
			if rounded {
				seen++
			}
			if rounded && tint {
				t.Errorf("%s: a hand-written tag (%q) -- use `@tagChip(label, color)` so the palette stays the one place a tag's colour is decided", e.Name(), m[1])
			}
		}
	}
	if seen == 0 {
		t.Fatal("no class set holding rounded-full was found anywhere -- the scan is looking at the wrong text")
	}
}

// handWrittenCreateForms counts the `hx-post` attributes that post to a Machine's generic create route
// (`/machines/<id>/records`, nothing after `records`) in a hand-written `.templ`, per file, **shrink-only** --
// the directive ratchet beside `TestEveryInstalledFormNamesNoRouteOrField`, installed after `Form` shipped
// (T1, 2026-10-09; build, migrate, then gate). A page now declares a create form with `component: Form` and
// `binding: {dataset, write: create}`; a new hand-written one should ask whether that can express it.
//
// **Every entry is a floor with the thing a bound Form does not carry yet**, none is a verdict:
//   - `machine.templ` x3: `cardComposer` (one text input, a hidden group value and a disclosure), `newCardPopover`
//     (every Field, including reference and group Fields whose options are another Machine's records -- T4) and
//     `createFormRow` (a table row of inputs, same reference options).
//   - `comments.templ`, `checklist.templ`: a child Machine's create, with the parent record's id as a hidden input.
//     That is a form *inside a record*, which `write: create` does not yet express (T3).
//   - `attachments.templ`: a `multipart/form-data` file upload submitted on `change`; a Form skips file Fields.
//
// Counting method: syntactic -- an `hx-post={ ... }` expression naming `/machines/` and ending the route at
// `/records"`, with `//` comments stripped. It cannot see a plain `<form method="POST" action=...>` (those post
// to bespoke routes such as `/workspace-members/...`, not the generic one), a route assembled in a Go helper, or
// a create driven from script; it states what is present and concludes nothing about absence.
var handWrittenCreateForms = map[string]int{
	"attachments.templ": 1, // multipart file upload on change; a Form skips file Fields
	"checklist.templ":   1, // child Machine create carrying the parent record id (T3)
	"comments.templ":    1, // child Machine create carrying the parent record id (T3)
	"machine.templ":     3, // card composer; every-Field popover and table row with reference options (T4)
}

var createPost = regexp.MustCompile(`hx-post=\{[^}\n]*/machines/[^}\n]*/records"[^}\n]*\}`)

func TestHandWrittenCreateFormsOnlyShrink(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	comment := regexp.MustCompile(`//[^\n]*`)
	actual := map[string]int{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if n := len(createPost.FindAllString(comment.ReplaceAllString(string(b), ""), -1)); n > 0 {
			actual[e.Name()] = n
		}
	}
	for _, f := range sortedFileNames(actual) {
		switch w := handWrittenCreateForms[f]; {
		case actual[f] > w:
			t.Errorf("%s: %d hand-written create form(s), frozen at %d -- declare it with `component: Form` and `binding: {dataset, write: create}` (007 §11.3). If it carries something a bound Form does not yet, say which in handWrittenCreateForms rather than widening it", f, actual[f], w)
		case actual[f] < w:
			t.Errorf("%s: %d hand-written create form(s), down from %d -- lower the entry in this file to lock the migration in", f, actual[f], w)
		}
	}
	for _, f := range sortedFileNames(handWrittenCreateForms) {
		if _, still := actual[f]; !still {
			t.Errorf("handWrittenCreateForms still lists %s, which no longer has one -- remove the entry", f)
		}
	}
}
