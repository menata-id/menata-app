package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// handWrittenChips counts the chips still drawn by hand instead of through `statusBadge`, per file,
// **shrink-only** -- the same terms as handWrittenLayoutSites, and for the same reason: a migration is
// locked in rather than left as room to regress, and a new hand-written chip fails with the Component
// it should have used.
//
// Built after two slices moved 25 chips onto `statusBadge` (2026-10-05); the build-migrate-then-gate order
// this repo uses. **Four floors remain, and none is debt** -- each carries something `statusBadge` must
// not accept, which is the boundary working:
//
//   - `approvalinbox.templ`: the pending card's SLA chip. Uppercase, tracking and semibold on the same
//     element: typography is the Component's, not an input (007 §12.3 "arbitrary properties"). Its tone is
//     also two-valued by a local `overdue`, where the Component takes the resolved tone.
//   - `appsettings.templ`: "Not built yet", the same uppercase/tracking shape on a muted ground.
//   - `machine.templ`: the **selected tab**. Chip-shaped, but not a status -- it is §12.2's `tabs`, which
//     design-system-decisions D0b records as unbuilt. It will leave this list when that primitive does.
//   - `reviewdocument.templ`: a `<code>` reference. Monospace content in a different element; choosing the
//     element is the capability `statusBadge` lacks (the same finding as `row`'s `<form>`/`<span>` floor).
//
// **Counting method, stated beside the number.** Syntactic and *narrow on purpose*: an opening `<span>`,
// `<code>` or `<li>` (a chip is not interactive, so buttons and links are out) whose attributes -- including
// any `templ.KV` conditional class -- hold `rounded`, a `-50`/`-100` tint background, `px-` and a `text-3xs|2xs|xs`
// size. The first draft allowed any element and matched ~13 files: alert paragraphs, buttons, menu items and
// wells. **What it cannot see** is a chip written with `div`/`a`, or one using `pl-`/`pr-` instead of `px-`
// (`approvalinbox`'s `approverChip` is an avatar-plus-name pill and is *not* a status chip, so that is a
// judgement rather than a miss). Concluding a chip is absent still needs reading the screen.
var handWrittenChips = map[string]int{
	"approvalinbox.templ":  1,
	"appsettings.templ":    1,
	"machine.templ":        1,
	"reviewdocument.templ": 1,
}

var (
	chipTag    = regexp.MustCompile(`(?s)<(?:span|code|li)\b((?:[^>{]|\{[^}]*\})*)>`)
	chipRound  = regexp.MustCompile(`\brounded(?:-full)?\b`)
	chipTint   = regexp.MustCompile(`(?:^|[\s"])bg-(?:slate-50|slate-100|blue-50|emerald-50|red-50|amber-50)\b`)
	chipPadX   = regexp.MustCompile(`(?:^|[\s"])px-`)
	chipSize   = regexp.MustCompile(`\btext-(?:3xs|2xs|xs)\b`)
	chipNoLine = regexp.MustCompile(`(?m)^\s*//.*$`)
)

func TestHandWrittenChipsOnlyShrink(t *testing.T) {
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
		// A comment naming the shape is not a chip.
		src := chipNoLine.ReplaceAllString(string(b), "")
		for _, m := range chipTag.FindAllStringSubmatch(src, -1) {
			a := m[1]
			if chipRound.MatchString(a) && chipTint.MatchString(a) && chipPadX.MatchString(a) && chipSize.MatchString(a) {
				actual[e.Name()]++
			}
		}
	}

	for _, f := range sortedFileNames(actual) {
		switch w := handWrittenChips[f]; {
		case actual[f] > w:
			t.Errorf("%s: %d hand-written chip(s), frozen at %d -- use `statusBadge(label, domain.Tone*, domain.Badge*)` (007 §12.3; registry.Components). If this one carries something the Component must not accept (typography, another element, a control), say which in handWrittenChips rather than widening it", f, actual[f], w)
		case actual[f] < w:
			t.Errorf("%s: %d hand-written chip(s), down from %d -- lower the entry in this file to lock the migration in", f, actual[f], w)
		}
	}
	for _, f := range sortedFileNames(handWrittenChips) {
		if _, still := actual[f]; !still {
			t.Errorf("handWrittenChips still lists %s, which no longer has one -- remove the entry", f)
		}
	}
	if len(actual) == 0 {
		t.Fatal("no hand-written chip found anywhere -- either every chip uses statusBadge (say so in capabilities.md and keep this gate as an ordinary one) or the pattern has stopped matching")
	}
}
