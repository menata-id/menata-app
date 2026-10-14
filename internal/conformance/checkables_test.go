package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// checkableExempt is every native checkbox or radio in `internal/rendering` that does not read `controlAccentClass`,
// with its reason and count. Closed both ways, like `emptyStateExempt`.
var checkableExempt = map[string]struct {
	n   int
	why string
}{
	// A visually hidden input (`peer sr-only`) behind a drawn switch: the native control is never painted, so its
	// `accent-color` shows nowhere. The switch's track reads the Theme itself.
	"account.templ": {1, "sr-only input behind a drawn switch; the native box is never painted"},
}

var (
	inputTagStart  = regexp.MustCompile(`(?i)<input[\s>]`)
	checkableType  = regexp.MustCompile(`(?i)\btype\s*=\s*["']?(checkbox|radio)\b`)
	accentReadsOne = regexp.MustCompile(`controlAccentClass\(`)
)

// TestEveryNativeCheckableReadsTheAccent is a zero-floor gate beside `controlAccentClass` (rencana 2.3b, 2026-10-14).
// The plan said "`accent-blue-500` with no control role"; measured, the literal was already gone (CP2 moved it to the
// reader), and what was left was two checkboxes that never had one and drew the browser's own blue. Nothing is
// registered: the boolean control is `FormInput`/`Field`'s `boolean` kind already, drawn at the same two sites.
//
// It reads every `<input type="checkbox|radio">` open tag with the quote- and brace-aware scanner the button gate uses,
// and requires the tag to call `controlAccentClass(`. **Blind spots, stated:** a tag assembled in Go, and a reader named
// in the tag that is not applied to `class`.
func TestEveryNativeCheckableReadsTheAccent(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	total := 0
	missing := map[string]int{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		src := stripTemplComments(string(b))
		for i := 0; i < len(src); {
			loc := inputTagStart.FindStringIndex(src[i:])
			if loc == nil {
				break
			}
			start := i + loc[0]
			end := scanTagEnd(src, i+loc[1]-1)
			tag := src[start:end]
			i = end
			if !checkableType.MatchString(tag) {
				continue
			}
			total++
			if !accentReadsOne.MatchString(tag) {
				missing[e.Name()]++
			}
		}
	}
	if total == 0 {
		t.Fatal("no checkbox or radio was found -- the gate is reading the wrong text")
	}
	files := make([]string, 0, len(missing))
	for f := range missing {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		if want := checkableExempt[f].n; missing[f] != want {
			t.Errorf("%s: %d checkbox/radio without controlAccentClass, exempt %d -- a native checkable draws the browser's blue "+
				"unless it reads the accent (class={ controlAccentClass(ctx) }); one that is never painted goes in checkableExempt with its reason",
				f, missing[f], want)
		}
	}
	for f, ex := range checkableExempt {
		if missing[f] == 0 {
			t.Errorf("checkableExempt[%q] = %d but the file has none -- drop it", f, ex.n)
		}
		if strings.TrimSpace(ex.why) == "" {
			t.Errorf("checkableExempt names %s with no reason", f)
		}
	}
	t.Logf("%d native checkbox/radio inputs, %d exempt", total, len(checkableExempt))
}
