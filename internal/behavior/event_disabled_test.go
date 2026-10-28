package behavior

import (
	"testing"
	"time"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// A rule switched off is matched by none of the three ways a rule runs -- a field changing, a record being created, a date passing -- and the same rule switched on is matched by each.
func TestASwitchedOffRuleIsMatchedByNoTrigger(t *testing.T) {
	m := &domain.Machine{ID: "mch_x", Events: []domain.Event{
		{ID: "evt_change", On: "fld_s", WhenEquals: "Done"},
		{ID: "evt_create", OnCreate: true},
		{ID: "evt_date", Schedule: &domain.Schedule{DateField: "fld_due", When: domain.ScheduleWhenOverdue}},
	}}
	record := &data.Record{ID: "r", Values: map[string]any{"fld_s": "Done", "fld_due": "2020-01-01"}}
	count := func() int {
		return len(MatchedEvents(m, map[string]any{"fld_s": "Open"}, record.Values)) + len(MatchedCreateEvents(m)) + len(MatchedScheduleEvents(m, record, time.Now()))
	}
	if got := count(); got != 3 {
		t.Fatalf("rules switched on matched %d times, want 3", got)
	}
	for i := range m.Events {
		m.Events[i].Disabled = true
	}
	if got := count(); got != 0 {
		t.Errorf("rules switched off matched %d times, want 0", got)
	}
}
