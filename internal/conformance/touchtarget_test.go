package conformance

import (
	"regexp"
	"strings"
	"testing"
)

// TestEveryIconControlHasATouchTarget is a zero-floor gate (gap plan K-05, narrowed by measurement, 2026-10-15): a `<button>`
// or `<summary>` drawn as a 20px icon circle (`size-5`) reads `touchTargetClass()`, which grows its hit area past 24px under a
// coarse pointer without changing how it looks (WCAG 2.5.8). The 2026-10-14 audit measured three such controls under 24px at 360px
// (the completion circle on a board card and in a checklist, and the card's edit pencil -- the two circles are one control since the Toggle, 2026-10-19, so the floor is two sites: the circle and the pencil); the 36px buttons and fields it first named
// already clear 24px. It reads each opening tag in internal/rendering (comments stripped, a `>` inside a quoted attribute or a templ
// expression not closing the tag, as `scanTagEnd` reads it). Blind spots: a control smaller than `size-5` written another way
// (`h-4 w-4`), a link, and a size arriving from a Go function. The sanity floor counts the tags it covered.
func TestEveryIconControlHasATouchTarget(t *testing.T) {
	open := regexp.MustCompile(`(?i)<(?:button|summary)[\s>]`)
	small := regexp.MustCompile(`(?:^|[\s"])size-(?:[1-5]|[1-5]\.5)(?:[\s"]|$)`)
	covered := 0
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
			if !small.MatchString(tag) {
				continue
			}
			if strings.Contains(tag, "touchTargetClass()") {
				covered++
				continue
			}
			t.Errorf("%s: an icon control of %.30q... is smaller than 24px and does not read touchTargetClass() -- add it so a finger has a target", name, strings.Join(strings.Fields(tag), " "))
		}
	}
	if covered < 2 {
		t.Fatalf("only %d icon controls read touchTargetClass(), want at least 2 -- the scan is measuring less than it appears to", covered)
	}
}
