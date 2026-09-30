package aiassist

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func validLeaveRequestChange() GeneratedChange {
	return GeneratedChange{
		Kind: KindNewApplication,
		Application: &GeneratedApplication{
			ID:            "app_leave_requests",
			Name:          "Leave Requests",
			Roles:         []string{"Employee", "Supervisor", "HR admin"},
			PublisherRole: "HR admin",
			Navigation:    []GeneratedMenuItem{{Label: "Leave Requests", MachineID: "mch_leave_request"}},
			Machines: []GeneratedMachine{
				{
					ID:   "mch_leave_request",
					Name: "Leave Request",
					Fields: []GeneratedField{
						{ID: "fld_title", Name: "Title", Type: "text", Required: true},
						{ID: "fld_status", Name: "Status", Type: "status", Required: true, Options: []string{"submitted", "approved", "rejected"}},
					},
					Permissions: []GeneratedPermission{
						{ID: "prm_create", Action: "create", Roles: []string{"Employee"}},
						{ID: "prm_edit", Action: "edit", Roles: []string{"Supervisor", "HR admin"}},
					},
					Transitions: []GeneratedTransition{
						{ID: "trn_approve", Name: "Approve", Field: "fld_status", From: "submitted", To: "approved"},
					},
					Events: []GeneratedEvent{
						{ID: "evt_submitted", OnCreate: true, Summary: "\"{fld_title}\" submitted"},
					},
				},
			},
		},
	}
}

