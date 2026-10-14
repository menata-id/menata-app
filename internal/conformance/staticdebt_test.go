package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
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
//   - `installapplication.templ`, `workspacesettings.templ`: a page's description at `leading-6`. Sharing a
//     line-height no text role names -- a **finding**, not a miss: if a role for it is ever declared, these
//     are the first consumers. (`appsettings.templ`'s two left 2026-10-11: they read `textClass`/`inkClass`
//     and keep `leading-6` as a literal, which is the same boundary drawn from the Theme side.)
//   - `rolematrix.templ` left 2026-10-11 the same way as `appsettings.templ`'s two: size and ink read the Theme,
//     while the spacing and width (`py-3`, `max-w-[760px]`) and board 06's inline link stay as they were --
//     Layout's (§12.2) and a leaf Static kind must not accept them (§12.3's "arbitrary properties").
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
	"approvalinbox.templ":      2,
	"inference.templ":          3,
	"installapplication.templ": 1,
	"newapplication.templ":     2, // + the thinking note (2026-10-13): `id` and `hidden`, a script toggles it
	"workspacemembers.templ":   1,
	"workspacesettings.templ":  1,
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
		// Ink readers read as their default literal (inkReadersAsLiterals, rencana 3.2d), and class expressions are
		// joined, so a paragraph whose ink moved to inkClass is still the hand-written paragraph it was.
		src := joinClassExpressions(inkReadersAsLiterals(mutedParaNote.ReplaceAllString(string(b), "")))
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
		src := joinClassExpressions(colourReadersAsLiterals(mutedParaNote.ReplaceAllString(string(b), "")))
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
	"chooseworkspace.templ":    1, // the archived-workspaces note: `px-0.5 leading-4` (seen since rencana 3.2d joined class expressions)
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
		// Read through joinClassExpressions since 2026-10-12: rencana 0.2b spliced a radius reader into one caption's
		// class, and a literal-only pattern lost the site by that edit alone (L13).
		src := joinClassExpressions(inkReadersAsLiterals(mutedParaNote.ReplaceAllString(string(b), "")))
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
		for _, m := range buttonLiteralOpen.FindAllStringSubmatch(joinClassExpressions(colourReadersAsLiterals(src)), -1) {
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

// handWrittenRawButtons is the third population of TestHandWrittenButtonsOnlyShrink (rencana 2.3a, 2026-10-13): every
// `<button>` in `internal/rendering/*.templ` that is neither a `buttonClasses(` caller nor a literal `h-9` button, which
// the two maps above already count. Before it, nothing counted these, so a new one could be added without anyone
// asking whether `@button` could draw it. **29 across 16 files**, each a floor with a reason read from the site.
//
// **The plan's "29 raw buttons" was this gate's blind spot, not a population of one meaning.** 2.3a was written as
// "Button variants `text`/`icon`/`menu`". Reading all 45 buttons outside the Button renderer showed the three
// variants do not hold:
//   - *icon* and *menu*: 14 of the 16 glyph, popover-trigger and menu-row sites carry what a Component must not
//     accept (007 §12.3): `popovertarget`, `hx-*`, a hyperscript `_=`, `aria-pressed`, `aria-controls`, `onclick`.
//     The other two are Sign out (a plain submit in a form) and Archive (a title and a description inside the button),
//     each alone in its shape, so neither is a system. What is missing is the `actions` capability the section names,
//     not a variant. (The first version of this comment said "all"; the 2.3a review found Sign out.)
//   - *text* is a **link look** (`text-xs`, the accent ink, its hover) written on 25 elements, 19 of them `<a>`; the
//     four `<button>`s are links that act instead of navigating. A Button variant would hold 4 of the 25 and split one
//     look across two places.
//   - what remains is per-site: a quiet destructive text action (4 sites, two resting inks and two sizes), a bordered
//     compact control (`px-3`, the D8 floor), pre-auth screens, two toggles, one draggable marker, one row with a
//     title and a description inside it.
//
// Counting method: syntactic and quote- and brace-aware (`buttonOpenTags`), `//` and HTML comments removed. It cannot
// see a button drawn by a Go function or a `<button>` assembled in a string, and a `<button` inside a Go raw string or a
// `<script>` string is counted as a tag (an over-count, none exists). **A reason can go stale at a constant count**: swap
// one raw button for another in the same file and the number holds while the sentence is false. That is inherent to
// counting; read the file's reason when you change one of its buttons.
var handWrittenRawButtons = map[string]struct {
	n   int
	why string
}{
	"account.templ":            {2, "'Sign out of other devices' is an h-11 bordered control at the 15px touch size, which no variant has; the disabled 'Enable' is a tile"},
	"appshell.templ":           {7, "five popover triggers and the shade (`popovertarget`: control behaviour, 007 §12.3 `actions`, unbuilt), and the Sign out menu row (a plain submit in a form)"},
	"approvalinbox.templ":      {1, "'Revise →': the link look (accent text, 25 sites on 19 `<a>`, 4 `<button>`, a `<summary>`, a `<span>`), in a form"},
	"attachments.templ":        {1, "an icon-only delete: `hx-delete`, `hx-target=\"closest li\"` and a glyph named by aria-label"},
	"authshell.templ":          {2, "pre-auth (no Theme reaches it): the password reveal toggle (aria-controls, aria-pressed, onclick) and the primary submit at h-11 sm:h-9"},
	"checklist.templ":          {1, "the completion circle: an `aria-pressed` toggle drawn as a glyph"},
	"chooseworkspace.templ":    {1, "a workspace card that submits: the whole tile is the button; pre-auth, no Workspace on ctx"},
	"detail.templ":             {1, "the Share row of the record menu: an `onclick` that copies the link and rewrites its own hint"},
	"documentsubmit.templ":     {4, "'+ Add approver' (`hx-get`, the link look) and the step's ▲ ▼ ✕ icon buttons, each a hyperscript `_=` that moves or removes its row"},
	"groups.templ":             {1, "'Delete this group': a quiet destructive text action, red at rest with an underline on hover"},
	"machine.templ":            {1, "the completion circle: an `aria-pressed` toggle drawn as a glyph"},
	"newapplication.templ":     {2, "two 'Discard' text actions, grey at rest and red on hover -- the same resting ink and hover as workspacemembers' Revoke (groups' is red at rest)"},
	"reviewdocument.templ":     {1, "'Clear': the link look with a `sig-modal-clear` hook class a script finds"},
	"signatureplacement.templ": {2, "the draggable signature marker (`sig-marker`, its own shape) and 'Re-centre': the link look"},
	"workspacemembers.templ":   {1, "'Revoke': a quiet destructive text action at text-sm with `p-0`, grey at rest and red on hover"},
	"workspacesettings.templ":  {1, "'Archive workspace': a danger row with a title and a description inside the button (a layout in a button)"},
}

// buttonOpenTags returns the text of every `<button ...>` opening tag in a templ source, in order. A `>` inside a
// quoted attribute value (`onclick="... => ..."`) or inside a `{ ... }` templ expression (which may hold its own strings
// and nested braces, as `if name != "" { name={ name } }` does) does not close the tag; a regexp stopping at the first
// `>` would, and the 3.5-era `fieldTagOpen` pattern has the same limit.
func buttonOpenTags(src string) []string {
	var out []string
	for i := 0; i < len(src); {
		loc := buttonTagStart.FindStringIndex(src[i:])
		if loc == nil {
			break
		}
		start := i + loc[0]
		end := scanTagEnd(src, i+loc[1]-1) // loc[1]-1 is the byte after `<button`, which the pattern consumed
		out = append(out, src[start:end])
		i = end
	}
	return out
}

// buttonTagStart matches `<button` as a whole tag name in any case (HTML names are case-insensitive, and the 2.3a
// review passed a `<BUTTON>` through the first version): the next byte is whitespace or `>`, so `<buttons` and
// `<button-group` are not it. The match consumes that byte.
var buttonTagStart = regexp.MustCompile(`(?i)<button[\s>]`)

// scanTagEnd returns the index just past the `>` that closes the tag whose attributes begin at k.
func scanTagEnd(src string, k int) int {
	depth := 0
	var quote byte
	for ; k < len(src); k++ {
		c := src[k]
		switch {
		case quote != 0:
			if c == '\\' && quote != '`' {
				k++ // an escaped character inside a string
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '`' || c == '\'':
			// A single quote delimits an attribute value at tag level as well (`title='a>b'`); inside a templ
			// expression it is a rune literal. The 2.3a review passed `<button title='it>s'>` through the first version.
			quote = c
		case c == '{':
			depth++
		case c == '}':
			if depth > 0 {
				depth--
			}
		case c == '>' && depth == 0:
			return k + 1
		}
	}
	return len(src)
}

// TestButtonOpenTags pins the scanner on the shapes a regexp stopping at the first `>` gets wrong.
func TestButtonOpenTags(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      []string
	}{
		{"plain", `<button type="submit">Go</button>`, []string{`<button type="submit">`}},
		{"arrow in an attribute", `<button type="button" onclick="a.then(() => { b() })">x</button>`, []string{`<button type="button" onclick="a.then(() => { b() })">`}},
		{"nested braces and a comparison", `<button type="submit" if name != "" { name={ name } } class={ f(ctx, a > b) }>x</button>`, []string{`<button type="submit" if name != "" { name={ name } } class={ f(ctx, a > b) }>`}},
		{"a brace inside a string inside braces", `<button class={ "a}", g(ctx) } type="button">x</button>`, []string{`<button class={ "a}", g(ctx) } type="button">`}},
		{"another tag name", `<buttons><button-group></button-group><button type="button">x</button>`, []string{`<button type="button">`}},
		{"two", `<button type="a">1</button><button type="b">2</button>`, []string{`<button type="a">`, `<button type="b">`}},
		{"newline before the end", "<button\n\ttype=\"button\"\n>x</button>", []string{"<button\n\ttype=\"button\"\n>"}},
		{"upper case", `<BUTTON type="button">x</BUTTON>`, []string{`<BUTTON type="button">`}},
		{"mixed case", `<Button type="button">x</Button>`, []string{`<Button type="button">`}},
		{"a single-quoted value holding >", `<button title='it>s' type="button">x</button>`, []string{`<button title='it>s' type="button">`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := buttonOpenTags(tc.src)
			if strings.Join(got, "\x00") != strings.Join(tc.want, "\x00") {
				t.Errorf("buttonOpenTags(%q) = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// isLiteralH9Button reports whether a button tag is one of handWrittenButtonLiterals' population: a literal `h-9` button
// with a white or slate-900 fill, read the way TestHandWrittenButtonsOnlyShrink reads it.
func isLiteralH9Button(tag string) bool {
	for _, m := range buttonLiteralOpen.FindAllStringSubmatch(joinClassExpressions(colourReadersAsLiterals(tag)), -1) {
		has := func(tok string) bool {
			return regexp.MustCompile(`(?:^|\s)` + regexp.QuoteMeta(tok) + `(?:\s|$)`).MatchString(m[1])
		}
		if has("h-9") && (has("bg-white") || has("bg-slate-900")) {
			return true
		}
	}
	return false
}

// renderingTemplSources returns every hand-written `.templ` source under internal/rendering with comments removed.
func renderingTemplSources(t *testing.T) map[string]string {
	t.Helper()
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	out := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		out[e.Name()] = stripTemplComments(string(b))
	}
	return out
}

// TestHandWrittenRawButtonsOnlyShrink is the third population of the button ratchet: a `<button>` that neither calls
// `buttonClasses(` nor is a literal h-9 button. Shrink-only both ways, a reason per file. A new one fails with the
// question the Component exists to answer: can `@button(label, variant, name, value)` draw it? It cannot when the
// button carries behaviour (`popovertarget`, `hx-*`, a script hook, `aria-pressed`) -- 007 §12.3 -- and then it goes in
// handWrittenRawButtons with that reason.
func TestHandWrittenRawButtonsOnlyShrink(t *testing.T) {
	actual, seen := map[string]int{}, 0
	for name, src := range renderingTemplSources(t) {
		for _, tag := range buttonOpenTags(src) {
			seen++
			if strings.Contains(tag, "buttonClasses(") || isLiteralH9Button(tag) {
				continue // counted by TestHandWrittenButtonsOnlyShrink
			}
			actual[name]++
		}
	}
	if seen < 40 {
		t.Fatalf("only %d <button> tags were found in internal/rendering (45 outside the Button renderer, 49 with it, on 2026-10-13) -- the scan is measuring far less than it appears to", seen)
	}
	for _, f := range sortedFileNames(actual) {
		switch w := handWrittenRawButtons[f].n; {
		case actual[f] > w:
			t.Errorf("%s: %d raw <button>(s), frozen at %d -- can `@button(label, variant, name, value)` draw it? If it carries behaviour the Component must not accept (popovertarget, hx-*, a script hook, aria-pressed, 007 §12.3), say which in handWrittenRawButtons rather than widening the contract", f, actual[f], w)
		case actual[f] < w:
			t.Errorf("%s: %d raw <button>(s), down from %d -- lower the entry in this file to lock the migration in", f, actual[f], w)
		}
	}
	for _, f := range sortedFileNames(handWrittenRawButtons) {
		if _, still := actual[f]; !still {
			t.Errorf("handWrittenRawButtons still names %s, which no longer has one -- remove the entry", f)
		}
		if strings.TrimSpace(handWrittenRawButtons[f].why) == "" {
			t.Errorf("handWrittenRawButtons names %s with no reason", f)
		}
	}
}

// tagAttrNames returns the attribute names of an opening tag, lower-cased: the words in name position, never the text
// of a value. A value is skipped whole -- a quoted string, a `{ ... }` templ expression (balanced, strings inside it
// respected) or a bare word -- and a templ conditional (`if cond { name={ n } }`, `else { ... }`) is read through, so
// an attribute set only in one branch still counts. The first version of the `type` gate matched `\stype=` anywhere in the
// tag, and the 2.3a review passed `<button title="set type=x">` and a `type=` inside an `onclick` string.
func tagAttrNames(tag string) []string {
	i := strings.IndexAny(tag, " \t\r\n>") // past the tag name
	if i < 0 {
		return nil
	}
	names, _ := parseAttrs(tag, i, '>')
	return names
}

// parseAttrs reads attributes from s[i:] up to the unquoted byte closer, returning the names and the index after it.
func parseAttrs(s string, i int, closer byte) ([]string, int) {
	var names []string
	skipValue := func() { // s[i] is the first byte of a value
		switch c := s[i]; {
		case c == '"' || c == '\'' || c == '`':
			for i++; i < len(s) && s[i] != c; i++ {
				if s[i] == '\\' && c != '`' {
					i++
				}
			}
			i++
		case c == '{':
			i = skipBraces(s, i)
		default: // a bare word
			for i < len(s) && !strings.ContainsRune(" \t\r\n>", rune(s[i])) {
				i++
			}
		}
	}
	for i < len(s) {
		for i < len(s) && strings.ContainsRune(" \t\r\n/", rune(s[i])) {
			i++
		}
		if i >= len(s) {
			break
		}
		if s[i] == closer {
			return names, i + 1
		}
		if s[i] == '{' { // a spread: `{ attrs... }`
			i = skipBraces(s, i)
			continue
		}
		start := i
		for i < len(s) && !strings.ContainsRune(" \t\r\n=>{}/", rune(s[i])) {
			i++
		}
		name := strings.ToLower(s[start:i])
		if name == "if" || name == "else" {
			// A conditional: skip the condition up to the `{` that opens the block, then read the block's attributes.
			for i < len(s) && s[i] != '{' && s[i] != '>' {
				if s[i] == '"' || s[i] == '`' {
					skipQuoted := s[i]
					for i++; i < len(s) && s[i] != skipQuoted; i++ {
					}
				}
				i++
			}
			if i < len(s) && s[i] == '{' {
				var inner []string
				inner, i = parseAttrs(s, i+1, '}')
				names = append(names, inner...)
			}
			continue
		}
		if name != "" {
			names = append(names, name)
		}
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i < len(s) && s[i] == '=' {
			i++
			for i < len(s) && strings.ContainsRune(" \t\r\n", rune(s[i])) {
				i++
			}
			if i < len(s) {
				skipValue()
			}
		}
		if name == "" && i < len(s) && s[i] != closer && !strings.ContainsRune(" \t\r\n=", rune(s[i])) {
			i++ // never stall on a byte that is neither a name nor a value
		}
	}
	return names, i
}

// skipBraces returns the index after the `}` matching the `{` at s[i], reading strings inside the braces whole.
func skipBraces(s string, i int) int {
	depth := 0
	for ; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"' || c == '`' || c == '\'':
			for i++; i < len(s) && s[i] != c; i++ {
				if s[i] == '\\' && c != '`' {
					i++
				}
			}
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(s)
}

// TestTagAttrNames pins the attribute reader on what a substring match gets wrong.
func TestTagAttrNames(t *testing.T) {
	for _, tc := range []struct {
		name, tag string
		want      string // space-joined
	}{
		{"plain", `<button type="submit" class="a">`, "type class"},
		{"type only inside a title", `<button title="set type=x" class="a">`, "title class"},
		{"type only inside onclick", `<button class="a" onclick="this.type=1; f({type:2})">`, "class onclick"},
		{"type only inside an hx-vals expression", `<button class="a" hx-vals={ "{\"type\": 1}" }>`, "class hx-vals"},
		{"bare attribute", `<button disabled type="button" class={ f(ctx) }>`, "disabled type class"},
		{"newline separated", "<button\n\ttype=\"button\"\n\tclass={ a }\n>", "type class"},
		{"conditional attributes", `<button type="submit" if name != "" { name={ name } value={ value } } class={ b }>`, "type name value class"},
		{"conditional type in both branches", `<button if x { type="button" } else { type="submit" } class="c">`, "type type class"},
		{"hyperscript attribute", `<button type="button" _="on click remove closest <.row/>">`, "type _"},
		{"data-type is not type", `<button data-type="x" class="a">`, "data-type class"},
		{"upper-case names", `<button TYPE="button" CLASS="a">`, "type class"},
		{"apostrophe in a value", `<button title="it's" type="button">`, "title type"},
		{"single quoted value", `<button title='a>b' type="button">`, "title type"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := strings.Join(tagAttrNames(tc.tag), " "); got != tc.want {
				t.Errorf("tagAttrNames(%q) = %q, want %q", tc.tag, got, tc.want)
			}
		})
	}
}

// TestEveryButtonNamesItsType is a zero-floor gate: every `<button>` in internal/rendering has a `type` attribute. A
// button with no type is a submit button, so inside a `<form>` it submits the form whatever its htmx attributes say.
// Reading the 45 buttons for 2.3a found five without one (the record Edit/Delete buttons in detail.templ and
// machine.templ, and account's disabled Enable); none was inside a form, so nothing was wrong yet, and nothing would
// have said so if one had been moved into one. It reads attribute *names* (`tagAttrNames`), not the tag's text.
func TestEveryButtonNamesItsType(t *testing.T) {
	seen := 0
	sources := renderingTemplSources(t)
	for _, name := range sortedFileNames(sources) {
		for _, tag := range buttonOpenTags(sources[name]) {
			seen++
			if !slices.Contains(tagAttrNames(tag), "type") {
				t.Errorf("%s: a <button> with no type= is a submit button, so inside a <form> it submits the form -- add type=\"button\" (or \"submit\" if it should): %.90q", name, strings.Join(strings.Fields(tag), " "))
			}
		}
	}
	if seen < 40 {
		t.Fatalf("only %d <button> tags were found -- the scan is measuring far less than it appears to", seen)
	}
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
var handWrittenFieldFloors = map[string]int{}

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
		src := joinClassExpressions(inkReadersAsLiterals(mutedParaNote.ReplaceAllString(string(b), "")))
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
var handWrittenDividerFloors = map[string]int{}

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
	"approvalinbox.templ": 1, // border colour is conditional (red when overdue); a reader would have to take the state
	"notifications.templ": 1, // an unread row swaps the border and fill; same shape
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
		src := joinClassExpressions(borderReadersAsLiterals(comment.ReplaceAllString(string(b), "")))
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
		src := joinClassExpressions(borderReadersAsLiterals(comment.ReplaceAllString(string(b), "")))
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

// TestNoHandWrittenTable is a zero-floor gate, installed after `dataTable` replaced the six hand-written
// `<table>` elements (K12 stage 1, 2026-10-10; build, migrate, then gate): the only `<table>` in
// `internal/rendering` is the one in `table.templ`. A table drawn by hand loses what the renderer forces -- an
// accessible name, `scope="col"` headers, the Theme-read cell rules -- and none of the six replaced had a name.
// Comments are stripped first, so prose naming the tag does not count; the sanity check requires exactly one
// real site so the scan cannot pass by reading the wrong text.
func TestNoHandWrittenTable(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	comment := regexp.MustCompile(`//[^\n]*`)
	tag := regexp.MustCompile(`<table[\s>]`)
	owner := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		n := len(tag.FindAllString(comment.ReplaceAllString(string(b), ""), -1))
		if e.Name() == "table.templ" {
			owner = n
			continue
		}
		if n > 0 {
			t.Errorf("%s: %d hand-written <table> -- use `@dataTable(label, columns)` so the table has an accessible name, scoped headers and the Theme's cell rules", e.Name(), n)
		}
	}
	if owner != 1 {
		t.Fatalf("table.templ holds %d <table> elements, want exactly 1 -- the scan is looking at the wrong text or dataTable moved", owner)
	}
}

// TestNoHandWrittenMarkGood is a zero-floor gate (2026-10-11): the Design System artifact's `mark-good` fill and
// edge (emerald-600) are written only by `markGoodFill`/`markGoodBorder`/`markGoodHoverBorder` in `layout.templ`. A
// "finished" mark elsewhere reads them. Three sites carried the literals and moved byte-identically (checklist, the
// card checklist, Progress); render output was compared at HEAD and after, 13,892 bytes each.
//
// It matches the token anywhere in a `.templ` outside `layout.templ`, `//` comments stripped, so a Go string built
// for `templ.KV` is seen as well as a class attribute.
//
// It also refuses `bg-`/`border-emerald-500`, the value the artifact rejected for a finished mark. Measured, that
// is 2.46:1 on white, below WCAG 1.4.11's 3:1, where emerald-600 is 3.67:1. Five approval-screen marks used it
// until the owner moved them on 2026-10-12 (`approvalinbox`'s progress bar, and `reviewdocument`'s done step and
// three "signed" markers). One use stays, in `markGoodExempt`: the board's drag-and-drop slot, a JavaScript string
// for a transient drop target, which is not a finished mark.
func TestNoHandWrittenMarkGood(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	comment := regexp.MustCompile(`//[^\n]*`)
	mark := regexp.MustCompile(`(?:^|[\s"])((?:hover:)?(?:bg|border)-emerald-600)\b`)
	rejected := regexp.MustCompile(`(?:^|[\s"])((?:hover:)?(?:bg|border)-emerald-500)\b`)
	readers := 0
	exemptSeen := map[string]int{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		found := mark.FindAllStringSubmatch(comment.ReplaceAllString(string(b), ""), -1)
		if e.Name() == "layout.templ" {
			readers += len(found)
			continue
		}
		if want := markGoodScriptUses[e.Name()]; len(found) != want {
			for _, m := range found {
				t.Errorf("%s: hand-written %q -- a finished mark reads markGoodFill/markGoodBorder/markGoodHoverBorder "+
					"(the Design System artifact's `mark-good`), so the colour stays decided in one place (markGoodScriptUses allows %d here)", e.Name(), m[1], want)
			}
		}
		n := len(rejected.FindAllStringSubmatch(comment.ReplaceAllString(string(b), ""), -1))
		if want := markGoodExempt[e.Name()]; n != want {
			if want > 0 {
				exemptSeen[e.Name()] = n
				continue
			}
			t.Errorf("%s: %d use(s) of bg-/border-emerald-500 -- the Design System rejected it for a finished mark "+
				"(2.46:1 on white); use markGoodFill/markGoodBorder, or, if this is not a finished mark, say why in markGoodExempt", e.Name(), n)
		} else if want > 0 {
			exemptSeen[e.Name()] = n
		}
	}
	if readers != 3 {
		t.Fatalf("expected the three mark-good readers in layout.templ, found %d -- the scan is looking at the wrong text", readers)
	}
	for file, want := range markGoodExempt {
		if got := exemptSeen[file]; got != want {
			t.Errorf("markGoodExempt[%q] = %d but the file has %d -- a closed exemption must match exactly; drop or lower it", file, want, got)
		}
	}
}

// markGoodExempt is the closed list of emerald-500 uses that are not a finished mark, with the count per file. Empty
// since owner decision CP6 (2026-10-13) moved the board's drag-and-drop slot to mark-good; it stays declared.
var markGoodExempt = map[string]int{}

// markGoodScriptUses is the closed list of mark-good literals a reader cannot reach: a class written by JavaScript.
// The board's drag-and-drop draws its landing slot (`slot.className = "h-1 rounded-full bg-emerald-600"`) and the
// target lane's ring in a script, where no Go reader runs (CP6, 2026-10-13: emerald-500 folded to mark-good).
var markGoodScriptUses = map[string]int{"machine.templ": 1}
