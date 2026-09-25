package composition

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// matrixWorkspace mirrors the real manifest's shape closely enough to exercise both sections: one
// Application claiming two Machines and one declaring no roles at all, plus two Machines no
// Application claims -- one append-only, one gated on the Workspace role.
func matrixWorkspace() ([]domain.Application, []*domain.Machine) {
	apps := []domain.Application{
		{
			ID:       "app_document_approval",
			Name:     "Document Approval",
			Machines: []string{"mch_document", "mch_approval_step"},
			Roles:    []string{"approver", "submitter", "reviewer"},
		},
		{ID: "app_project_management", Name: "Project Management", Machines: []string{"mch_task"}},
	}
	all := []*domain.Machine{
		{
			ID: "mch_document", Name: "Document", ApplicationID: "app_document_approval",
			Fields: []domain.Field{{ID: "fld_submitted_by", Name: "Submitted By", Type: domain.FieldTypePerson}},
			Permissions: []domain.Permission{
				{ID: "prm_create_own_document", Action: domain.ActionCreate, ActorField: "fld_submitted_by"},
			},
			Transitions: []domain.Transition{
				{ID: "trn_document_approved", Name: "All steps approved", Field: "fld_status", From: "in_review", To: "approved"},
				{ID: "trn_document_rejected", Name: "A step rejected", Field: "fld_status", From: "in_review", To: "rejected"},
			},
		},
		{
			ID: "mch_approval_step", Name: "Approval Step", ApplicationID: "app_document_approval",
			Fields: []domain.Field{{ID: "fld_assignee", Name: "Assignee", Type: domain.FieldTypePerson}},
			Permissions: []domain.Permission{{
				ID: "prm_decide_own_step", Action: domain.ActionDecide,
				Roles: []string{"approver", "reviewer"}, ActorField: "fld_assignee",
				DynamicActor: &domain.DynamicActorGate{
					ActorTypeField: "fld_approver_type", ActorUserField: "fld_assignee", ActorGroupField: "fld_approver_group",
				},
			}},
			Transitions: []domain.Transition{
				{ID: "trn_step_approve", Name: "Approve", Field: "fld_decision", From: "pending", To: "approved", Action: domain.ActionDecide},
			},
		},
		{ID: "mch_task", Name: "Task", ApplicationID: "app_project_management"},
		{ID: "mch_activity", Name: "Activity", AppendOnly: true},
		{
			ID: "mch_user", Name: "User",
			Permissions: []domain.Permission{
				{ID: "prm_edit_user_is_admin", Action: domain.ActionEdit, WorkspaceRole: domain.WorkspaceRoleAdmin},
				{ID: "prm_delete_user_is_admin", Action: domain.ActionDelete, WorkspaceRole: domain.WorkspaceRoleAdmin},
			},
		},
	}
	return apps, all
}

func rowsOf(app rendering.RoleMatrixApp) map[string]rendering.RoleMatrixRow {
	out := map[string]rendering.RoleMatrixRow{}
	for _, g := range app.Groups {
		for _, r := range g.Rows {
			out[g.Label+" · "+r.Action] = r
		}
	}
	return out
}

func TestRoleMatrix_applicationBlock(t *testing.T) {
	v := RoleMatrix(matrixWorkspace())

	if len(v.Applications) != 2 {
		t.Fatalf("applications = %d, want 2 -- every Application renders, including one with no roles", len(v.Applications))
	}
	da := v.Applications[0]
	rows := rowsOf(da)

	approve, ok := rows["Approval Step · Approve"]
	if !ok {
		t.Fatalf("no Approve row; got %v", keys(rows))
	}
	if approve.Who != "Approver, Reviewer" {
		t.Errorf("Approve who = %q, want %q -- roles within one permission are alternatives, in app.Roles' own declared order", approve.Who, "Approver, Reviewer")
	}
	// Who answers the role question only. The record-scoped arm has to be said, or the row
	// overstates the grant: "an approver may approve" is true, "any approver may approve any
	// step" is not. It must not repeat the role half Who already carries.
	if !strings.Contains(approve.Qualifier, "assigned to") || strings.Contains(approve.Qualifier, "roles ticked here") {
		t.Errorf("Approve qualifier = %q, want it to name only the record-scoped arm in plain words", approve.Qualifier)
	}

	create := rows["Document · Create"]
	if !strings.Contains(create.Qualifier, "own name") || create.Open {
		t.Errorf("Create row = %+v, want a governed row reading as a rule about the record you may write", create)
	}

	// edit and delete share one rule (none), so they fold into a single line -- and that line has
	// to be marked open, because an ungoverned action and a fully granted one are otherwise
	// indistinguishable.
	editDelete, ok := rows["Document · Edit or delete"]
	if !ok {
		t.Fatalf("edit and delete did not fold into one row; got %v", keys(rows))
	}
	if !editDelete.Open || !strings.Contains(editDelete.Qualifier, "No restriction set yet") {
		t.Errorf("edit/delete row = %+v, want it flagged as governed by nothing", editDelete)
	}
}

