package metadata

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func parentActorMachines() (child, parent *domain.Machine) {
	parent = &domain.Machine{ID: "mch_parent", Name: "Parent", Fields: []domain.Field{
		{ID: "fld_owner", Name: "Owner", Type: domain.FieldTypePerson, RelatedMachine: domain.UserMachineID},
		{ID: "fld_note", Name: "Note", Type: domain.FieldTypeText},
	}}
	child = &domain.Machine{ID: "mch_child", Name: "Child", Fields: []domain.Field{
		{ID: "fld_parent", Name: "Parent", Type: domain.FieldTypeRelation, RelatedMachine: "mch_parent"},
		{ID: "fld_label", Name: "Label", Type: domain.FieldTypeText},
	}, Permissions: []domain.Permission{{
		ID: "prm_create_child", Action: domain.ActionCreate,
		ParentActor: &domain.ParentActorGate{ViaField: "fld_parent", ActorField: "fld_owner"},
	}}}
	return child, parent
}

func TestParentActor_validDeclarationLoads(t *testing.T) {
	child, parent := parentActorMachines()
	if err := Validate(child); err != nil {
		t.Fatalf("Validate(child): %v", err)
	}
	if err := validateParentActors([]*domain.Machine{child, parent}); err != nil {
		t.Fatalf("validateParentActors: %v", err)
	}
}

func TestParentActor_refusesEveryMisdeclaration(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(child, parent *domain.Machine)
		want   string
		stage  string // "machine" or "workspace"
	}{
		"via is not a field":      {func(c, _ *domain.Machine) { c.Permissions[0].ParentActor.ViaField = "fld_nope" }, "is not a field of machine", "machine"},
		"via is not a relation":   {func(c, _ *domain.Machine) { c.Permissions[0].ParentActor.ViaField = "fld_label" }, "must be a relation field", "machine"},
		"half declared":           {func(c, _ *domain.Machine) { c.Permissions[0].ParentActor.ActorField = "" }, "needs both via and field", "machine"},
		"field missing on parent": {func(c, _ *domain.Machine) { c.Permissions[0].ParentActor.ActorField = "fld_nope" }, "is not a field of", "workspace"},
		"field is not a person":   {func(c, _ *domain.Machine) { c.Permissions[0].ParentActor.ActorField = "fld_note" }, "must be a person field", "workspace"},
	} {
		t.Run(name, func(t *testing.T) {
			child, parent := parentActorMachines()
			tc.mutate(child, parent)
			var err error
			if tc.stage == "machine" {
				err = Validate(child)
			} else {
				err = validateParentActors([]*domain.Machine{child, parent})
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

// A Permission whose only arm is the parent arm is a real guard, not "a permission that gates on nothing".
func TestParentActor_countsAsAnArm(t *testing.T) {
	child, _ := parentActorMachines()
	if err := Validate(child); err != nil && strings.Contains(err.Error(), "gates on nothing") {
		t.Errorf("a parent_actor-only Permission was reported as gating nothing: %v", err)
	}
}
