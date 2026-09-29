package behavior

import (
	"testing"
	"time"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

func taskMachineWithEvent() *domain.Machine {
	return &domain.Machine{
		ID:   "mch_task",
		Name: "Task",
		// The Field the Event below watches. Absent until 2026-09-29 (metadata.Validate).
		Fields: []domain.Field{{ID: "fld_status", Name: "Status", Type: domain.FieldTypeStatus, Options: []string{"todo", "done"}}},
		Events: []domain.Event{
			{
				ID: "evt_task_status_changed",
				On: "fld_status",
				Then: domain.Service{
					Name:                domain.ServiceLogActivity,
					Summary:             "moved from {old} to {new}",
					SummaryOverrideWhen: "done",
					SummaryOverride:     "completed",
				},
			},
		},
	}
}

func TestMatchedEvents_firesOnValueChange(t *testing.T) {
	old := map[string]any{"fld_status": "todo"}
	new_ := map[string]any{"fld_status": "in_progress"}

	got := MatchedEvents(taskMachineWithEvent(), old, new_)
	if len(got) != 1 || got[0].ID != "evt_task_status_changed" {
		t.Fatalf("MatchedEvents() = %v, want exactly evt_task_status_changed", got)
	}
}

func TestMatchedEvents_doesNotFireWhenUnchanged(t *testing.T) {
	values := map[string]any{"fld_status": "todo"}

	got := MatchedEvents(taskMachineWithEvent(), values, values)
	if len(got) != 0 {
		t.Errorf("MatchedEvents() = %v, want none: fld_status did not change", got)
	}
}

func TestMatchedEvents_respectsWhenEquals(t *testing.T) {
	m := &domain.Machine{
		ID: "mch_task",
		Events: []domain.Event{
			{ID: "evt_task_done", On: "fld_status", WhenEquals: "done", Then: domain.Service{Name: domain.ServiceLogActivity, Summary: "done"}},
		},
	}

	old := map[string]any{"fld_status": "todo"}

	if got := MatchedEvents(m, old, map[string]any{"fld_status": "in_progress"}); len(got) != 0 {
		t.Errorf("MatchedEvents(-> in_progress) = %v, want none: when_equals is %q", got, "done")
	}
	if got := MatchedEvents(m, old, map[string]any{"fld_status": "done"}); len(got) != 1 {
		t.Errorf("MatchedEvents(-> done) = %v, want exactly one match", got)
	}
}

func TestMatchedEvents_noEventsDeclaredYieldsEmpty(t *testing.T) {
	m := &domain.Machine{ID: "mch_project"}

	got := MatchedEvents(m, map[string]any{"fld_status": "todo"}, map[string]any{"fld_status": "done"})
	if len(got) != 0 {
		t.Errorf("MatchedEvents() = %v, want none: this Machine declares no Events", got)
	}
}

// TestMatchedEvents_ignoresOnCreateEvents is the field-change side of OnCreate's mutual
// exclusion: an OnCreate Event must never fire from MatchedEvents (the update path), only from
// MatchedCreateEvents -- even if its Machine also has an unrelated field genuinely changing.
func TestMatchedEvents_ignoresOnCreateEvents(t *testing.T) {
	m := &domain.Machine{
		ID: "mch_task",
		Events: []domain.Event{
			{ID: "evt_task_created", OnCreate: true, Then: domain.Service{Name: domain.ServiceLogActivity, Summary: "created"}},
		},
	}

	got := MatchedEvents(m, map[string]any{"fld_status": "todo"}, map[string]any{"fld_status": "done"})
	if len(got) != 0 {
		t.Errorf("MatchedEvents() = %v, want none: evt_task_created is OnCreate, not a field-change Event", got)
	}
}

func taskMachineWithCreateEvent() *domain.Machine {
	return &domain.Machine{
		ID:   "mch_task",
		Name: "Task",
		// The Field the Event below watches. Absent until 2026-09-29 (metadata.Validate).
		Fields: []domain.Field{{ID: "fld_status", Name: "Status", Type: domain.FieldTypeStatus, Options: []string{"todo", "done"}}},
		Events: []domain.Event{
			{ID: "evt_task_status_changed", On: "fld_status", Then: domain.Service{Name: domain.ServiceLogActivity, Summary: "moved"}},
			{ID: "evt_task_created", OnCreate: true, Then: domain.Service{Name: domain.ServiceLogActivity, Summary: "created"}},
		},
	}
}

func TestMatchedCreateEvents_firesUnconditionally(t *testing.T) {
	got := MatchedCreateEvents(taskMachineWithCreateEvent())
	if len(got) != 1 || got[0].ID != "evt_task_created" {
		t.Fatalf("MatchedCreateEvents() = %v, want exactly evt_task_created", got)
	}
}

func TestMatchedCreateEvents_noCreateEventsDeclaredYieldsEmpty(t *testing.T) {
	m := &domain.Machine{
		ID:     "mch_task",
		Events: []domain.Event{{ID: "evt_task_status_changed", On: "fld_status", Then: domain.Service{Name: domain.ServiceLogActivity, Summary: "moved"}}},
	}

	if got := MatchedCreateEvents(m); len(got) != 0 {
		t.Errorf("MatchedCreateEvents() = %v, want none: this Machine declares no OnCreate Events", got)
	}
}

func documentMachineWithScheduleEvent() *domain.Machine {
	return &domain.Machine{
		ID: "mch_document", Name: "Document",
		// The two Fields the schedule Event below reads -- its date and its guard (metadata.Validate,
		// 2026-09-29).
		Fields: []domain.Field{
			{ID: "fld_due_date", Name: "Due", Type: domain.FieldTypeDate},
			{ID: "fld_status", Name: "Status", Type: domain.FieldTypeStatus, Options: []string{"in_review", "approved"}},
		},
		Events: []domain.Event{
			{
				ID: "evt_document_overdue_notify",
				Schedule: &domain.Schedule{
					DateField: "fld_due_date", When: domain.ScheduleWhenOverdue,
					GuardField: "fld_status", GuardEquals: "in_review",
				},
				Then: domain.Service{Name: domain.ServiceLogActivity, Summary: "overdue"},
			},
		},
	}
}

func TestMatchedScheduleEvents_firesWhenOverdueAndGuardMatches(t *testing.T) {
	m := documentMachineWithScheduleEvent()
	record := &data.Record{ID: "doc_1", Values: map[string]any{"fld_due_date": "2026-09-01", "fld_status": "in_review"}}
	now := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)

	got := MatchedScheduleEvents(m, record, now)
	if len(got) != 1 || got[0].ID != "evt_document_overdue_notify" {
		t.Fatalf("MatchedScheduleEvents() = %v, want exactly evt_document_overdue_notify", got)
	}
}