// A move no Action performs becomes one sentence per Machine, never a row: as rows they
// outnumbered everything else on the page, and no role can ever be granted one.
func TestRoleMatrix_automaticMovesAreProseNotRows(t *testing.T) {
	v := RoleMatrix(matrixWorkspace())
	da := v.Applications[0]

	for label := range rowsOf(da) {
		if strings.Contains(label, "All steps approved") || strings.Contains(label, "A step rejected") {
			t.Errorf("%q is a system-derived move and must not be a row", label)
		}
	}
	if len(da.Automatic) != 1 {
		t.Fatalf("automatic = %v, want one line for the one Machine with derived moves", da.Automatic)
	}
	for _, want := range []string{"approved or rejected", "not even an admin"} {
		if !strings.Contains(da.Automatic[0], want) {
			t.Errorf("automatic line %q missing %q", da.Automatic[0], want)
		}
	}
}

// An Application declaring no roles renders as itself, with no rows -- not omitted, and not an
// empty grid suggesting roles are merely unassigned there.
func TestRoleMatrix_applicationWithNoRoles(t *testing.T) {
	pm := RoleMatrix(matrixWorkspace()).Applications[1]

	if pm.Name != "Project Management" {
		t.Fatalf("applications[1] = %q, want the role-less Application to still be on the page", pm.Name)
	}
	if len(pm.Roles) != 0 || len(pm.Groups) != 0 {
		t.Errorf("%+v: an Application declaring no roles has no columns and no rows to draw", pm)
	}
}

func TestRoleMatrix_workspaceSection(t *testing.T) {
	v := RoleMatrix(matrixWorkspace())

	if len(v.Workspace) != 3 {
		t.Fatalf("workspace rules = %d, want 3 (administration, activity, user); got %+v", len(v.Workspace), v.Workspace)
	}
	// The administration row is not a projection of metadata and says so, because route-level
	// gating is not something metadata can declare here.
	if !v.Workspace[0].Undeclared {
		t.Error("the workspace-administration row must be marked as enforced in code, not declared")
	}
	if !strings.Contains(v.Workspace[1].Who, "No one") {
		t.Errorf("row 1 = %+v, want the append-only Activity rule", v.Workspace[1])
	}
	if !strings.Contains(v.Workspace[2].Rule, "Edit or delete") || !strings.Contains(v.Workspace[2].Who, "admin") {
		t.Errorf("row 2 = %+v, want User's edit+delete grouped under one Workspace-admin rule", v.Workspace[2])
	}
	// A Machine an Application claims is never repeated here: its rules already appear in that
	// Application's own block, read against its own roles.
	for _, n := range v.Workspace {
		if strings.Contains(n.Rule, "document") || strings.Contains(n.Rule, "approval step") {
			t.Errorf("%q belongs to an application block, not the workspace section", n.Rule)
		}
	}
}

// The cell is the *intersection* across Permissions on one Action, because AllowsAction requires
// every one of them to pass. A union would tick a role the server refuses.
func TestRoleMatrix_severalPermissionsOnOneActionIntersect(t *testing.T) {
	apps, all := matrixWorkspace()
	all[1].Permissions = append(all[1].Permissions, domain.Permission{
		ID: "prm_decide_senior", Action: domain.ActionDecide, Roles: []string{"reviewer"},
	})

	got := rowsOf(RoleMatrix(apps, all).Applications[0])["Approval Step · Approve"]
	if got.Who != "Reviewer" {
		t.Errorf("who = %q, want %q -- only a role satisfying BOTH permissions reaches the action", got.Who, "Reviewer")
	}
}

