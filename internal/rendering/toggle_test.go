package rendering

import (
	"bytes"
	"strings"
	"testing"
)

func renderToggle(t *testing.T, describes string, checked bool) string {
	t.Helper()
	var b bytes.Buffer
	if err := toggle(describes, checked, "/machines/mch_t/records/r1", "fld_status", "done").Render(assignedNavFixture(), &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// TestToggle_isOneButtonThatSaysWhatItDoes: the circle is a real submit button in a one-field PATCH form, named for what pressing it
// does to *this* record and pressed for a finished one; a finished record draws the check and the finished fill.
func TestToggle_isOneButtonThatSaysWhatItDoes(t *testing.T) {
	open := renderToggle(t, "Scout", false)
	for _, want := range []string{`hx-patch="/machines/mch_t/records/r1"`, `hx-swap="none"`, `name="fld_status" value="done"`, `type="submit"`, `aria-pressed="false"`, `aria-label="Mark “Scout” complete"`} {
		if !strings.Contains(open, want) {
			t.Errorf("an open Toggle lacks %s: %s", want, open)
		}
	}
	if strings.Contains(open, "<svg") {
		t.Errorf("an open Toggle draws a check: %s", open)
	}
	done := renderToggle(t, "Scout", true)
	for _, want := range []string{`aria-pressed="true"`, `aria-label="Mark “Scout” not complete"`, "<svg", "bg-emerald-600"} {
		if !strings.Contains(done, want) {
			t.Errorf("a finished Toggle lacks %s: %s", want, done)
		}
	}
	if strings.Contains(done, "bg-emerald-600") == strings.Contains(open, "bg-emerald-600") {
		t.Error("a finished and an open Toggle are filled the same")
	}
}

// TestCompletionCircle_isOneControlWhereverItIsDrawn: the Toggle, a board card's hover circle and a checklist row all draw
// `completionCircle`, so the three cannot come to look or sound different. A hand-written circle beside it would pass every other
// test and drift the first time one of them is edited.
func TestCompletionCircle_isOneControlWhereverItIsDrawn(t *testing.T) {
	var circle bytes.Buffer
	if err := completionCircle(completeLabel(true, "Scout"), true).Render(assignedNavFixture(), &circle); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(renderToggle(t, "Scout", true), circle.String()) {
		t.Errorf("the Toggle does not draw the shared circle:\n%s\nvs\n%s", renderToggle(t, "Scout", true), circle.String())
	}
}
