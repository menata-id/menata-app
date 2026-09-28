package domain

import "testing"

// The precedence in MachineInWorkflowRole is the whole content of that method, and each of its three
// branches exists for a route that really behaves that way:
//
//   - a screen inside an Application (the /machines/{id}/... surface, the approval inbox) scopes the
//     question with that Application;
//   - the submit wizard (/documents/new, POST /documents) is named by no navigation item, so nothing
//     resolves an Application for it, and the Workspace's sole binding is the only right answer;
//   - two approval Applications and no Application on ctx is genuinely ambiguous, and guessing would
//     put one Application's records on the other's screen.
func TestMachineInWorkflowRole_precedence(t *testing.T) {
	surat := &Machine{ID: "mch_surat", ApplicationID: "app_a", WorkflowEngine: WorkflowEngineDocumentApproval, WorkflowRole: WorkflowRoleDocument}
	letter := &Machine{ID: "mch_letter", ApplicationID: "app_b", WorkflowEngine: WorkflowEngineDocumentApproval, WorkflowRole: WorkflowRoleDocument}
	plain := &Machine{ID: "mch_note"}

	appA := Application{ID: "app_a", Machines: []string{"mch_surat"}, Workflow: &Workflow{
		Engine: WorkflowEngineDocumentApproval, Roles: map[string]string{WorkflowRoleDocument: "mch_surat"},
	}}
	appB := Application{ID: "app_b", Machines: []string{"mch_letter"}, Workflow: &Workflow{
		Engine: WorkflowEngineDocumentApproval, Roles: map[string]string{WorkflowRoleDocument: "mch_letter"},
	}}
	plainApp := Application{ID: "app_plain", Machines: []string{"mch_note"}}

	one := Workspace{Machines: []*Machine{surat, plain}, Applications: []Application{appA, plainApp}}
	two := Workspace{Machines: []*Machine{surat, letter}, Applications: []Application{appA, appB}}
	none := Workspace{Machines: []*Machine{plain}, Applications: []Application{plainApp}}

	tests := []struct {
		name          string
		ws            Workspace
		applicationID string
		want          *Machine
	}{
		{"the Application on ctx answers", two, "app_b", letter},
		{"the other one answers for itself", two, "app_a", surat},
		{"no Application on ctx, one binding: the sole answer", one, "", surat},
		{"no Application on ctx, two bindings: ambiguous", two, "", nil},
		{"no binding anywhere", none, "", nil},
		// An Application that binds no engine is not a *scope* for this question, so it falls through
		// rather than answering nil. /dashboard depends on exactly this: it is declared by Project
		// Management, and its Document status tiles must still see the approval Application installed
		// beside it. Same for an Application id that names nothing installed here.
		{"an Application binding no engine falls through", one, "app_plain", surat},
		{"an Application that is not installed here falls through", one, "app_ghost", surat},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.ws.MachineInWorkflowRole(WorkflowEngineDocumentApproval, WorkflowRoleDocument, tt.applicationID)
			if got != tt.want {
				t.Errorf("MachineInWorkflowRole(%q) = %v, want %v", tt.applicationID, name(got), name(tt.want))
			}
		})
	}

	// A role the engine casts nobody in is nil even with the Application named -- an optional role's
	// whole point, and what every caller of one has to handle.
	if got := one.MachineInWorkflowRole(WorkflowEngineDocumentApproval, WorkflowRoleSignature, "app_a"); got != nil {
		t.Errorf("an uncast optional role resolved to %v, want nil", name(got))
	}
	// And another engine's role name never borrows this engine's cast.
	if got := one.MachineInWorkflowRole("some_other_engine", WorkflowRoleDocument, "app_a"); got != nil {
		t.Errorf("another engine's role resolved to %v, want nil", name(got))
	}
}

// MachinesInWorkflowRole is what the Workspace-level chrome sums over, so the case that matters is
// "several" -- the one the badge and Workspace Home silently got wrong while they read one id.
func TestMachinesInWorkflowRole_returnsEveryBinding(t *testing.T) {
	a := &Machine{ID: "mch_a", WorkflowEngine: WorkflowEngineDocumentApproval, WorkflowRole: WorkflowRoleStep}
	b := &Machine{ID: "mch_b", WorkflowEngine: WorkflowEngineDocumentApproval, WorkflowRole: WorkflowRoleStep}
	doc := &Machine{ID: "mch_c", WorkflowEngine: WorkflowEngineDocumentApproval, WorkflowRole: WorkflowRoleDocument}
	ws := Workspace{Machines: []*Machine{a, doc, b}}

	got := ws.MachinesInWorkflowRole(WorkflowEngineDocumentApproval, WorkflowRoleStep)
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Errorf("MachinesInWorkflowRole(step) = %v, want both step Machines in the Workspace's own order", names(got))
	}
	if got := (Workspace{}).MachinesInWorkflowRole(WorkflowEngineDocumentApproval, WorkflowRoleStep); len(got) != 0 {
		t.Errorf("an empty Workspace returned %v, want none -- zero is a real answer, not a missing one", names(got))
	}
}

func name(m *Machine) string {
	if m == nil {
		return "nil"
	}
	return m.ID
}

func names(ms []*Machine) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.ID)
	}
	return out
}
