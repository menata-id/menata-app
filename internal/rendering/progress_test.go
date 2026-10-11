package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func renderMetric(t *testing.T, hint string, tone domain.BadgeTone) string {
	t.Helper()
	var buf bytes.Buffer
	if err := metric("Overdue", "3", hint, "", tone).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

// A Metric's value is drawn in its tone's ink and the hint line is optional: the three older screens pass
// neither and must not grow an empty hint element.
func TestMetric_hintAndToneAreOptional(t *testing.T) {
	plain := renderMetric(t, "", "")
	if strings.Contains(plain, "text-2xs text-slate-400") {
		t.Errorf("a Metric with no hint drew a hint line: %s", plain)
	}
	if strings.Contains(plain, "text-red-700") || strings.Contains(plain, "text-emerald-700") {
		t.Errorf("a Metric with no tone drew a signal colour: %s", plain)
	}

	red := renderMetric(t, "Past its due date", domain.ToneBad)
	if !strings.Contains(red, "text-red-700") || !strings.Contains(red, ">Past its due date<") {
		t.Errorf("a bad-tone Metric with a hint did not draw both: %s", red)
	}
	if green := renderMetric(t, "", domain.ToneGood); !strings.Contains(green, "text-emerald-700") {
		t.Errorf("a good-tone Metric did not draw green: %s", green)
	}
}

// progressRow fills to value out of max, clamped, and always says the two numbers in words -- the fill is never
// the only thing saying how much. Nothing out of nothing is an empty bar, not a division by zero.
func TestProgressRow_fillsToTheShareAndSaysItInWords(t *testing.T) {
	for _, tc := range []struct{ value, max, width string }{
		{"2", "3", "width:66%"}, {"3", "3", "width:100%"}, {"5", "3", "width:100%"}, {"0", "0", "width:0%"}, {"1", "0", "width:0%"}, {"-2", "3", "width:0%"},
	} {
		var buf bytes.Buffer
		if err := progressRow("Silvia", tc.value, tc.max).Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		html := buf.String()
		if !strings.HasPrefix(strings.TrimSpace(html), `<div class="w-full">`) {
			t.Errorf("the bar's wrapper must fill its row, or a Collection item leaves the bar 0px wide:\n%s", html)
		}
		words := tc.value + " of " + tc.max
		if !strings.Contains(html, tc.width) || !strings.Contains(html, `aria-valuetext="`+words+`"`) || !strings.Contains(html, ">"+words+"<") || !strings.Contains(html, `aria-label="Silvia"`) {
			t.Errorf("%s: want %s and the words %q:\n%s", words, tc.width, words, html)
		}
	}
}
