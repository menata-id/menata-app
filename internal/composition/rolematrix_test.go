package composition

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// matrixWorkspace mirrors the real manifest's shape closely enough to exercise every section: an
// Application claiming two Machines, and two Machines it does not claim -- one append-only, one
// gated on the Workspace role.
func matrixWorkspace() (domain.Application, []*domain.Machine) {
	app := domain.Application{
		ID:       "app_document_approval",
		Name:     "Document Approval",
		Machines: []string{"mch_document", "mch_approval_step"},
		Roles:    []string{"approver", "submitter", "reviewer"},
	}
	all := []*domain.Machine{
		{
			ID: "mch_document", Name: "Document", ApplicationID: app.ID,
			Fields: []domain.Field{{ID: "fld_submitted_by", Name: "Submitted By", Type: domain.FieldTypePerson}},
			Permissions: []domain.Permission{
				{ID: "prm_create_own_document", Action: domain.ActionCreate, ActorField: "fld_submitted_by"},
			},
			Transitions: []domain.Transition{
				{ID: "trn_document_approved", Name: "All steps approved", Field: "fld_status", From: "in_review", To: "approved"},
			},
		},
		{
			ID: "mch_approval_step", Name: "Approval Step", ApplicationID: app.ID,
			Fields: []domain.Field{{ID: "fld_assignee", Name: "Assignee", Type: domain.FieldTypePerson}},
			Permissions: []domain.Permission{{
				ID: "prm_decide_own_step", Action: domain.ActionDecide,
				Roles: []string{"approver", "reviewer"}, ActorField: "fld_assignee",
			}},
			Transitions: []domain.Transition{
				{ID: "trn_step_approve", Name: "Approve", Field: "fld_decision", From: "pending", To: "approved", Action: domain.ActionDecide},
			},
		},
		{ID: "mch_activity", Name: "Activity", AppendOnly: true},
		{
			ID: "mch_user", Name: "User",
			Permissions: []domain.Permission{
				{ID: "prm_edit_user_is_admin", Action: domain.ActionEdit, WorkspaceRole: domain.WorkspaceRoleAdmin},
				{ID: "prm_delete_user_is_admin", Action: domain.ActionDelete, WorkspaceRole: domain.WorkspaceRoleAdmin},
			},
		},
	}
	return app, all
}

func TestRoleMatrix_transitions(t *testing.T) {
	v := RoleMatrix(matrixWorkspace())

	if got, want := len(v.Transitions), 2; got != want {
		t.Fatalf("transitions = %d, want %d (one per declared edge, in the Application's own machine order)", got, want)
	}
	if v.Transitions[0].Name != "All steps approved" || !v.Transitions[0].System {
		t.Errorf("row 0 = %+v, want the Document's derived edge, marked System", v.Transitions[0])
	}
	if len(v.Transitions[0].Granted) != 0 {
		t.Error("a System row grants nothing to anyone -- it is performed by no action at all")
	}

	decide := v.Transitions[1]
	if decide.Unrestricted {
		t.Error("the decide row is role-restricted; reporting it as unrestricted would draw three ticks for a rule that names two roles")
	}
	if want := []bool{true, false, true}; !sameGrants(decide.Granted, want) {
		t.Errorf("granted = %v, want %v -- roles within one permission are alternatives", decide.Granted, want)
	}
	// A tick answers the role question only. The record-scoped arm has to be said, or the row
	// overstates the grant: "an approver may approve" is true, "any approver may approve any
	// step" is not.
	if !strings.Contains(decide.Note, "Assignee") {
		t.Errorf("note = %q, want it to name the record-scoped arm", decide.Note)
	}
}

// The record-actions section is the half board 06 never drew, and the reason it matters is that
// an ungoverned action and a fully granted one look identical in the cells.
func TestRoleMatrix_recordActions(t *testing.T) {
	v := RoleMatrix(matrixWorkspace())

	byName := map[string]string{}
	for _, row := range v.RecordActions {
		byName[row.MachineName+" · "+row.Detail] = row.Note
	}

	if note, ok := byName["Document · create"]; !ok {
		t.Fatalf("no Document create row; got %v", byName)
	} else if !strings.Contains(note, "own name") {
		t.Errorf("create note = %q, want it to read as a rule about the record you may write", note)
	}
	// edit and delete on Document are governed by nothing, and the row has to say so.
	if note, ok := byName["Document · edit, delete"]; !ok {
		t.Errorf("edit and delete share one rule (none), so they fold into one row; got %v", byName)
	} else if !strings.Contains(note, "No rule is declared") {
		t.Errorf("ungoverned note = %q, want it to say no rule is declared", note)
	}
}

