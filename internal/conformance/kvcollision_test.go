package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoTwoKVKeysFromOneThemeReader is a zero-floor gate (2026-10-12). One class expression may not hold two
// `templ.KV` keys built from the **same** Theme reader (`toneClass`, `inkClass`, ...).
//
// **Why.** A Workspace's `theme:` may map two roles to one step: `good` and `info` to one palette, `strong` and
// `body` to one shade. The two keys are then the same string, and templ keeps the last value, so whichever state
// is true can lose its class. The default theme never collides, so a byte-diff cannot see it. Two sites shipped
// this way and drew a done chip without colour (`approvalinbox`) and a current settings link without ink
// (`appsettings`). Both were found by a test under a remapped theme, and both now choose the class in Go.
//
// Keys from **different** readers cannot collide (a `bg-` class is never a `font-` class), so they are allowed;
// counting them would be a false finding on every state-styled element. Blind spot: a key built from a Go helper
// that calls a reader inside it.
func TestNoTwoKVKeysFromOneThemeReader(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	comment := regexp.MustCompile(`//[^\n]*`)
	kvKey := regexp.MustCompile(`templ\.KV\(\s*(\w+)\(ctx`)
	expressions := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		for _, expr := range classExpression.FindAllString(comment.ReplaceAllString(string(b), ""), -1) {
			expressions++
			seen := map[string]int{}
			for _, m := range kvKey.FindAllStringSubmatch(expr, -1) {
				seen[m[1]]++
			}
			for reader, n := range seen {
				if n > 1 {
					t.Errorf("%s: %d templ.KV keys from %s(ctx, ...) in one class expression -- a theme mapping two roles to one "+
						"step makes the keys one string and templ keeps the last value; choose the class in Go: %s",
						e.Name(), n, reader, strings.Join(strings.Fields(expr), " "))
				}
			}
		}
	}
	if expressions == 0 {
		t.Fatal("no class expression was scanned -- the gate is reading the wrong text")
	}
}
