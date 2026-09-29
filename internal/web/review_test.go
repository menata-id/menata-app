package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/action"
	"menata.app/internal/authorization"
	"menata.app/internal/data"
	"menata.app/internal/rendering"
)

// TestShowReviewDocument_opensOnAStepWhenGivenADocument is the regression test for a live 404 that
// ran in production for a day (2026-09-28), and it exists because nothing covered this route at all.
//
// The Review screen accepts either a Step id or a *Document* id -- My Documents lists Documents, and
// the viewer there is the submitter, who has no step of their own to point at, so
// composition.ReviewStepForDocument resolves which step the screen opens on. That function looks a
// Document's steps up by the Field reaching the parent, and since the derivation slice that Field came
// from action.DeclaredFields(stepMachine, docMachine) -- called with **nil** for the document Machine.
//
// The parent Field is derived by comparing the step Machine's relations against the document Machine's
// id, so nil meant no id to match, an empty Field name, a ListRecordsBy filtering on "" and zero rows.
// Every Document then looked like a Document with no steps, which is a legitimate state with a
// legitimate answer: 404. **That is why it was invisible** -- the screen failed in a shape it is
// supposed to have, no error was logged, no test failed, and the only signal was three 404s in an
// access log.
//
// So this asserts the plain thing no test asserted: a Document that has steps opens its Review screen.
func TestShowReviewDocument_opensOnAStepWhenGivenADocument(t *testing.T) {
	s := newDecideStepTestSetup(t, "review_by_document_id")

	rec := getReviewAs(t, s, s.ids.document, s.documentID, s.assignee)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET review by Document id = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	// And by Step id, the other half of the same route -- so a fix to one arm cannot silently break
	// the other.
	if rec := getReviewAs(t, s, s.ids.step, s.stepID, s.assignee); rec.Code != http.StatusOK {
		t.Errorf("GET review by Step id = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

// TestShowReviewDocument_documentWithNoStepsIs404 holds the branch the bug was hiding behind. A
// Document really can have no Approval Steps -- the generic create form makes one, the wizard never
// does -- and 404 is the right answer for it. Asserting it here is what keeps the fix above from
// being "make /review always return 200", which would have passed the test above just as well.
func TestShowReviewDocument_documentWithNoStepsIs404(t *testing.T) {
	s := newDecideStepTestSetup(t, "review_no_steps")

	bare, err := s.store.CreateRecord(s.ctx, s.ids.document, map[string]any{
		"fld_title": "No steps at all", "fld_status": "in_review",
	})
	if err != nil {
		t.Fatalf("CreateRecord(document): %v", err)
	}
	t.Cleanup(func() { _ = s.store.DeleteRecord(s.ctx, s.ids.document, bare.ID) })

	if rec := getReviewAs(t, s, s.ids.document, bare.ID, s.assignee); rec.Code != http.StatusNotFound {
		t.Errorf("GET review for a Document with no steps = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}

// TestShowReviewDocument_opensOnTheViewersOwnDecidedStep is the regression test for
// composition.ReviewStepForDocument's step 0 (2026-09-29): an approver who already decided their
// own step, then reopens the Document from a link (My Documents, an activity entry, a bookmark --
// anything handing the screen a Document id rather than their own Step id), must see *their own*
// decision reflected, not the sibling step the Document is now waiting on instead.
//
// Found on a real Document: the first of two approvers approved, then revisited via the Document
// link and saw the second approver's still-pending step -- the footer read "Awaiting its own
// assignee's decision" with no mention that the viewer's own had already landed, while the
// Approval Progress list right beside it already said "Approved". Two panels, same viewer, two
// different answers, because only one of them was ever asked about the viewer's own step.
func TestShowReviewDocument_opensOnTheViewersOwnDecidedStep(t *testing.T) {
	s := newDecideStepTestSetup(t, "review_own_decided_step")

	if rec := postDecideAs(t, s, s.stepID, s.assignee, action.DecisionApproved, map[string]string{
		"signature_image": testSignaturePNGDataURL(t),
	}); rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Fatalf("approve step 1: %d, want a redirect; body=%s", rec.Code, rec.Body.String())
	}

	rec := getReviewAs(t, s, s.ids.document, s.documentID, s.assignee)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET review by Document id = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "Awaiting its own assignee's decision") {
		t.Errorf("review page for the assignee who already decided still reads as if nobody had:\n%s", body)
	}
	if !strings.Contains(body, action.DecisionApproved) {
		t.Errorf("review page does not reflect the viewer's own %q decision:\n%s", action.DecisionApproved, body)
	}
}

// getReviewAs issues the real GET through the real route, the same shape postDecideAs already uses
// for the decide route -- this bug was only visible end to end, so the test is end to end.
func getReviewAs(t *testing.T, s decideStepTestSetup, machineID, recordID, actorID string) *httptest.ResponseRecorder {
	t.Helper()
	// The *real* installed Workspace, not testWorkspaceFor: this screen renders a full page, and
	// pageShell resolves its back link through routeByID("nav_approval_inbox"), which a Workspace
	// carrying Machines but no Application cannot answer. Using the real one is also the stronger
	// test -- the route then runs against the metadata that actually ships.
	_, installed := loadRealMachines(t)
	req := httptest.NewRequest(http.MethodGet, "/machines/"+machineID+"/records/"+recordID+"/review", nil)
	req = req.WithContext(rendering.WithCurrentWorkspace(data.WithWorkspaceScope(req.Context(), s.workspaceID), installed, "Test Workspace", false))
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, s.cfg, actorID, 0)})

	r := chi.NewRouter()
	r.Get("/machines/{machineID}/records/{id}/review", showReviewDocument(s.store, s.files, s.cfg))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}
