package action

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// Every fixture here uses ids this repo has never used -- mch_persetujuan, fld_putusan, fld_oleh --
// because the whole claim of Stage B is that an Action's effect is declared rather than named in Go. A
// fixture using fld_decision could not tell a declaration apart from the constant it replaced.
func decisionMachine() *domain.Machine {
	return &domain.Machine{
		ID:   "mch_persetujuan",
		Name: "Persetujuan",
		Fields: []domain.Field{
			{ID: "fld_putusan", Name: "Putusan", Type: domain.FieldTypeStatus, Options: []string{"menunggu", "setuju", "tolak"}},
			{ID: "fld_oleh", Name: "Oleh", Type: domain.FieldTypeText},
		},
		Transitions: []domain.Transition{
			{ID: "trn_setuju", Name: "Setuju", Field: "fld_putusan", From: "menunggu", To: "setuju", Action: domain.ActionDecide},
			{ID: "trn_tolak", Name: "Tolak", Field: "fld_putusan", From: "menunggu", To: "tolak", Action: domain.ActionDecide},
		},
		ActionEffects: []domain.ActionEffect{{
			Action: domain.ActionDecide,
			Writes: []domain.FieldWrite{{Field: "fld_oleh", From: domain.WriteFromActorName}},
		}},
	}
}

func TestApplyEffect_writesWhatTheMachineDeclares(t *testing.T) {
	values := map[string]any{}
	written, err := ApplyEffect(decisionMachine(), domain.ActionDecide, values,
		EffectInput{Submitted: "setuju", ActorID: "usr_rina", ActorName: "Rina Nur"})
	if err != nil {
		t.Fatalf("ApplyEffect: %v", err)
	}
	if got, want := values["fld_oleh"], "Rina Nur"; got != want {
		t.Errorf("fld_oleh = %v, want %q -- the actor's name at this moment, a snapshot", got, want)
	}
	if len(written) != 1 || written[0] != "fld_oleh" {
		t.Errorf("written = %v, want [fld_oleh]", written)
	}
	// Only what was declared. The status move is the Transition's business, not this function's.
	if _, set := values["fld_putusan"]; set {
		t.Error("ApplyEffect wrote the status Field -- that comes from the declared Transition (ApplyStatusMove)")
	}
}

func TestApplyEffect_allThreeSources(t *testing.T) {
	m := decisionMachine()
	m.ActionEffects = []domain.ActionEffect{{
		Action: domain.ActionDecide,
		Writes: []domain.FieldWrite{
			{Field: "fld_putusan", From: domain.WriteFromSubmitted},
			{Field: "fld_oleh", From: domain.WriteFromActor},
		},
	}}
	values := map[string]any{}
	if _, err := ApplyEffect(m, domain.ActionDecide, values, EffectInput{Submitted: "tolak", ActorID: "usr_budi", ActorName: "Budi"}); err != nil {
		t.Fatalf("ApplyEffect: %v", err)
	}
	if values["fld_putusan"] != "tolak" || values["fld_oleh"] != "usr_budi" {
		t.Errorf("values = %v, want the submitted value and the actor's id", values)
	}

	// A declared literal, which is how an Action moves a Field whose state model deliberately declares
	// no person-performed edge (mch_document's fld_status).
	m.ActionEffects = []domain.ActionEffect{{
		Action: domain.ActionRevise,
		Writes: []domain.FieldWrite{{Field: "fld_putusan", Value: "menunggu"}},
	}}
	values = map[string]any{}
	if _, err := ApplyEffect(m, domain.ActionRevise, values, EffectInput{}); err != nil {
		t.Fatalf("ApplyEffect(revise): %v", err)
	}
	if values["fld_putusan"] != "menunggu" {
		t.Errorf("fld_putusan = %v, want the declared literal", values["fld_putusan"])
	}
}

// A Machine declaring no effect for an Action is the normal case -- edit and delete write only what was
// submitted -- so this is silence, not an error.
func TestApplyEffect_noEffectDeclaredIsNotAnError(t *testing.T) {
	values := map[string]any{"fld_oleh": "untouched"}
	written, err := ApplyEffect(decisionMachine(), domain.ActionEdit, values, EffectInput{ActorName: "Rina"})
	if err != nil || len(written) != 0 {
		t.Errorf("ApplyEffect(edit) = (%v, %v), want (no fields, no error)", written, err)
	}
	if values["fld_oleh"] != "untouched" {
		t.Error("ApplyEffect wrote something for an Action the Machine declares no effect for")
	}
}

// Unreachable through the loader, which refuses an unknown source -- and still an error here, because
// the alternative is storing "" and looking like it worked.
func TestApplyEffect_refusesASourceItCannotResolve(t *testing.T) {
	m := decisionMachine()
	m.ActionEffects = []domain.ActionEffect{{
		Action: domain.ActionDecide,
		Writes: []domain.FieldWrite{{Field: "fld_oleh", From: "tomorrow"}},
	}}
	if _, err := ApplyEffect(m, domain.ActionDecide, map[string]any{}, EffectInput{}); err == nil {
		t.Fatal("ApplyEffect() error = nil, want one for an unresolvable source")
	}
}

// TestApplyStatusMove_derivesFieldAndTargetsFromTheTransitions is the other half of Stage B, and the
// one that removes a hardcoded pair of values: what a decide may move, and to what, is read off the
// declared edges.
func TestApplyStatusMove_derivesFieldAndTargetsFromTheTransitions(t *testing.T) {
	m := decisionMachine()

	values := map[string]any{}
	field, err := ApplyStatusMove(m, domain.ActionDecide, values, "setuju")
	if err != nil {
		t.Fatalf("ApplyStatusMove: %v", err)
	}
	if field != "fld_putusan" || values["fld_putusan"] != "setuju" {
		t.Errorf("moved %q to %v, want fld_putusan = setuju -- both read from the declared transitions", field, values["fld_putusan"])
	}

	// "menunggu" is a real option of that Field and *not* a target of any decide edge: a decision is
	// never sent back to pending, which the edges already say and no Go constant has to.
	if _, err := ApplyStatusMove(m, domain.ActionDecide, map[string]any{}, "menunggu"); err == nil {
		t.Error("ApplyStatusMove() error = nil, want a refusal for a value no declared edge names")
	} else if !strings.Contains(err.Error(), "fld_putusan") {
		t.Errorf("error = %v, want it to name the Field it would have moved", err)
	}

	// An Action with no declared edge moves nothing, and says so without erroring: that is `revise` on a
	// Machine whose status is derived, where the value is a declared literal write instead.
	if field, err := ApplyStatusMove(m, domain.ActionRevise, map[string]any{}, "menunggu"); field != "" || err != nil {
		t.Errorf("ApplyStatusMove(revise) = (%q, %v), want (\"\", nil)", field, err)
	}
}
