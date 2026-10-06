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
	"rolematrix.templ":         3,
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

// TestNoHandWrittenHeadingOrEyebrow holds three floors (the third, the faint overline, joined 2026-10-06) at **zero**: a heading element at `text-xl` is a screen's
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
