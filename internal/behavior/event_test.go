package behavior

import (
	"testing"

	"menata.app/internal/domain"
)

func taskMachineWithEvent() *domain.Machine {
	return &domain.Machine{
		ID:   "mch_task",
		Name: "Task",
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
