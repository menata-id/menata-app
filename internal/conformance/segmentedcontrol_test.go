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
// segmentMarker is `aria-current` set to "true" in any of the quoting forms templ accepts (`"true"`, `'true'`,
// `{ "true" }`); the review found the first version read only the double-quoted one.
var segmentMarker = regexp.MustCompile(`aria-current\s*=\s*(?:\{\s*)?["'` + "`" + `]true["'` + "`" + `]`)

// segmentedControlExempt are the hand-written places that carry the marker without being a segmented control, each with
// the count and why. Closed both ways.
var segmentedControlExempt = map[string]struct {
	n   int
	why string
}{
	"appshell.templ": {1, "menuLink: one row of a menu (the account menu's, a view group's scope menu), the viewer's current choice marked for assistive technology. A column of links in a panel, not a row of exclusive choices drawn side by side on a track"},
	"tree.templ":     {1, "treeBranch: the selected row of a Tree, one row among a nested list of rows that each open something, marked for assistive technology; a tree is not a row of exclusive choices on a track"},
	"filter.templ":   {1, "filterChipLink: a pill with a count, one per filter (the Approval Inbox's SLA chips, gap-plan P3a, and every declared FilterChips); the active one is marked for assistive technology. Folding it into the control would change its look and its count"},
}

func TestNoHandWrittenSegmentedControl(t *testing.T) {
	open := regexp.MustCompile(`(?i)<[a-z][a-z0-9]*[\s>]`)
	own := 0
	exempt := map[string]int{}
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
			marker := segmentMarker.MatchString(tag)
			track := strings.Contains(tag, "tileClasses(") && strings.Contains(tag, "p-0.5")
			if name == "segmentedcontrol.templ" {
				if marker {
					own++
				}
				continue
			}
			if marker && !track {
				if _, ok := segmentedControlExempt[name]; ok {
					exempt[name]++
					continue
				}
			}
			if marker || track {
				t.Errorf("%s: a hand-written segmented control (%.90q) -- a row of exclusive choices reads @segmentedControl(label, query, options, selected)", name, strings.Join(strings.Fields(tag), " "))
			}
		}
	}
	for name, e := range segmentedControlExempt {
		if exempt[name] != e.n {
			t.Errorf("%s: %d exempt marker(s), listed %d (%s) -- update segmentedControlExempt, or remove the entry if the site is gone", name, exempt[name], e.n, e.why)
		}
	}
	if own != 1 {
		t.Fatalf("segmentedcontrol.templ holds %d aria-current=\"true\" tags, want exactly 1 -- the scan is measuring something other than the control", own)
	}
}
