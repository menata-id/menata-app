package conformance

import (
	"strings"
	"testing"
)

// TestNoHandWrittenCompletionCircle is a zero-floor gate beside `Toggle`, the registered Component for a record's completion circle
// (2026-10-19; build, migrate, then gate). A ringed 20px circle that says whether a record is finished is `completionCircle` in
// controls.templ and nothing else: the Toggle a `page:` declares, a board card's hover circle and a checklist row each reach it, so
// the ring, the finished fill, the check and the accessible name ("Mark “x” complete") cannot drift apart. Before it there were two
// hand-written copies of one class expression (a board card's and a checklist's), and a Toggle would have been a third.
//
// It reads every `<button>` opening tag in internal/rendering (comments stripped, the scanner `buttonOpenTags` reads a `>` inside a
// quoted attribute or a templ expression as part of the tag) and fails on one that is `rounded-full`, `border-2` and a small size
// outside controls.templ -- a ringed circle of any size from `size-4` to `size-7`, whether or not it says `aria-pressed` (a copy that
// dropped the attribute, or grew a size, is the same circle). **What it cannot see, stated:** a circle drawn from a `<span>` or an
// `<a>`, and one whose size arrives from a Go function; the floor below counts the one it does see, so it cannot pass by measuring
// nothing.
func TestNoHandWrittenCompletionCircle(t *testing.T) {
	seen := 0
	for _, name := range sortedFileNames(renderingTemplSources(t)) {
		for _, tag := range buttonOpenTags(renderingTemplSources(t)[name]) {
			flat := strings.Join(strings.Fields(tag), " ")
			if !isCircleSized(flat) || !hasToken(flat, "rounded-full") || !hasToken(flat, "border-2") {
				continue
			}
			seen++
			if name != "controls.templ" {
				t.Errorf("%s: a hand-written completion circle (%.60q...) -- draw it with @completionCircle(label, done) or @toggle(...) so every circle looks and sounds the same", name, flat)
			}
		}
	}
	if seen != 1 {
		t.Errorf("found %d completion circles in internal/rendering, want exactly one (controls.templ's completionCircle) -- the scan is measuring something other than it appears to", seen)
	}
}

// hasToken reports whether tok appears as a whole word inside a class string of tag (not as a prefix of another utility).
func hasToken(tag, tok string) bool {
	for _, sep := range []string{` `, `"`, `{`, `,`} {
		for _, end := range []string{` `, `"`, `}`, `,`} {
			if strings.Contains(tag, sep+tok+end) {
				return true
			}
		}
	}
	return false
}

// isCircleSized reports whether tag carries a `size-4` .. `size-7` utility: the sizes a completion circle has been or might be drawn in.
func isCircleSized(tag string) bool {
	for _, n := range []string{"size-4", "size-5", "size-6", "size-7"} {
		if hasToken(tag, n) {
			return true
		}
	}
	return false
}