// foldIdentical may never hide a difference: two actions merge only when every rendered value
// matches, so giving one of them a rule splits the row again.
func TestRoleMatrix_foldSplitsWhenARuleDiffers(t *testing.T) {
	app, all := matrixWorkspace()
	all[0].Permissions = append(all[0].Permissions, domain.Permission{
		ID: "prm_delete_own_document", Action: domain.ActionDelete, ActorField: "fld_submitted_by",
	})

	var details []string
	for _, row := range RoleMatrix(app, all).RecordActions {
		if row.MachineName == "Document" {
			details = append(details, row.Detail)
		}
	}
	for _, d := range details {
		if d == "edit, delete" {
			t.Fatalf("edit and delete folded together while only delete is governed; rows = %v", details)
		}
	}
}

// An append-only Machine refuses edit and delete for everyone, which is the opposite of an
// undeclared action -- so the row is empty cells plus a sentence, never a row of ticks.
func TestRoleMatrix_workspaceRules(t *testing.T) {
	v := RoleMatrix(matrixWorkspace())

	// Two rows, not three: User's edit and delete are governed by the same Workspace role, so
	// they group into one line rather than making a reader compare two identical sentences.
	if len(v.WorkspaceRules) != 2 {
		t.Fatalf("workspace rules = %d, want 2 (activity append-only, user edit+delete); got %+v", len(v.WorkspaceRules), v.WorkspaceRules)
	}
	if v.WorkspaceRules[0].MachineName != "Activity" || !strings.Contains(v.WorkspaceRules[0].Who, "No one") {
		t.Errorf("row 0 = %+v, want the append-only Activity rule", v.WorkspaceRules[0])
	}
	if u := v.WorkspaceRules[1]; u.MachineName != "User" || !strings.Contains(u.Who, "admin") || u.Detail != "edit, delete" {
		t.Errorf("row 1 = %+v, want User's Workspace-admin rule covering edit and delete", u)
	}
	// A Machine an Application claims is never repeated here: its rules already appear in the
	// sections above, read against that Application's own roles.
	for _, n := range v.WorkspaceRules {
		if n.MachineName == "Document" || n.MachineName == "Approval Step" {
			t.Errorf("%s is claimed by an application and must not also appear as a workspace rule", n.MachineName)
		}
	}
}

// The cell is the *intersection* across Permissions on one Action, because AllowsAction requires
// every one of them to pass. A union would tick a role the server refuses.
func TestRoleMatrix_severalPermissionsOnOneActionIntersect(t *testing.T) {
	app, all := matrixWorkspace()
	step := all[1]
	step.Permissions = append(step.Permissions, domain.Permission{
		ID: "prm_decide_senior", Action: domain.ActionDecide, Roles: []string{"reviewer"},
	})

	granted := RoleMatrix(app, all).Transitions[1].Granted
	if want := []bool{false, false, true}; !sameGrants(granted, want) {
		t.Errorf("granted = %v, want %v -- only a role satisfying BOTH permissions reaches the action", granted, want)
	}
}

// Two Permissions naming disjoint role sets intersect to nothing, and "nothing" here means
// *nobody may* -- the exact opposite of no Permission naming a role at all, which means anybody
// may. Inferring one from the other would draw a full row of ticks for an action the server
// refuses for everyone.
func TestRoleMatrix_disjointPermissionsGrantNobody(t *testing.T) {
	app, all := matrixWorkspace()
	step := all[1]
	step.Permissions = append(step.Permissions, domain.Permission{
		ID: "prm_decide_submitter", Action: domain.ActionDecide, Roles: []string{"submitter"},
	})

	row := RoleMatrix(app, all).Transitions[1]
	if row.Unrestricted {
		t.Fatal("two permissions with no role in common restrict everyone, they do not restrict nobody")
	}
	if want := []bool{false, false, false}; !sameGrants(row.Granted, want) {
		t.Errorf("granted = %v, want %v", row.Granted, want)
	}
}

// An action no Permission restricts by role is reported as Unrestricted rather than as three
// ticks, because "no rule was written" and "every role was listed" look identical in the table
// and only one of them changes when the Application declares a new role.
func TestRoleMatrix_noRoleBearingPermissionIsUnrestricted(t *testing.T) {
	app, all := matrixWorkspace()
	all[1].Permissions[0].Roles = nil

	row := RoleMatrix(app, all).Transitions[1]
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
