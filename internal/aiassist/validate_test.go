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
			ID:    "app_leave_requests",
			Name:  "Leave Requests",
			Roles: []string{"Employee", "Supervisor", "HR admin"},
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
