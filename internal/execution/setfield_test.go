package execution

import (
	"context"
	"testing"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// setField writes the declared value, runs the record's field-change Events afterwards, leaves a record that already
// holds the value alone, and moves a governed status only along an edge the runtime performs (one with no action).
func TestSetField(t *testing.T) {
	pool := testPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	ws, err := store.CreateWorkspace(ctx, "Set Field Test", "set-field-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupTest(t, pool, ws.ID, "set_field_test@example.com")
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)

	m := &domain.Machine{
		ID: "mch_set_field_fixture",
		Fields: []domain.Field{
			{ID: "fld_title", Type: domain.FieldTypeText},
			{ID: "fld_priority", Type: domain.FieldTypeStatus, Options: []string{"low", "urgent"}},
			{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"open", "late", "closed"}},
		},
		Transitions: []domain.Transition{
			{ID: "trn_late", Name: "Late", Field: "fld_status", From: "open", To: "late"},
			{ID: "trn_close", Name: "Close", Field: "fld_status", From: "open", To: "closed", Action: domain.ActionEdit},
		},
		Events: []domain.Event{{ID: "evt_priority_changed", On: "fld_priority", Then: domain.Service{Name: domain.ServiceLogActivity, Summary: "priority {old} -> {new}"}}},
	}
	machines := map[string]*domain.Machine{m.ID: m}
	svc := Services{Store: store}
	create := func(values map[string]any) *data.Record {
		r, err := store.CreateRecord(wsCtx, m.ID, values)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	value := func(r *data.Record, f string) any {
		got, err := store.GetRecord(wsCtx, m.ID, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got.Values[f]
	}
	activities := func(r *data.Record) int {
		a, err := store.ListRecordsBy(wsCtx, "mch_activity", "fld_record_id", r.ID)
		if err != nil {
			t.Fatal(err)
		}
		return len(a)
	}

	low := create(map[string]any{"fld_title": "a", "fld_priority": "low", "fld_status": "open"})
	setField(wsCtx, svc, machines, m, low, "", domain.SetField{TargetField: "fld_priority", Value: "urgent"})
	if got := value(low, "fld_priority"); got != "urgent" {
		t.Errorf("an ungoverned field: priority = %v, want urgent", got)
	}
	if n := activities(low); n != 1 {
		t.Errorf("the field-change Event on priority ran %d times, want 1", n)
	}

	held := create(map[string]any{"fld_title": "b", "fld_priority": "urgent", "fld_status": "open"})
	setField(wsCtx, svc, machines, m, held, "", domain.SetField{TargetField: "fld_priority", Value: "urgent"})
	if again, err := store.GetRecord(wsCtx, m.ID, held.ID); err != nil || !again.UpdatedAt.Equal(held.UpdatedAt) {
		t.Errorf("a value already held was written again (updated_at %v -> %v, %v)", held.UpdatedAt, again.UpdatedAt, err)
	}

	open := create(map[string]any{"fld_title": "c", "fld_status": "open"})
	setField(wsCtx, svc, machines, m, open, "", domain.SetField{TargetField: "fld_status", Value: "late"})
	if got := value(open, "fld_status"); got != "late" {
		t.Errorf("an edge the runtime performs: status = %v, want late", got)
	}

	closed := create(map[string]any{"fld_title": "d", "fld_status": "closed"})
	setField(wsCtx, svc, machines, m, closed, "", domain.SetField{TargetField: "fld_status", Value: "late"})
	if got := value(closed, "fld_status"); got != "closed" {
		t.Errorf("an undeclared move was written: status = %v, want closed", got)
	}
	openToClosed := create(map[string]any{"fld_title": "e", "fld_status": "open"})
	setField(wsCtx, svc, machines, m, openToClosed, "", domain.SetField{TargetField: "fld_status", Value: "closed"})
	if got := value(openToClosed, "fld_status"); got != "open" {
		t.Errorf("an edge reserved for a person's edit was taken by the runtime: status = %v, want open", got)
	}
}
