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
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// reviseDocument had no test at all until 2026-09-29, and that is why these exist.
//
// It is a declared-Action write path -- action.ApplyEffect over mch_document's own `actions:` block
// (Stage B) -- gated by a declared Permission and a business-state guard. And it was edited by
// `14c9223`, the same commit that broke /review, by the same kind of edit: a literal Field id
// (action.FieldDocumentStatus) replaced with a derivation (machine.StatusField()). That change was
// correct here, but nothing proved it, which is the same position /review was in the day before it
// 404'd for everyone.
//
// So these assert the three things this route promises, and the first one asserts it *through the
// declaration* rather than against the string "draft" -- otherwise the test would keep passing over a
// Machine whose `actions:` block says something else entirely.

// TestReviseDocument_writesTheStatusTheMachineDeclares is the happy path, and it checks the declared
// value rather than a literal: `revise` writes whatever mch_document's own actions: block names.
//
// **What that does and does not prove**, since both sides read the same declaration. Changing the
// declared value alone does *not* fail this test -- want and got move together, which is correct: the
// property is "the route honours the declaration", not "the declaration says draft". What fails it is
// the route writing a literal while the declaration says something else, which is the actual
// regression to fear and was mutation-proved that way (declaration `in_review`, handler `draft` ->
// fails). Stated here because a reader mutating the YAML and seeing green would otherwise conclude
// this test is vacuous.
func TestReviseDocument_writesTheStatusTheMachineDeclares(t *testing.T) {
	s := newDecideStepTestSetup(t, "revise_declared")
	rejectDocument(t, s)

	rec := postRevise(t, s, s.documentID, s.assignee)
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Fatalf("revise = %d, want a redirect; body=%s", rec.Code, rec.Body.String())
	}

	machine := realDocumentMachine(t)
	want := declaredWrite(t, machine, domain.ActionRevise)
	document, err := s.store.GetRecord(s.ctx, s.ids.document, s.documentID)
	if err != nil {
		t.Fatalf("GetRecord(document): %v", err)
	}
	if got := toDisplayString(document.Values[machine.StatusField()]); got != want {
		t.Errorf("%s = %q, want %q -- the value mch_document's own actions: block declares for revise",
			machine.StatusField(), got, want)
	}

	// Revising clears the decided steps, so the chain is rebuilt rather than resumed mid-rejection.
	steps, err := s.store.ListRecordsBy(s.ctx, s.ids.step, action.FieldStepDocument, s.documentID)
	if err != nil {
		t.Fatalf("ListRecordsBy(steps): %v", err)
	}
	if len(steps) != 0 {
		t.Errorf("revise left %d Approval Step(s) behind -- a revised Document starts its approval over", len(steps))
	}
}

// TestReviseDocument_refusesADocumentThatIsNotRejected holds the business-state guard
// (action.CanReviseDocument). Without it a submitter could pull a Document back out of review, or out
// of approved, after other people had acted on it.
func TestReviseDocument_refusesADocumentThatIsNotRejected(t *testing.T) {
	s := newDecideStepTestSetup(t, "revise_wrong_state")

	rec := postRevise(t, s, s.documentID, s.assignee)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("revise of an in-review Document = %d, want 422; body=%s", rec.Code, rec.Body.String())
	}
}

