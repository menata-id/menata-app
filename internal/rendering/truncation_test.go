package rendering

import (
	"bytes"
	"strings"
	"testing"
)

// TestTruncationIsRenderedNotSwallowed is the gate for a signal that existed and was read by nothing.
//
// `composition.Selection.Truncated` shipped 2026-09-29 and had **zero** consumers: the runtime knew a
// `limit:` had cut a list short and no screen said so. 007 §21.9 requires a composed experience
// exceeding its budget to "fail clearly or degrade through an explicit runtime policy", and §20 names
// the neighbouring shape with the word *never* -- a bound that silently drops rows is the same class,
// and at 13 Documents against a limit of 500 it is invisible, which is exactly how it would have stayed.
//
// The assertions read the **limit out of the value under test**, never a literal 500: a test that
// hardcodes the bound passes when the page hardcodes the same bound, which would defeat the point of
// carrying `Selection.Limit` at all (001 #8).
func TestTruncationIsRenderedNotSwallowed(t *testing.T) {
	filters := []FilterChip{{Key: "all", Label: "All", Count: 1, Active: true}}

	for _, tc := range []struct {
		name string
		in   Truncation
		want bool
	}{
		{"bound bit", Truncation{Limit: 500, Hit: true}, true},
		// The common case, and the one that must stay silent: a list inside its bound has nothing to
		// report, so the notice costs no markup on every ordinary render.
		{"bound did not bite", Truncation{Limit: 500, Hit: false}, false},
		// An uncapped list. Hit cannot be true without a limit, but a zero Limit with Hit set is what a
		// future caller building the struct by hand would produce, and rendering "showing the first 0"
		// would be worse than saying nothing.
		{"no bound declared", Truncation{Limit: 0, Hit: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			page := ApprovalInboxPage(nil, nil, nil, nil, nil, nil, filters, tc.in, TabAssigned, "", "Acme", Viewer{Initials: "AN"}, "")
			if err := page.Render(assignedNavFixture(), &buf); err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			html := buf.String()
			got := strings.Contains(html, "Showing the first")
			if got != tc.want {
				t.Errorf("notice rendered = %v, want %v -- a capped list that does not say so is indistinguishable from a complete one", got, tc.want)
			}
			if tc.want && !strings.Contains(html, "500") {
				t.Error("the notice must name the bound it hit; \"some rows were dropped\" tells a reader nothing actionable")
			}
		})
	}
}

// TestTruncationNoticeNamesTheDeclaredLimit is the half the table above cannot assert: that the number
// rendered is the one it was handed rather than a constant that happens to match.
//
// `ds_documents_with_steps` declares 500 today, so every other assertion in this file would pass
// against a page with `500` typed into it. A bound nothing in production uses is the only way to tell
// the difference -- the same trick `stepFieldsForTest` uses for the wizard's input names and
// `modeField` for its option values.
func TestTruncationNoticeNamesTheDeclaredLimit(t *testing.T) {
	var buf bytes.Buffer
	if err := truncationNotice(Truncation{Limit: 37, Hit: true}).Render(assignedNavFixture(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(buf.String(), "37") {
		t.Errorf("notice did not name the limit it was handed; got:\n%s", buf.String())
	}
}
