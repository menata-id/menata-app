package composition

import (
	"testing"

	"menata.app/internal/data"
	"menata.app/internal/rendering"
)

// TestBuildersCarryTheBoundToTheirView closes the half the rendering gate structurally cannot see, and
// it exists because a mutation found the hole rather than because the hole was predicted.
//
// `rendering.TestTruncationIsRenderedNotSwallowed` proves the page renders a filled Truncation and stays
// silent on an empty one. Deleting the assignment in buildInbox -- so the Data Plane knows the bound bit
// and the view is handed `Truncation{}` -- left that gate **green**: every rendering case still passed,
// because each one constructs its own Truncation. The two halves of the chain were each covered and the
// join between them was not, which is how a signal with zero consumers got shipped in the first place
// (Selection.Truncated, 2026-09-29, read by nothing for a day).
//
// So this asserts the join: a Selection that was cut short produces a view that says so.
func TestBuildersCarryTheBoundToTheirView(t *testing.T) {
	docs := []*data.Record{{ID: "doc_1", Values: map[string]any{"fld_title": "Contract", "fld_status": "in_review"}}}

	// Deliberately not 500. ds_documents_with_steps declares 500, so asserting that number would pass
	// against a builder that hardcoded it instead of reading Selection.Limit -- the same reason
	// stepFieldsForTest renames the wizard's input Fields.
	sel := inboxFixture(docs, nil)
	sel.Truncated, sel.Limit = true, 41

	t.Run("inbox", func(t *testing.T) {
		got := buildInbox(sel, nil, nil, "usr_ana", at(10), stepMachineForTest(), docMachineForTest(), nil)
		assertBound(t, got.Truncated, 41)
	})
	t.Run("assigned", func(t *testing.T) {
		got := buildAssigned(sel, nil, nil, nil, "usr_ana", at(10), stepMachineForTest(), docMachineForTest())
		assertBound(t, got.Truncated, 41)
	})

	// And the inverse, because a builder that always reports truncation is as wrong as one that never
	// does -- it would put a false notice on every ordinary render.
	whole := inboxFixture(docs, nil)
	t.Run("inbox within its bound", func(t *testing.T) {
		if got := buildInbox(whole, nil, nil, "usr_ana", at(10), stepMachineForTest(), docMachineForTest(), nil); got.Truncated.Hit {
			t.Error("an untruncated Selection produced a view claiming its list was cut short")
		}
	})
	t.Run("assigned within its bound", func(t *testing.T) {
		if got := buildAssigned(whole, nil, nil, nil, "usr_ana", at(10), stepMachineForTest(), docMachineForTest()); got.Truncated.Hit {
			t.Error("an untruncated Selection produced a view claiming its list was cut short")
		}
	})
}

func assertBound(t *testing.T, got rendering.Truncation, wantLimit int) {
	t.Helper()
	if !got.Hit {
		t.Error("the Selection was cut short and the view does not say so -- the screen would be indistinguishable from a complete one")
	}
	if got.Limit != wantLimit {
		t.Errorf("view carries limit %d, want %d from the Dataset's own `limit:` -- a bound the view cannot name is one it cannot report", got.Limit, wantLimit)
	}
}
