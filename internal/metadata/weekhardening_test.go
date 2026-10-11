package metadata

import (
	"strings"
	"testing"
)

// Kills the formerly surviving mutation `pageReadsWeek` always true: the existing "a step to a page that reads no $week" case
// links to a navigation item with NO page, so only the `dest.Page == nil` arm was exercised.
func AStepToAPageWhoseDatasetsReadNoWeekIsRefused(t *testing.T) {
	other := `
  - id: nav_other
    label: Other
    route: /pages/nav_other
    page:
      layout: stack
      children:
        - component: Collection
          gap: tight
          empty: none
          binding: { dataset: ds_by_param, rows: records }
          children:
            - static: paragraph
              from: { text: title }
`
	nav := other + strings.Replace(weekToolbar(okStep, okWindow, okWhenKey), "to: nav_week", "to: nav_other", 1)
	err := pageFixture(t, nav)
	if err == nil || !strings.Contains(err.Error(), "reading $week") {
		t.Errorf("a step to a page whose Dataset reads no $week loaded: %v", err)
	}
}

// A step or an operand beyond the clamp (+-520 weeks) is refused at load: a `by` that large only ever lands on the
// bound, and a `when:` operand past it can never equal what the runtime reads.
func TestAWeekNumberBeyondTheClampIsRefusedAtLoad(t *testing.T) {
	for name, c := range map[string]struct{ step, when, want string }{
		"a step past the clamp":     {"          step: { parameter: week, by: 521 }", okWhenKey, "weeks either way"},
		"a negative step past it":   {"          step: { parameter: week, by: -9999999999 }", okWhenKey, "weeks either way"},
		"an operand past the clamp": {okStep, "          when: { parameter: week, is: \"600\" }", "clamped"},
	} {
		if err := pageFixture(t, weekToolbar(c.step, okWindow, c.when)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want an error mentioning %q, got %v", name, c.want, err)
		}
	}
	if err := pageFixture(t, weekToolbar("          step: { parameter: week, by: 520 }", okWindow, "          when: { parameter: week, is: \"-520\" }")); err != nil {
		t.Errorf("the clamp itself was refused: %v", err)
	}
}
