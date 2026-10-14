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

// typeExpressionExempt is every `<input type={ … }>` whose type is computed, with its count and reason. Closed both ways.
var typeExpressionExempt = map[string]struct {
	n   int
	why string
}{
	// authField's `kind` is "text", "email" or "password" at all eleven call sites (read from the callers, 2026-10-14).
	"authshell.templ": {1, "authField: kind is text, email or password at every caller"},
}

var (
	inputTagStart  = regexp.MustCompile(`(?i)<input[\s>]`)
	checkableType  = regexp.MustCompile(`(?i)\stype\s*=\s*["']?(checkbox|radio)\b`)
	typeExpression = regexp.MustCompile(`(?i)\stype\s*=\s*\{`)
	classAttr      = regexp.MustCompile(`(?s)\sclass\s*=\s*(\{[^}]*\}|"[^"]*"|'[^']*')`)
)

// TestEveryNativeCheckableReadsTheAccent is a zero-floor gate beside `controlAccentClass` (rencana 2.3b, 2026-10-14).
// The plan said "`accent-blue-500` with no control role"; measured, the literal was already gone (CP2 moved it to the
// reader), and what was left was two checkboxes that never had one and drew the browser's own blue. Nothing is
// registered: the boolean control is `FormInput`/`Field`'s `boolean` kind already, drawn at the same two sites.
//
// It reads every `<input type="checkbox|radio">` open tag with the quote- and brace-aware scanner the button gate uses,
// and requires its `class` attribute to call `controlAccentClass(`; a `type={ … }` expression is refused, since the text
// cannot say what it is (the independent review's two surviving mutants). **Blind spot, stated:** a tag assembled in Go.
func TestEveryNativeCheckableReadsTheAccent(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	total := 0
	missing := map[string]int{}
	computed := map[string]int{}
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
			// A `type={ … }` expression cannot be read as text: refuse it rather than count the input as something else.
			if typeExpression.MatchString(tag) {
				computed[e.Name()]++
				continue
			}
			if !checkableType.MatchString(tag) {
				continue
			}
			total++
			// The reader must sit in the class attribute itself: a data-*, title or aria-label naming it applies nothing.
			if m := classAttr.FindStringSubmatch(tag); m == nil || !strings.Contains(m[1], "controlAccentClass(") {
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
	for f, n := range computed {
		if want := typeExpressionExempt[f].n; n != want {
			t.Errorf("%s: %d <input> with a computed type={ … }, exempt %d -- the gate cannot tell whether it is a checkbox; write the type literally, "+
				"or list it in typeExpressionExempt with the values it can take", f, n, want)
		}
	}
	for f, ex := range typeExpressionExempt {
		if computed[f] == 0 {
			t.Errorf("typeExpressionExempt[%q] = %d but the file has none -- drop it", f, ex.n)
		}
		if strings.TrimSpace(ex.why) == "" {
			t.Errorf("typeExpressionExempt names %s with no reason", f)
		}
	}
	t.Logf("%d native checkbox/radio inputs, %d exempt", total, len(checkableExempt))
}
