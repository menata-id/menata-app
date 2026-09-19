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
)

// putRecordAs PUTs a form-encoded update to machineID/id, as actorID, through the real
// updateRecordForm handler -- the generic route every RecordRow/signature-placement form also
// submits to, so this exercises exactly what a browser would send.
func putRecordAs(t *testing.T, s decideStepTestSetup, machineID, id, actorID string, form map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	values := make([]string, 0, len(form))
	for k, v := range form {
		values = append(values, k+"="+v)
	}
	req := httptest.NewRequest(http.MethodPut, "/machines/"+machineID+"/records/"+id, strings.NewReader(strings.Join(values, "&")))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(data.WithWorkspaceScope(req.Context(), s.workspaceID))
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, s.cfg, actorID, 0)})

	r := chi.NewRouter()
	r.Put("/machines/{machineID}/records/{id}", updateRecordForm(s.machines, s.store, s.files, s.cfg))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// deleteRecordAs DELETEs machineID/id as actorID, through the real deleteRecord handler.
func deleteRecordAs(t *testing.T, s decideStepTestSetup, machineID, id, actorID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/machines/"+machineID+"/records/"+id, nil)
	req = req.WithContext(data.WithWorkspaceScope(req.Context(), s.workspaceID))
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, s.cfg, actorID, 0)})

	r := chi.NewRouter()
	r.Delete("/machines/{machineID}/records/{id}", deleteRecord(s.machines, s.store, s.cfg))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// approvalStepEditForm carries the same fields sigMarker/placeStepForm/widthControl's own hidden
// inputs do (internal/rendering/signatureplacement.templ's stepHiddenFields), a real edit through
// the generic route rather than a partial one.
func approvalStepEditForm(s decideStepTestSetup, stepID string) map[string]string {
	return map[string]string{
		action.FieldStepDocument:       s.documentID,
		action.FieldStepSequence:       "1",
		action.FieldStepAssignee:       s.assignee,
		action.FieldStepDecision:       action.DecisionPending,
		action.FieldStepSignaturePage:  "1",
		action.FieldStepSignatureX:     "60",
		action.FieldStepSignatureY:     "60",
		action.FieldStepSignatureWidth: "20",
	}
}

// TestUpdateRecordForm_blocksNonAssigneeEdit is the closed gap this permission change exists for:
// before it, any authenticated Workspace member could drag/reposition another approver's own
// signature marker (or edit the step's other fields) through the generic PUT route, not just its
// own assignee (prm_edit_own_step, metadata/approval_step.yaml).
func TestUpdateRecordForm_blocksNonAssigneeEdit(t *testing.T) {
	s := newDecideStepTestSetup(t, "update_blocks_non_assignee")

	rec := putRecordAs(t, s, action.StepMachineID, s.stepID, s.assignee2, approvalStepEditForm(s, s.stepID))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("updateRecordForm(non-assignee) status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}

	step, err := s.store.GetRecord(s.ctx, action.StepMachineID, s.stepID)
	if err != nil {
		t.Fatalf("GetRecord(step): %v", err)
	}
	if got := step.Values[action.FieldStepSignatureX]; got != float64(50) {
		t.Errorf("step fld_signature_x = %v, want unchanged (50) after a refused edit", got)
	}
}

// TestUpdateRecordForm_allowsAssigneeEdit is the other side of the same gate: the step's own
// assignee can still edit it, unaffected by the new check.
func TestUpdateRecordForm_allowsAssigneeEdit(t *testing.T) {
	s := newDecideStepTestSetup(t, "update_allows_assignee")

	rec := putRecordAs(t, s, action.StepMachineID, s.stepID, s.assignee, approvalStepEditForm(s, s.stepID))
	if rec.Code != http.StatusOK {
		t.Fatalf("updateRecordForm(assignee) status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	step, err := s.store.GetRecord(s.ctx, action.StepMachineID, s.stepID)
	if err != nil {
		t.Fatalf("GetRecord(step): %v", err)
	}
	if got := step.Values[action.FieldStepSignatureX]; got != float64(60) {
		t.Errorf("step fld_signature_x = %v, want 60 after the assignee's own edit", got)
	}
}

// TestDeleteRecord_blocksNonAssigneeDelete mirrors the edit gate for delete
// (prm_delete_own_step): only the step's own assignee may delete it, ANDed with the pre-existing
// CanDeleteApprovalStep business-state guard (deleteAllowed, internal/web/record.go).
func TestDeleteRecord_blocksNonAssigneeDelete(t *testing.T) {
	s := newDecideStepTestSetup(t, "delete_blocks_non_assignee")

	rec := deleteRecordAs(t, s, action.StepMachineID, s.stepID, s.assignee2)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("deleteRecord(non-assignee) status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if _, err := s.store.GetRecord(s.ctx, action.StepMachineID, s.stepID); err != nil {
		t.Errorf("GetRecord(step) after a refused delete: %v, want the step to still exist", err)
	}
}

// TestDeleteRecord_allowsAssigneeDelete: the assignee can still delete its own pending step.
func TestDeleteRecord_allowsAssigneeDelete(t *testing.T) {
	s := newDecideStepTestSetup(t, "delete_allows_assignee")

	rec := deleteRecordAs(t, s, action.StepMachineID, s.stepID, s.assignee)
	if rec.Code != http.StatusOK {
		t.Fatalf("deleteRecord(assignee) status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if _, err := s.store.GetRecord(s.ctx, action.StepMachineID, s.stepID); err == nil {
		t.Error("GetRecord(step) after delete: want an error, the record should be gone")
	}
}

// TestUpdateRecordForm_unrestrictedMachineAllowsAnyAuthenticatedUser is the regression guard: a
// Machine declaring no edit Permission (mch_document here, same as every Machine other than
// mch_approval_step) must behave exactly as before this change -- any authenticated Workspace
// member may still edit it, not just some declared actor.
func TestUpdateRecordForm_unrestrictedMachineAllowsAnyAuthenticatedUser(t *testing.T) {
	s := newDecideStepTestSetup(t, "update_unrestricted_machine")

	rec := putRecordAs(t, s, action.DocumentMachineID, s.documentID, s.assignee2, map[string]string{
		action.FieldDocumentMode:   "sequential",
		action.FieldDocumentStatus: action.DocumentStatusInReview,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("updateRecordForm(unrestricted Machine, non-owner) status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}