func TestMatchedScheduleEvents_skipsWhenNotYetDue(t *testing.T) {
	m := documentMachineWithScheduleEvent()
	record := &data.Record{ID: "doc_1", Values: map[string]any{"fld_due_date": "2026-09-30", "fld_status": "in_review"}}
	now := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)

	if got := MatchedScheduleEvents(m, record, now); len(got) != 0 {
		t.Errorf("MatchedScheduleEvents() = %v, want none: fld_due_date has not passed yet", got)
	}
}

func TestMatchedScheduleEvents_skipsWhenGuardFieldDoesNotMatch(t *testing.T) {
	m := documentMachineWithScheduleEvent()
	record := &data.Record{ID: "doc_1", Values: map[string]any{"fld_due_date": "2026-09-01", "fld_status": "approved"}}
	now := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)

	if got := MatchedScheduleEvents(m, record, now); len(got) != 0 {
		t.Errorf("MatchedScheduleEvents() = %v, want none: fld_status is approved, guard requires in_review", got)
	}
}

func TestMatchedScheduleEvents_skipsWhenDateFieldEmptyOrUnparseable(t *testing.T) {
	m := documentMachineWithScheduleEvent()
	now := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)

	empty := &data.Record{ID: "doc_1", Values: map[string]any{"fld_status": "in_review"}}
	if got := MatchedScheduleEvents(m, empty, now); len(got) != 0 {
		t.Errorf("MatchedScheduleEvents(no due date) = %v, want none", got)
	}

	garbled := &data.Record{ID: "doc_2", Values: map[string]any{"fld_due_date": "not-a-date", "fld_status": "in_review"}}
	if got := MatchedScheduleEvents(m, garbled, now); len(got) != 0 {
		t.Errorf("MatchedScheduleEvents(unparseable due date) = %v, want none", got)
	}
}
