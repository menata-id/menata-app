package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// darkVariant is Tailwind's `dark:` variant at the start of a class token, behind any other variant prefix.
var darkVariant = regexp.MustCompile(`(?:^|[\s"'{` + "`" + `:])dark:`)

// TestNoSiteWritesADarkVariant holds rencana 3.3's rule (owner decisions DK1–DK11, 2026-10-13): dark mode is one role
// with two values, set in static/css/input.css, and no site writes a `dark:` class. A colour written that way would be
// caught by the palette and role gates, which read variant prefixes; the 3.3 review showed a non-colour one
// (`dark:opacity-50`) passed every gate. Comments are stripped.
func TestNoSiteWritesADarkVariant(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !darkVariant.MatchString(`class="flex dark:opacity-50"`) || !darkVariant.MatchString(`"md:dark:hidden"`) {
		t.Fatal("darkVariant no longer matches a dark: class -- the scan would pass anything")
	}
	scanned := 0
	for _, e := range entries {
		n := e.Name()
		if strings.HasSuffix(n, "_test.go") || strings.HasSuffix(n, "_templ.go") || !(strings.HasSuffix(n, ".templ") || strings.HasSuffix(n, ".go")) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		for _, line := range strings.Split(stripTemplComments(string(b)), "\n") {
			if darkVariant.MatchString(line) {
				t.Errorf("%s writes a dark: class (%s) -- give the role or property a dark value in input.css instead", n, strings.TrimSpace(line))
			}
		}
	}
	if scanned < 20 {
		t.Fatalf("scanned only %d files", scanned)
	}
}