// Two Permissions naming disjoint role sets intersect to nothing, and "nothing" here means
// *nobody may* -- the exact opposite of no Permission naming a role at all, which means anybody
// may. Inferring one from the other would draw a full row of ticks for an action the server
// refuses for everyone.
func TestRoleMatrix_disjointPermissionsGrantNobody(t *testing.T) {
	apps, all := matrixWorkspace()
	all[1].Permissions = append(all[1].Permissions, domain.Permission{
		ID: "prm_decide_submitter", Action: domain.ActionDecide, Roles: []string{"submitter"},
	})

	got := rowsOf(RoleMatrix(apps, all).Applications[0])["Approval Step · Approve"]
	if got.Open {
		t.Fatal("two permissions with no role in common restrict everyone, they do not restrict nobody")
	}
	if got.Who != "No one" {
		t.Errorf("who = %q, want %q", got.Who, "No one")
	}
}

// foldIdentical may never hide a difference: two actions merge only when every rendered value
// matches, so giving one of them a rule splits the row again.
func TestRoleMatrix_foldSplitsWhenARuleDiffers(t *testing.T) {
	apps, all := matrixWorkspace()
	all[0].Permissions = append(all[0].Permissions, domain.Permission{
		ID: "prm_delete_own_document", Action: domain.ActionDelete, ActorField: "fld_submitted_by",
	})

	if _, folded := rowsOf(RoleMatrix(apps, all).Applications[0])["Document · Edit or delete"]; folded {
		t.Error("edit and delete folded together while only delete is governed")
	}
}

// An append-only Machine refuses edit and delete for everyone, which is the opposite of an
// undeclared action -- so the row is empty cells plus a sentence, never a row of ticks.
func TestRoleMatrix_appendOnlyMachineInsideAnApplication(t *testing.T) {
	apps, all := matrixWorkspace()
	all[3].ApplicationID = "app_document_approval"
	apps[0].Machines = append(apps[0].Machines, "mch_activity")

	got := rowsOf(RoleMatrix(apps, all).Applications[0])["Activity · Edit or delete"]
	if got.Who != "No one" {
		t.Errorf("who = %q, want %q -- append-only refuses everyone", got.Who, "No one")
	}
	if got.Open {
		t.Error("append-only is a rule, not the absence of one")
	}
}

func keys(m map[string]rendering.RoleMatrixRow) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// The Access row is what makes a view-only role legible. Without it a role granted nothing else
// renders as an empty column, which reads as "this role can do nothing" -- the opposite of what a
// reviewer is for.
func TestRoleMatrix_accessRowComesFirstAndEveryRoleHasIt(t *testing.T) {
	v := RoleMatrix(matrixWorkspace())
	da := v.Applications[0]

	if len(da.Groups) == 0 || da.Groups[0].Label != "Access" {
		t.Fatalf("groups = %v, want Access first", da.Groups)
	}
	row := da.Groups[0].Rows[0]
	if row.Who != "All roles" {
		t.Errorf("who = %q, want %q -- holding any role is what grants entry", row.Who, "All roles")
	}
	if !strings.Contains(row.Qualifier, "no role") {
		t.Errorf("qualifier = %q, want it to state what someone without a role sees", row.Qualifier)
	}

	// An Application declaring no roles is gated on nothing, so it has no Access row to draw
	// either -- and says so in a sentence instead.
	if pm := v.Applications[1]; len(pm.Groups) != 0 {
		t.Errorf("%s declares no roles, so it has no access rule to render", pm.Name)
	}
	if da.RolesSummary != "Approver, Submitter, Reviewer" {
		t.Errorf("rolesSummary = %q, want %q", da.RolesSummary, "Approver, Submitter, Reviewer")
	}
}

// whoText's two edge cases named in requiredRoles' own doc comment: no role-bearing Permission at
// all reads "All roles", and two Permissions leaving nothing in common reads "No one" -- neither
// is "some roles", so both need their own test rather than being inferred from the ordinary case.
func TestWhoText(t *testing.T) {
	roles := []string{"approver", "submitter", "reviewer"}

	if got := whoText(roles, nil, false); got != "All roles" {
		t.Errorf("unrestricted = %q, want %q", got, "All roles")
	}
	if got := whoText(roles, map[string]bool{"reviewer": true}, true); got != "Reviewer" {
		t.Errorf("one match = %q, want %q", got, "Reviewer")
	}
	if got := whoText(roles, map[string]bool{"reviewer": true, "approver": true}, true); got != "Approver, Reviewer" {
		t.Errorf("two matches = %q, want them in roles' own declared order, got %q", got, got)
	}
	if got := whoText(roles, map[string]bool{}, true); got != "No one" {
		t.Errorf("disjoint permissions = %q, want %q", got, "No one")
	}
}
