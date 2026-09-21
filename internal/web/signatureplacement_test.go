package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/action"
	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// TestSignaturePlacementPut_preservesApproverFields is the test Fase 6c-2's doc comment claimed
// already existed. It did not: that comment named a test in this package that was never written,
// while the only real guard was a render test in internal/rendering. A written claim standing in
// for the artifact it describes -- inside the fix whose own argument is that claims must match
// artifacts. Written here in Fase 6c-3, and the comment that named it is gone.
//
// What it actually holds: the placement forms PUT to the *generic* update route, which rewrites a
// record from the submitted form, and data.ValuesFromForm drops whatever is absent. So a field the
// form does not echo back is erased -- with no error, because nothing was invalid. The render test
// proves the inputs are drawn; only this proves the round-trip keeps the values.
func TestSignaturePlacementPut_preservesApproverFields(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()

	ws, err := store.CreateWorkspace(ctx, "Placement PUT Preserve", "placement-put-preserve-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, "placement_put_preserve@example.com")
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)

	group, err := store.CreateGroup(wsCtx, ws.ID, "Legal Group")
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	document, err := store.CreateRecord(wsCtx, action.DocumentMachineID, map[string]any{"fld_title": "Contract"})
	if err != nil {
		t.Fatalf("CreateRecord(document): %v", err)
	}
	step, err := store.CreateRecord(wsCtx, action.StepMachineID, map[string]any{
		action.FieldStepDocument:       document.ID,
		action.FieldStepSequence:       float64(1),
		action.FieldStepName:           "Legal Review",
		action.FieldStepApproverType:   domain.ActorKindGroup,
		action.FieldStepApproverGroup:  group.ID,
		action.FieldStepDecision:       action.DecisionPending,
		action.FieldStepSignatureImage: "sig_one_time_key",
	})
	if err != nil {
		t.Fatalf("CreateRecord(step): %v", err)
	}

	// A real identity, and a real membership of the owning Group. Both halves matter: Fase 6c-3
	// is what makes a Group-held step editable at all, and before it this PUT was refused for
	// everyone -- which is exactly the 403 this test hits if the metadata arm is removed.
	member, err := store.CreateRecord(wsCtx, domain.UserMachineID, map[string]any{"fld_name": "Budi", "fld_email": "budi@example.com"})
	if err != nil {
		t.Fatalf("CreateRecord(user): %v", err)
	}
	if err := store.SetGroupMembers(wsCtx, group.ID, []string{member.ID}); err != nil {
		t.Fatalf("SetGroupMembers: %v", err)
	}
	// The role half, added in Fase 7: prm_edit_own_step now also requires an Application role
	// (CAP-P01), so being in the owning Group is necessary and no longer sufficient. Granted
	// through the Group rather than directly on purpose -- it exercises the property CAP-O07 and
	// CAP-P01 have to agree on, that a role held through a Group gates identically to one held
	// directly (data.EffectiveRoles), on the one request path that reads both.
	if err := store.SetGroupAppRole(wsCtx, group.ID, "app_document_approval", "approver"); err != nil {
		t.Fatalf("SetGroupAppRole: %v", err)
	}

	machines := loadRealMachines(t)
	cfg := config.Config{SessionSecret: "test-secret-for-placement-put"}

	// Exactly what a marker drag submits, and nothing else -- which is the point of the route it
	// now goes to. The three forms on board 09 send four Fields; every other Field on the step is
	// untouched because the route never writes it, rather than because the form remembered to
	// echo it back.
	form := url.Values{}
	form.Set(action.FieldStepSignaturePage, "1")
	form.Set(action.FieldStepSignatureX, "20.0")
	form.Set(action.FieldStepSignatureY, "84.0")
	form.Set(action.FieldStepSignatureWidth, "25")

	r := chi.NewRouter()
	r.Put("/machines/{machineID}/records/{id}/signature-placement", updateSignaturePlacement(machines, store, cfg))
	req := httptest.NewRequest(http.MethodPut,
		fmt.Sprintf("/machines/%s/records/%s/signature-placement", action.StepMachineID, step.ID),
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, cfg, member.ID, 0)})
	req = req.WithContext(wsCtx)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code >= 400 {
		t.Fatalf("PUT status = %d; body=%s", rec.Code, rec.Body.String())
	}

	after, err := store.GetRecord(wsCtx, action.StepMachineID, step.ID)
	if err != nil {
		t.Fatalf("GetRecord: %v", err)
	}
	for field, want := range map[string]string{
		action.FieldStepName:           "Legal Review",
		action.FieldStepApproverType:   domain.ActorKindGroup,
		action.FieldStepApproverGroup:  group.ID,
		action.FieldStepSignatureImage: "sig_one_time_key",
		action.FieldStepDecision:       action.DecisionPending,
	} {
		if got := fmt.Sprint(after.Values[field]); got != want {
			t.Errorf("after a marker drag, %s = %q, want %q -- the generic PUT erased it", field, got, want)
		}
	}
	// And the drag itself took effect.
	if got := fmt.Sprint(after.Values[action.FieldStepSignatureX]); got != "20" {
		t.Errorf("fld_signature_x = %q, want the dragged position", got)
	}
}