func TestValidate_newApplication_valid(t *testing.T) {
	if err := Validate(validLeaveRequestChange(), ExistingState{}); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestValidate_newApplication_badID(t *testing.T) {
	change := validLeaveRequestChange()
	change.Application.ID = "LeaveRequests"
	if err := Validate(change, ExistingState{}); err == nil {
		t.Fatal("Validate() = nil, want an error for a malformed application id")
	}
}

func TestValidate_newApplication_collidesWithExisting(t *testing.T) {
	change := validLeaveRequestChange()
	existing := ExistingState{
		MachineIDs:     map[string]bool{"mch_leave_request": true},
		ApplicationIDs: map[string]bool{},
	}
	err := Validate(change, existing)
	if err == nil {
		t.Fatal("Validate() = nil, want an error for a machine id already in the workspace")
	}
	if !strings.Contains(err.Error(), "mch_leave_request") {
		t.Errorf("error %v does not name the colliding machine id", err)
	}
}

func TestValidate_newApplication_unknownFieldType(t *testing.T) {
	change := validLeaveRequestChange()
	change.Application.Machines[0].Fields[0].Type = "voting"
	if err := Validate(change, ExistingState{}); err == nil {
		t.Fatal("Validate() = nil, want an error for an unknown field type")
	}
}

func TestValidate_newApplication_hardcodedActionRefused(t *testing.T) {
	change := validLeaveRequestChange()
	change.Application.Machines[0].Permissions = append(change.Application.Machines[0].Permissions,
		GeneratedPermission{ID: "prm_decide", Action: domain.ActionDecide, Roles: []string{"Supervisor"}})
	err := Validate(change, ExistingState{})
	if err == nil {
		t.Fatal("Validate() = nil, want an error for a permission naming the hardcoded decide action")
	}
	if !strings.Contains(err.Error(), "decide") {
		t.Errorf("error %v does not explain the decide-action refusal", err)
	}
}

func TestValidate_newApplication_relationMustStayWithinChange(t *testing.T) {
	change := validLeaveRequestChange()
	change.Application.Machines[0].Fields = append(change.Application.Machines[0].Fields,
		GeneratedField{ID: "fld_approver", Name: "Approver", Type: "relation", RelatedMachine: "mch_user"})
	err := Validate(change, ExistingState{})
	if err == nil {
		t.Fatal("Validate() = nil, want an error for a relation targeting a machine outside this change")
	}
	if !strings.Contains(err.Error(), "mch_user") {
		t.Errorf("error %v does not name the out-of-change relation target", err)
	}
}

func TestValidate_extendApplication_newOption(t *testing.T) {
	existing := ExistingState{
		Applications: map[string]ExistingApplicationState{
			"app_document_approval": {
				Roles: []string{"approver", "submitter"},
				Machines: map[string]*domain.Machine{
					"mch_document": {
						ID: "mch_document",
						Fields: []domain.Field{
							{ID: "fld_document_type", Type: domain.FieldTypeStatus, Options: []string{"Kontrak", "Tagihan"}},
						},
					},
				},
			},
		},
	}
	change := GeneratedChange{
		Kind:        KindExtendApplication,
		TargetAppID: "app_document_approval",
		Additions: []MetadataAddition{
			{MachineID: "mch_document", FieldID: "fld_document_type", NewOption: "Nota Dinas"},
		},
	}
	if err := Validate(change, existing); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestValidate_extendApplication_optionAlreadyExists(t *testing.T) {
	existing := ExistingState{
		Applications: map[string]ExistingApplicationState{
			"app_document_approval": {
				Machines: map[string]*domain.Machine{
					"mch_document": {
						ID: "mch_document",
						Fields: []domain.Field{
							{ID: "fld_document_type", Type: domain.FieldTypeStatus, Options: []string{"Kontrak", "Tagihan"}},
						},
					},
				},
			},
		},
	}
	change := GeneratedChange{
		Kind:        KindExtendApplication,
		TargetAppID: "app_document_approval",
		Additions: []MetadataAddition{
			{MachineID: "mch_document", FieldID: "fld_document_type", NewOption: "Kontrak"},
		},
	}
	if err := Validate(change, existing); err == nil {
		t.Fatal("Validate() = nil, want an error for an option that's already declared")
	}
}

func TestValidate_extendApplication_unknownTarget(t *testing.T) {
	change := GeneratedChange{Kind: KindExtendApplication, TargetAppID: "app_ghost", Additions: []MetadataAddition{{NewRole: "X"}}}
	if err := Validate(change, ExistingState{}); err == nil {
		t.Fatal("Validate() = nil, want an error for an application not installed in this workspace")
	}
}

func TestValidate_unknownKind(t *testing.T) {
	if err := Validate(GeneratedChange{Kind: "delete_everything"}, ExistingState{}); err == nil {
		t.Fatal("Validate() = nil, want an error for an unrecognized kind")
	}
}

// TestValidate_newApplication_menuIsThePersonsAnswer: the menu is asked for, never derived. No menu
// is refused, a Machine left out of it is the person's choice and allowed, and an entry may only open
// one of this Application's own Machines, once.
func TestValidate_newApplication_menuIsThePersonsAnswer(t *testing.T) {
	twoMachines := func(menu ...GeneratedMenuItem) GeneratedChange {
		c := validLeaveRequestChange()
		c.Application.Machines = append(c.Application.Machines, GeneratedMachine{
			ID: "mch_leave_balance", Name: "Leave Balance",
			Fields: []GeneratedField{{ID: "fld_days", Name: "Days", Type: "number", Required: true}},
		})
		c.Application.Navigation = menu
		return c
	}
	cases := []struct {
		name    string
		change  GeneratedChange
		wantErr string
	}{
		{"no menu", twoMachines(), "navigation is required"},
		{"one machine left out", twoMachines(GeneratedMenuItem{Label: "Requests", MachineID: "mch_leave_request"}), ""},
		{"both, in the person's order", twoMachines(
			GeneratedMenuItem{Label: "Balance", MachineID: "mch_leave_balance"},
			GeneratedMenuItem{Label: "Requests", MachineID: "mch_leave_request"}), ""},
		{"foreign machine", twoMachines(GeneratedMenuItem{Label: "Users", MachineID: "mch_user"}), "does not declare"},
		{"twice", twoMachines(
			GeneratedMenuItem{Label: "A", MachineID: "mch_leave_request"},
			GeneratedMenuItem{Label: "B", MachineID: "mch_leave_request"}), "more than one navigation entry"},
		{"no label", twoMachines(GeneratedMenuItem{MachineID: "mch_leave_request"}), "has no label"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.change, ExistingState{})
			switch {
			case tc.wantErr == "" && err != nil:
				t.Errorf("Validate() = %v, want nil", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Errorf("Validate() = %v, want an issue containing %q", err, tc.wantErr)
			}
		})
	}
}

// TestSystemPrompt_asksForTheMenu: the rule that makes the assistant ask. Without it the schema's
// required "navigation" would be filled by the model's own guess.
func TestSystemPrompt_asksForTheMenu(t *testing.T) {
	prompt := SystemPromptFor(nil, nil)
	for _, want := range []string{`"navigation"`, "Never decide the menu on their behalf"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("system prompt does not contain %q", want)
		}
	}
}
