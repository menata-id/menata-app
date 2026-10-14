package conformance

import (
	"regexp"
	"strings"
	"testing"
)

// TestNoHandWrittenSegmentedControl is a zero-floor gate beside `SegmentedControl` (rencana 2.5 step 1, 2026-10-14):
// a row of mutually exclusive choices of the page, one marked, is `@segmentedControl(label, query, options, selected)`,
// so its track, knob and inks follow one contract. The view switch was the one site and is the first use.
//
// It reads each opening tag in internal/rendering (comments stripped, a `>` inside a quoted attribute or a templ
// expression not closing the tag, as `scanTagEnd` reads it) and refuses, outside segmentedcontrol.templ, either half of
// the recipe: the marker (`aria-current="true"`, the value only this control writes -- the others in the tree are
// `page` and `date`) or the track (`p-0.5` on an element that reads `tileClasses(`). The sanity check requires the
// control's own file to hold exactly one marker, so a rewrite of the marker cannot leave the scan measuring nothing.
// What it cannot see: a choice row drawn with another marker (`aria-pressed`, a radio group), a class from a Go function.
// The Builder mockup's `role="radiogroup"` is another job (a choice held in the page, switched by script) and is not
// registered.
func TestNoHandWrittenSegmentedControl(t *testing.T) {
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
			marker := strings.Contains(tag, `aria-current="true"`)
			track := strings.Contains(tag, "tileClasses(") && strings.Contains(tag, "p-0.5")
			if name == "segmentedcontrol.templ" {
				if marker {
					own++
				}
				continue
			}
			if marker || track {
				t.Errorf("%s: a hand-written segmented control (%.90q) -- a row of exclusive choices reads @segmentedControl(label, query, options, selected)", name, strings.Join(strings.Fields(tag), " "))
			}
		}
	}
	if own != 1 {
		t.Fatalf("segmentedcontrol.templ holds %d aria-current=\"true\" tags, want exactly 1 -- the scan is measuring something other than the control", own)
	}
}
