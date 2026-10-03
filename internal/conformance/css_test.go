package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoClassLivesOnlyInAComment holds a failure mode this repository caused for itself, which is the only
// reason it is worth a gate: **Tailwind scans the `.templ` files as text, so a comment naming a class emits
// that class.**
//
// `make css` runs the standalone CLI over the sources named by `static/css/input.css`'s own `@source`
// directives. Nothing in that pipeline knows what a Go comment is. So when the first pass of the Experience
// Plane work shipped a `gridLayout` with no callers, and the fix-up comment recorded the mistake by naming
// the three-column desktop class it had leaked, the class stayed in app.css -- sourced by nothing but the
// sentence saying it should not be there. 63 bytes, invisible, and self-perpetuating: every reader of the
// comment was being told the bug was fixed by the text that was causing it.
//
// The check is narrow on purpose. A comment that mentions a class some screen *does* use costs nothing, and
// three of them exist legitimately (`controls.templ`'s `items-center`, `appshell.templ`'s `gap-1`,
// `layout.templ`'s `flex-col` -- each explaining the shape its own code renders). What fails is a token
// that is **both** an emitted class in app.css **and** absent from every live `class` attribute.
//
// Measured 2026-10-03 over all 38 `.templ` files: 4 comment-borne class tokens, of which **2** were dead.
// The second was found by this gate on its first run and is the better illustration, because no one was
// citing a class at all: `activity.templ` described the Dashboard's feed as "a flat top-10 list", and
// Tailwind emitted `.top-10`. Intent is invisible to the scanner, so the cost is the same. Describe the
// class rather than naming it ("a three-column desktop class"), or cite one the code beside it uses.
//
// **Its blind spot, and the gate that covers it**: this reads the *committed* app.css, so a leak introduced
// without re-running `make css` is invisible here. `make check-generated` is the other half -- it
// regenerates and `git diff --exit-code`s app.css, so a stale bundle fails there and this fires on the
// fresh one. Neither alone is sufficient; say so rather than reading a green run as full coverage.
func TestNoClassLivesOnlyInAComment(t *testing.T) {
	root := repoRoot()
	css, err := os.ReadFile(filepath.Join(root, "static", "css", "app.css"))
	if err != nil {
		t.Fatalf("read app.css: %v", err)
	}
	// Tailwind escapes `:`, `.`, `[`, `/` and friends in selectors; unescaping once lets a token be looked
	// for in its authored spelling rather than per-character.
	emitted := regexp.MustCompile(`\\(.)`).ReplaceAllString(string(css), "$1")

	dir := filepath.Join(root, "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}

	comment := regexp.MustCompile(`(?m)^[\t ]*//.*$`)
	// Unambiguously a utility: a variant chain plus a name ending in a number or an arbitrary value. A bare
	// word is excluded deliberately -- `static`, `filter`, `capitalize` and `transform` are all real Tailwind
	// classes *and* ordinary English, and a gate that flagged them would report prose as a leak.
	token := regexp.MustCompile(`\b(?:(?:sm|md|lg|xl|hover|focus|group-hover|peer-checked):)*[a-z][a-z-]*-(?:\d+(?:\.\d+)?|\[[^\]\s]+\])\b`)

	var liveText strings.Builder
	commentTokens := map[string][]string{} // token -> files whose comments name it
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		src := string(b)
		for _, line := range comment.FindAllString(src, -1) {
			for _, tok := range token.FindAllString(line, -1) {
				commentTokens[tok] = append(commentTokens[tok], e.Name())
			}
		}
		liveText.WriteString(comment.ReplaceAllString(src, ""))
	}
	// input.css may name a class directly, and the generated Go is not scanned, so neither is read here.
	if b, err := os.ReadFile(filepath.Join(root, "static", "css", "input.css")); err == nil {
		liveText.Write(b)
	}
	live := liveText.String()

	if len(commentTokens) == 0 {
		t.Fatal("no class-shaped token found in any .templ comment -- the pattern has stopped matching, so this gate is measuring nothing (there were 4 on 2026-10-03)")
	}
	for tok, files := range commentTokens {
		if isLiveClass(live, tok) {
			continue // some screen uses it; the comment costs nothing
		}
		if !isEmittedClass(emitted, tok) {
			continue // not a class Tailwind recognised; prose that merely looks like one
		}
		t.Errorf("%q is in static/css/app.css but appears in no live class attribute -- its only source is a comment in %s. Tailwind scans .templ as text: describe the class in prose instead of naming it, then re-run `make css`", tok, strings.Join(files, ", "))
	}
}

// isLiveClass asks whether a token is used as a class in its own right, **not merely as the tail of a
// longer one**.
//
// `strings.Contains` is wrong here and this gate shipped with it for one commit. `grid-cols-4` is a
// substring of `sm:grid-cols-4`, so a comment naming the unprefixed class looked live while only the
// breakpoint variant was -- and the unprefixed one went into the bundle. The leak was caught by diffing
// app.css across a worktree baseline, not by this gate, which is the honest order of events: the render-diff
// found what the gate was built to find.
//
// So a match must not be preceded by `:` (a variant prefix), `-` or a word character, nor followed by one
// that would extend the utility's own value.
func isLiveClass(live, tok string) bool {
	re := regexp.MustCompile(`(?:^|[^\w:.-])` + regexp.QuoteMeta(tok) + `(?:$|[^\w.-])`)
	return re.MatchString(live)
}

// isEmittedClass asks whether Tailwind emitted the token as a **class selector**, not merely as some other
// substring of the stylesheet.
//
// The looser version reported `slate-500` on its first run, which is not a class at all -- app.css carries
// `--color-slate-500` as a theme variable, and the token extractor had matched the tail of `text-slate-500`
// in a comment. Requiring a leading `.` separates the two: a variable is preceded by `-`, a selector by a
// dot. It also keeps a variant from vouching for its own base -- `.sm\:top-17` unescapes to `.sm:top-17`,
// which does not contain `.top-17`.
func isEmittedClass(css, tok string) bool {
	re := regexp.MustCompile(`\.` + regexp.QuoteMeta(tok) + `(?:[,{:>~+\[\s]|$)`)
	return re.MatchString(css)
}
