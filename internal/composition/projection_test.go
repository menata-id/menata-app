package composition

import (
	"reflect"
	"testing"
	"time"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

func TestProjectCardFields(t *testing.T) {
	m := &domain.Machine{
		ID: "mch_widget",
		Fields: []domain.Field{
			{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText},
			{ID: "fld_owner", Name: "Owner", Type: domain.FieldTypePerson, RelatedMachine: domain.UserMachineID},
		},
		CardFields: []domain.CardField{
			{Field: "fld_title", Role: domain.CardFieldRoleTitle},
			{Field: "fld_owner", Role: domain.CardFieldRolePerson},
		},
	}
	r := rec("wdg_1", map[string]any{
		"fld_title": "Widget One",
		"fld_owner": "usr_ana",
	})
	relations := rendering.RelationOptions{
		"mch_user": {{ID: "usr_ana", Label: "Ana Putri"}},
	}

	got := ProjectCardFields(m, r, relations)
	want := []rendering.ProjectedField{
		{Label: "Title", Role: "title", Display: "Widget One"},
		{Label: "Owner", Role: "person", Display: "Ana Putri"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ProjectCardFields() = %+v, want %+v", got, want)
	}
}

// TestProjectCardFields_skipsMissingField is the defensive case: card_fields: naming a Field
// id that no longer exists on m (stale metadata after a rename/removal that internal/metadata.
// Validate would already reject at load time) must not panic a live render -- it's skipped.
func TestProjectCardFields_skipsMissingField(t *testing.T) {
	m := &domain.Machine{
		ID:     "mch_widget",
		Fields: []domain.Field{{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText}},
		CardFields: []domain.CardField{
			{Field: "fld_ghost", Role: domain.CardFieldRoleTitle},
		},
	}
	r := rec("wdg_1", map[string]any{"fld_title": "Widget One"})

	got := ProjectCardFields(m, r, nil)
	if len(got) != 0 {
		t.Errorf("ProjectCardFields() = %+v, want empty (stale field id skipped)", got)
	}
}

func TestProjectCardFields_dateRole(t *testing.T) {
	m := &domain.Machine{
		ID:     "mch_widget",
		Fields: []domain.Field{{ID: "fld_due", Name: "Due", Type: domain.FieldTypeDate}},
		CardFields: []domain.CardField{
			{Field: "fld_due", Role: domain.CardFieldRoleDate},
		},
	}
	r := rec("wdg_1", map[string]any{"fld_due": "2026-09-19"})

	got := ProjectCardFields(m, r, nil)
	want := []rendering.ProjectedField{{Label: "Due", Role: "date", Display: "19 Sep 2026"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ProjectCardFields() = %+v, want %+v", got, want)
	}
}

// RecordCards hands each card its date pill already resolved, and "finished" is whatever the Machine's own
// completion: declares -- here a value that is *not* called "done", which is what proves nothing is
// hardcoded: P3 compared against the literal.
func TestRecordCards_datePillFollowsDeclaredCompletion(t *testing.T) {
	m := &domain.Machine{
		ID: "mch_job",
		Fields: []domain.Field{
			{ID: "fld_name", Name: "Name", Type: domain.FieldTypeText},
			{ID: "fld_state", Name: "State", Type: domain.FieldTypeStatus, Options: []string{"open", "shipped"}},
			{ID: "fld_due", Name: "Due", Type: domain.FieldTypeDate},
		},
		Completion: &domain.Completion{Field: "fld_state", Done: "shipped"},
		CardFields: []domain.CardField{
			{Field: "fld_name", Role: domain.CardFieldRoleTitle},
			{Field: "fld_due", Role: domain.CardFieldRoleDate},
		},
	}
	board := domain.View{ID: "vw", Type: domain.ViewBoard}
	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	rec := func(state, due string) *data.Record {
		return &data.Record{ID: "r", Values: map[string]any{"fld_name": "x", "fld_state": state, "fld_due": due}}
	}

	cards := RecordCards(m, board, []*data.Record{rec("open", "2026-10-01"), rec("shipped", "2026-10-01"), rec("open", "")}, nil, nil, now)
	if got := cards[0].Date; got.Tone != domain.ToneBad || got.Done {
		t.Errorf("open and past due = %+v, want overdue", got)
	}
	if got := cards[1].Date; got.Tone != domain.ToneGood || !got.Done {
		t.Errorf("shipped and past due = %+v, want finished, not overdue", got)
	}
	if cards[2].Date.Present {
		t.Errorf("a record with no date drew a pill: %+v", cards[2].Date)
	}

	m.Completion = nil
	if cards := RecordCards(m, board, []*data.Record{rec("shipped", "2026-10-01")}, nil, nil, now); cards[0].Date.Tone != domain.ToneBad {
		t.Errorf("a Machine declaring no completion has no finished records, got %+v", cards[0].Date)
	}
}

// A card's circle writes the opposite of what the record holds: the declared done value to finish it, the
// Field's default (else its first option) to reopen it. A Machine declaring no completion offers none.
func TestRecordCards_completeToggleWritesTheOpposite(t *testing.T) {
	m := &domain.Machine{
		ID: "mch_job",
		Fields: []domain.Field{
			{ID: "fld_name", Name: "Name", Type: domain.FieldTypeText},
			{ID: "fld_state", Name: "State", Type: domain.FieldTypeStatus, Options: []string{"open", "wip", "shipped"}, Default: "wip"},
		},
		Completion: &domain.Completion{Field: "fld_state", Done: "shipped"},
		CardFields: []domain.CardField{{Field: "fld_name", Role: domain.CardFieldRoleTitle}},
	}
	board := domain.View{ID: "vw", Type: domain.ViewBoard}
	rec := func(state string) *data.Record {
		return &data.Record{ID: "r", Values: map[string]any{"fld_name": "x", "fld_state": state}}
	}
	cards := RecordCards(m, board, []*data.Record{rec("open"), rec("shipped")}, nil, nil, time.Now())
	if got := cards[0].Complete; got == nil || got.Done || got.Next != "shipped" || got.Field != "fld_state" {
		t.Errorf("open card toggle = %+v, want to write shipped", got)
	}
	if got := cards[1].Complete; got == nil || !got.Done || got.Next != "wip" {
		t.Errorf("finished card toggle = %+v, want to reopen to the Field's default", got)
	}
	m.Fields[1].Default = nil
	if got := RecordCards(m, board, []*data.Record{rec("shipped")}, nil, nil, time.Now())[0].Complete; got.Next != "open" {
		t.Errorf("with no default the first option reopens, got %q", got.Next)
	}
	m.Completion = nil
	if got := RecordCards(m, board, []*data.Record{rec("open")}, nil, nil, time.Now())[0].Complete; got != nil {
		t.Errorf("no completion declared, yet the card offers %+v", got)
	}
}
