package conformance

import (
	"regexp"
	"strings"
	"testing"
)

// TestNoHandWrittenDisclosure is a zero-floor gate beside `Disclosure` (rencana T-A, first step, 2026-10-14): a panel
// that opens on a click -- a `<details>` drawn as a surface -- is `@disclosure(summary) { ... }`, so its surface,
// summary size, weight and ink follow one contract. It reads each `<details ...>` opening tag in internal/rendering
// (comments stripped, a `>` inside a quoted attribute or a templ expression not closing the tag, as `scanTagEnd` reads
// it) and refuses one that reads a surface (`surfaceClasses(`, `tileClasses(`) or writes the surface recipe's literal
// corner (`rounded-lg`). What it cannot see: a `<details>` whose surface arrives from a Go function or a variable, and a
// panel built on another element. The other `<details>` in the tree are not panels
// (the tail fold hides its own summary, the card menus open a floating list, the archived fold is a quiet line), and the
// sanity floor below keeps the scan from measuring nothing.
func TestNoHandWrittenDisclosure(t *testing.T) {
	open := regexp.MustCompile(`(?i)<details[\s>]`)
	total := 0
	for _, name := range sortedFileNames(renderingTemplSources(t)) {
		src := stripTemplComments(renderingTemplSources(t)[name])
		for i := 0; i < len(src); {
			loc := open.FindStringIndex(src[i:])
			if loc == nil {
				break
			}
			start := i + loc[0]
			end := scanTagEnd(src, i+loc[1]-1)
			tag := src[start:end]
			i = end
			total++
			surface := strings.Contains(tag, "surfaceClasses(") || strings.Contains(tag, "tileClasses(") || strings.Contains(tag, "rounded-lg")
			if surface && name != "disclosure.templ" {
				t.Errorf("%s: a hand-written panel disclosure (%.80q) -- a panel that opens reads @disclosure(summary) { ... }", name, strings.Join(strings.Fields(tag), " "))
			}
		}
	}
	if total < 8 {
		t.Fatalf("only %d <details> tags were found -- the scan is measuring far less than it appears to", total)
	}
}
