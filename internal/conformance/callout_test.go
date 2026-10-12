package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestNoHandWrittenCallout is the zero-floor gate beside `Callout`, the eleventh registered Component (2026-10-12;
// build, migrate, then gate). A message box is `@callout(text, tone)`: the tone decides the fill, the ink and the
// live-region role, and a Workspace's `theme:` moves all of them.
//
// What it counts is a **tone pair in a box**: one class set holding a corner (`rounded`, `rounded-md`,
// `rounded-lg`), a `-50` tint and the same hue's `-700`/`-800`/`-900` ink. A pill (`rounded-full`) is a badge or a
// chip, which other gates hold, so it is not counted. Class sets are read after `joinClassExpressions`, so a tint
// spliced into a `class={ ... }` expression is seen too, and so is a tone reader called inline
// (`toneReadersAsLiterals`). **Blind spots:** a pair split across a parent and a child
// (the parent tinted, the child inked, as Install Application's refusal well is), a pair returned by a Go function,
// and a box tinted with a hue outside the five tone hues.
//
// The floors are `calloutExempt`, a closed map with the count per file. Each is a box a two-input Callout must not
// absorb, with its reason. A count that drifts either way fails.
func TestNoHandWrittenCallout(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	comment := regexp.MustCompile(`//[^\n]*`)
	classAttr := regexp.MustCompile(`class="([^"]*)"`)
	tint := regexp.MustCompile(`^bg-(red|emerald|green|amber|blue)-50$`)
	ink := regexp.MustCompile(`^text-(red|emerald|green|amber|blue)-[789]00$`)
	scanned := 0
	seen := map[string]int{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		src := joinClassExpressions(toneReadersAsLiterals(comment.ReplaceAllString(string(b), "")))
		for _, m := range classAttr.FindAllStringSubmatch(src, -1) {
			scanned++
			if isToneBox(m[1], tint, ink) {
				seen[e.Name()]++
			}
		}
	}
	if scanned == 0 {
		t.Fatal("no class attribute was scanned -- the gate is reading the wrong text")
	}
	var files []string
	for f := range seen {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		if want := calloutExempt[f]; seen[f] != want {
			t.Errorf("%s: %d hand-written tinted message box(es), exempt %d -- a message reads @callout(text, tone), so its "+
				"tone, live-region role and Theme follow one contract; a box a Callout must not absorb goes in calloutExempt with its reason", f, seen[f], want)
		}
	}
	for f, want := range calloutExempt {
		if seen[f] == 0 && want > 0 {
			t.Errorf("calloutExempt[%q] = %d but the file has none -- a closed exemption must match exactly; drop it", f, want)
		}
	}
}

// toneReadersAsLiterals rewrites an inline reader of a message tone (info, good, bad, warn; a neutral or muted
// tint is a label, not a message) as the literal it answers by default, so a box that reads
// its tint and ink through `toneFillClass`/`toneInkClass`/`toneClass` is still seen as a hand-written tone box. Without
// it, moving a floor's colours to the Theme (rencana 0.2c) made both floors vanish from this gate by an edit to an
// attribute while neither became a Callout -- the blind spot `joinClassExpressions` exists for, one level down.
func toneReadersAsLiterals(src string) string {
	src = toneFillCall.ReplaceAllString(src, `"bg-red-50"`)
	src = toneInkCall.ReplaceAllString(src, `"text-red-700"`)
	return tonePairCall.ReplaceAllString(src, `"bg-red-50 text-red-700"`)
}

var (
	toneFillCall = regexp.MustCompile(`toneFillClass\(ctx, domain\.Tone(?:Info|Good|Bad|Warn)\)`)
	toneInkCall  = regexp.MustCompile(`toneInkClass\(ctx, domain\.Tone(?:Info|Good|Bad|Warn)\)`)
	tonePairCall = regexp.MustCompile(`toneClass\(ctx, domain\.Tone(?:Info|Good|Bad|Warn)\)`)
)

// isToneBox reports whether one class set is a cornered box with a tone hue's tint and the same hue's ink.
// Variant-prefixed tokens (`hover:`, `sm:`) are state and breakpoint, not the box.
func isToneBox(classes string, tint, ink *regexp.Regexp) bool {
	cornered := false
	tints, inks := map[string]bool{}, map[string]bool{}
	for _, tok := range strings.Fields(classes) {
		switch {
		case strings.Contains(tok, ":"):
		case tok == "rounded" || tok == "rounded-md" || tok == "rounded-lg":
			cornered = true
		case tint.MatchString(tok):
			tints[tint.FindStringSubmatch(tok)[1]] = true
		case ink.MatchString(tok):
			inks[ink.FindStringSubmatch(tok)[1]] = true
		}
	}
	if !cornered {
		return false
	}
	for hue := range tints {
		if inks[hue] {
			return true
		}
	}
	return false
}

// calloutExempt is the closed list of tinted boxes that are not a two-input Callout, with the count per file.
var calloutExempt = map[string]int{
	// Delete account's blockers: a title and a list of reasons (`role="alert"`), not one sentence. A Callout with a
	// title and items is the shape callouts grow elsewhere; it waits for a second box that needs it.
	"deleteaccount.templ": 1,
	// `truncationNotice`, drawn by `Collection`: bordered and caption-sized (11px) inside a list. Moving it to a
	// Callout would grow it to body size and drop the border, which is a design decision (menjaga-design-system R1),
	// not a migration.
	"machine.templ": 1,
}
