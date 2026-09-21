package composition

import (
	"testing"

	"menata.app/internal/domain"
)

func matrixWorkspace() (domain.Application, map[string]*domain.Machine) {
	app := domain.Application{
		ID:       "app_document_approval",
		Name:     "Document Approval",
		Machines: []string{"mch_document", "mch_approval_step"},
		Roles:    []string{"approver", "submitter", "reviewer"},
	}
	machines := map[string]*domain.Machine{
		// Declared first in the Application's own machines: list, so its rows come first.
		"mch_document": {
			ID: "mch_document", Name: "Document", ApplicationID: app.ID,
			Transitions: []domain.Transition{
				{ID: "trn_document_approved", Name: "All steps approved", Field: "fld_status", From: "in_review", To: "approved"},
			},
		},
		"mch_approval_step": {
			ID: "mch_approval_step", Name: "Approval Step", ApplicationID: app.ID,
			Permissions: []domain.Permission{{
				ID: "prm_decide_own_step", Action: domain.ActionDecide,
				Roles: []string{"approver", "reviewer"}, ActorField: "fld_assignee",
			}},
			Transitions: []domain.Transition{
				{ID: "trn_step_approve", Name: "Approve", Field: "fld_decision", From: "pending", To: "approved", Action: domain.ActionDecide},
			},
		},
	}
	return app, machines
}

func TestRoleMatrix(t *testing.T) {
	v := RoleMatrix(matrixWorkspace())

	if got, want := len(v.Rows), 2; got != want {
		t.Fatalf("rows = %d, want %d (one per declared transition, in the Application's own machine order)", got, want)
	}
	if v.Rows[0].Name != "All steps approved" || !v.Rows[0].System {
		t.Errorf("row 0 = %+v, want the Document's derived edge, marked System", v.Rows[0])
	}
	if len(v.Rows[0].Granted) != 0 {
		t.Error("a System row grants nothing to anyone -- it is performed by no action at all")
	}

	decide := v.Rows[1]
	if decide.Unrestricted {
		t.Error("the decide row is role-restricted; reporting it as unrestricted would draw six ticks for a rule that names two roles")
	}
	// Columns follow Application.Roles order: approver, submitter, reviewer.
	if want := []bool{true, false, true}; !sameGrants(decide.Granted, want) {
		t.Errorf("granted = %v, want %v -- roles within one permission are alternatives", decide.Granted, want)
	}
}

// The cell is the *intersection* across Permissions on one Action, because AllowsAction requires
// every one of them to pass. A union would tick a role the server refuses, which is the exact
// button-says-yes/server-says-no disagreement the matrix exists to make visible.
func TestRoleMatrix_severalPermissionsOnOneActionIntersect(t *testing.T) {
	app, machines := matrixWorkspace()
	step := machines["mch_approval_step"]
	step.Permissions = append(step.Permissions, domain.Permission{
		ID: "prm_decide_senior", Action: domain.ActionDecide, Roles: []string{"reviewer"},
	})

	granted := RoleMatrix(app, machines).Rows[1].Granted
	if want := []bool{false, false, true}; !sameGrants(granted, want) {
		t.Errorf("granted = %v, want %v -- only a role satisfying BOTH permissions reaches the action", granted, want)
	}
}

// An Action whose Permissions name no role restricts nobody by role (Principle #6). It is
// reported as Unrestricted rather than as six ticks, because "no rule was written" and "every
// role was listed" look identical in the table and only one of them changes when the Application
// declares a new role.
func TestRoleMatrix_noRoleBearingPermissionIsUnrestricted(t *testing.T) {
	app, machines := matrixWorkspace()
	machines["mch_approval_step"].Permissions[0].Roles = nil

	row := RoleMatrix(app, machines).Rows[1]
	if !row.Unrestricted {
		t.Fatal("an action no permission restricts by role must be reported as unrestricted")
	}
	if want := []bool{true, true, true}; !sameGrants(row.Granted, want) {
		t.Errorf("granted = %v, want %v", row.Granted, want)
	}
}

func sameGrants(got, want []bool) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// Two Permissions naming disjoint role sets intersect to nothing, and "nothing" here means
// *nobody may* -- the exact opposite of no Permission naming a role at all, which means anybody
// may. Inferring one from the other would draw a full row of ticks for an action the server
// refuses for everyone.
func TestRoleMatrix_disjointPermissionsGrantNobody(t *testing.T) {
	app, machines := matrixWorkspace()
	step := machines["mch_approval_step"]
	step.Permissions = append(step.Permissions, domain.Permission{
		ID: "prm_decide_submitter", Action: domain.ActionDecide, Roles: []string{"submitter"},
	})

	row := RoleMatrix(app, machines).Rows[1]
	if row.Unrestricted {
		t.Fatal("two permissions with no role in common restrict everyone, they do not restrict nobody")
	}
	if want := []bool{false, false, false}; !sameGrants(row.Granted, want) {
		t.Errorf("granted = %v, want %v", row.Granted, want)
	}
}