// TestReviseDocument_refusesAnActorThePermissionDoesNotAllow holds prm_revise_document's own roles:
// arm. A reviewer may look at a Document and nothing else (owner decision, 2026-09-21), so the
// interesting caller is one holding a real Application role that is not on the list.
func TestReviseDocument_refusesAnActorThePermissionDoesNotAllow(t *testing.T) {
	s := newDecideStepTestSetup(t, "revise_wrong_role")
	rejectDocument(t, s)

	reviewer := addMemberWithAppRole(t, s, "reviewer")
	rec := postRevise(t, s, s.documentID, reviewer)
	if rec.Code != http.StatusForbidden {
		t.Errorf("revise as a reviewer = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}

	// And the state did not move, which is the assertion that would catch a refusal that returns 403
	// *after* writing.
	document, err := s.store.GetRecord(s.ctx, s.ids.document, s.documentID)
	if err != nil {
		t.Fatalf("GetRecord(document): %v", err)
	}
	if got := toDisplayString(document.Values[realDocumentMachine(t).StatusField()]); got != action.DocumentStatusRejected {
		t.Errorf("status = %q after a refused revise, want it untouched at %q", got, action.DocumentStatusRejected)
	}
}

// realDocumentMachine is the Document Machine as the default Workspace actually installs it -- the
// same one the handler resolves off ctx, so an assertion here cannot drift from what ran.
func realDocumentMachine(t *testing.T) *domain.Machine {
	t.Helper()
	machines, _ := loadRealMachines(t)
	m, ok := machines[action.DocumentMachineID]
	if !ok {
		t.Fatalf("the default Workspace installs no %s", action.DocumentMachineID)
	}
	return m
}

// declaredWrite reads the value a Machine's own actions: block writes for one Action -- so a test can
// assert "what the declaration says" instead of restating it.
func declaredWrite(t *testing.T, m *domain.Machine, actionName string) string {
	t.Helper()
	effect, ok := m.EffectFor(actionName)
	if !ok {
		t.Fatalf("%s declares no %s effect -- this fixture cannot assert what the route writes", m.ID, actionName)
	}
	for _, w := range effect.Writes {
		if w.Value != "" {
			return w.Value
		}
	}
	t.Fatalf("%s's %s effect declares no literal value to check", m.ID, actionName)
	return ""
}

// rejectDocument puts the seeded Document into the one state revise accepts, through the real decide
// route rather than by writing the status directly -- so the fixture reaches that state the way a
// person does, and the rollup that derives it stays in the path.
func rejectDocument(t *testing.T, s decideStepTestSetup) {
	t.Helper()
	if rec := postDecide(t, s, action.DecisionRejected, nil); rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Fatalf("reject the document first: status = %d; body=%s", rec.Code, rec.Body.String())
	}
	document, err := s.store.GetRecord(s.ctx, s.ids.document, s.documentID)
	if err != nil {
		t.Fatalf("GetRecord(document): %v", err)
	}
	if got := toDisplayString(document.Values[realDocumentMachine(t).StatusField()]); got != action.DocumentStatusRejected {
		t.Fatalf("document is %q, not rejected -- revise's own precondition was not reached", got)
	}
}

// addMemberWithAppRole adds a second member holding one Application role, for the Permission tests.
func addMemberWithAppRole(t *testing.T, s decideStepTestSetup, role string) string {
	t.Helper()
	record, err := s.store.CreateRecord(s.ctx, domain.UserMachineID, map[string]any{})
	if err != nil {
		t.Fatalf("CreateRecord(member): %v", err)
	}
	t.Cleanup(func() { _ = s.store.DeleteRecord(s.ctx, domain.UserMachineID, record.ID) })
	email := role + "-" + record.ID + "@example.com"
	if err := s.store.AddMember(s.ctx, s.workspaceID, record.ID, email, "member", ""); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if err := s.store.SetMemberAppRole(s.ctx, s.workspaceID, record.ID, "app_document_approval", role); err != nil {
		t.Fatalf("SetMemberAppRole: %v", err)
	}
	return record.ID
}

// postRevise posts the revise route the same way postDecideAs posts decide -- the handler mounted
// directly, session cookie only, since this package's handler tests do not run the CSRF middleware
// (TestPostRoutesRefuseUnauthenticatedAndUnCSRFed is what covers that, for every POST at once).
func postRevise(t *testing.T, s decideStepTestSetup, documentID, actorID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/machines/"+s.ids.document+"/records/"+documentID+"/revise", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// The real installed Workspace, not testWorkspaceFor: this route redirects through
	// navRouteByID("nav_my_documents"), which a Workspace carrying Machines but no Application cannot
	// answer -- it panics. Using the real one also means the route runs against the metadata that
	// ships, which is the stronger test (getReviewAs takes the same shape for the same reason).
	_, installed := loadRealMachines(t)
	req = req.WithContext(rendering.WithCurrentWorkspace(data.WithWorkspaceScope(req.Context(), s.workspaceID), installed, "Test Workspace", false))
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, s.cfg, actorID, 0)})

	r := chi.NewRouter()
	r.Post("/machines/{machineID}/records/{id}/revise", reviseDocument(s.store, s.cfg))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}
