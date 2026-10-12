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
		raw := stripTemplComments(string(b))
		src := joinClassExpressions(toneReadersAsLiterals(raw))
		if e.Name() != "callout.templ" && strings.Contains(raw, "calloutClasses(") {
			t.Errorf("%s: calls calloutClasses outside the Callout renderer -- borrowing the box without @callout drops the live-region role the contract decides", e.Name())
		}
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
//
// A tone passed as a variable (`toneClass(ctx, t)`) counts as a message tone too: a hand-written box whose tone is
// chosen at run time is still a hand-written callout (found by the 2026-10-12 independent review).
func toneReadersAsLiterals(src string) string {
	return toneReaderCall.ReplaceAllStringFunc(src, func(m string) string {
		sub := toneReaderCall.FindStringSubmatch(m)
		if sub[2] == "domain.ToneNeutral" || sub[2] == "domain.ToneMuted" {
			return m
		}
		return map[string]string{"toneFillClass": `"bg-red-50"`, "toneInkClass": `"text-red-700"`, "toneClass": `"bg-red-50 text-red-700"`}[sub[1]]
	})
}

var toneReaderCall = regexp.MustCompile(`\b(toneFillClass|toneInkClass|toneClass)\(ctx, ([\w.]+)\)`)

// stripTemplComments removes `//` line comments and HTML comments, but not a `//` that is part of a URL: after a scheme
// (`https://`) or opening a scheme-relative one (`"//host"`). A plain `//[^\n]*` cut either short together with the
// class after it on the same line.
func stripTemplComments(src string) string {
	src = htmlComment.ReplaceAllString(src, "")
	return lineComment.ReplaceAllString(src, "$1")
}

var (
	htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	lineComment = regexp.MustCompile(`(^|[^:"'=(])//[^\n]*`)
)

// isToneBox reports whether one class set is a cornered box with a tone hue's tint and the same hue's ink.
// Variant-prefixed tokens (`hover:`, `sm:`) are state and breakpoint, not the box.
func isToneBox(classes string, tint, ink *regexp.Regexp) bool {
	cornered := false
	tints, inks := map[string]bool{}, map[string]bool{}
	for _, tok := range strings.Fields(classes) {
		switch {
		case strings.Contains(tok, ":"):
		case tok == "rounded" || tok == "rounded-sm" || tok == "rounded-md" || tok == "rounded-lg" || tok == "rounded-xl" || tok == "rounded-2xl":
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
	// not a migration. And the board card's due-date chip (`toneClass(ctx, c.Date.Tone)`, a 4px label): a label,
	// not a message, seen once tone variables counted (2026-10-12).
	"machine.templ": 2,
	// The workspace-switch leave guard's warning: bordered, meta-sized, and carrying inline `<b>` markup a Callout's
	// `text` cannot hold. Unseen by this gate until 2026-10-12, because its corner and tint were both readers; the
	// radius rewrite in joinClassExpressions (rencana 0.2b) made it visible.
	"leaveguard.templ": 1,
}
