package conformance

import (
	"regexp"
	"strings"
	"testing"
)

// TestNoHandWrittenPopover is a zero-floor gate beside `Popover` (gap plan K-04, 2026-10-15): a panel that floats under
// its trigger is `@popover(label, title, trigger, align, density) { ... }`, so its panel recipe (the surface, the shadow, the
// width and the 6px under the trigger), its trigger looks and its room follow one contract. It reads each opening tag in
// internal/rendering (comments stripped, a `>` inside a quoted attribute or a templ expression not closing the tag, as
// `scanTagEnd` reads it) and refuses, outside popover.templ, one that is positioned (`absolute`), reads the surface recipe
// (`surfaceClasses(`) and carries `shadow-lg`.
//
// What it cannot see: a floating panel whose surface or shadow arrives from a Go function (the app shell's `sheetPanel`
// does, and is the native Popover API with its own bottom sheet on a phone, a different thing), and an `absolute` panel
// with no shadow. The board card's quick edit and a lane's "Add a card" expand in place (no `absolute` surface, no shadow)
// and are not popovers. The sanity check requires popover.templ to hold exactly one such tag.
func TestNoHandWrittenPopover(t *testing.T) {
	open := regexp.MustCompile(`(?i)<[a-z][a-z0-9]*[\s>]`)
	own := 0
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
			panel := strings.Contains(tag, "surfaceClasses(") && strings.Contains(tag, "shadow-lg") && regexp.MustCompile(`(?:^|[\s"])absolute(?:[\s"]|$)`).MatchString(tag)
			if !panel {
				continue
			}
			if name == "popover.templ" {
				own++
				continue
			}
			t.Errorf("%s: a hand-written floating panel (%.90q) -- a panel under a trigger reads @popover(label, title, trigger, align, density) { ... }", name, strings.Join(strings.Fields(tag), " "))
		}
	}
	if own != 1 {
		t.Fatalf("popover.templ holds %d floating panel tags, want exactly 1 -- the scan is measuring something other than the component", own)
	}
}
