package registry

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func TestValidateSetField(t *testing.T) {
	m := &domain.Machine{ID: "mch_x"}
	fields := map[string]domain.Field{
		"fld_priority": {ID: "fld_priority", Type: domain.FieldTypeStatus, Options: []string{"low", "urgent"}},
		"fld_note":     {ID: "fld_note", Type: domain.FieldTypeText},
	}
	sched := &domain.Schedule{DateField: "fld_due", When: domain.ScheduleWhenOverdue}
	ev := func(s *domain.Schedule, set *domain.SetField) domain.Event {
		return domain.Event{ID: "evt_x", Schedule: s, Then: domain.Service{Name: domain.ServiceSetField, Set: set}}
	}
	if p := validateSetField(m, ev(sched, &domain.SetField{TargetField: "fld_priority", Value: "urgent"}), fields); len(p) != 0 {
		t.Fatalf("a sound set_field was refused: %v", p)
	}
	for _, tc := range []struct {
		name string
		e    domain.Event
		want string
	}{
		{"no target", ev(sched, nil), "requires target_field and value"},
		{"on a write", ev(nil, &domain.SetField{TargetField: "fld_priority", Value: "urgent"}), "only on a schedule event"},
		{"a missing field", ev(sched, &domain.SetField{TargetField: "fld_nope", Value: "urgent"}), "is not a field"},
		{"a text field", ev(sched, &domain.SetField{TargetField: "fld_note", Value: "x"}), "must be a status field"},
		{"a value that is no option", ev(sched, &domain.SetField{TargetField: "fld_priority", Value: "high"}), "is not one of"},
	} {
		if got := strings.Join(validateSetField(m, tc.e, fields), "; "); !strings.Contains(got, tc.want) {
			t.Errorf("%s: got %q, want it to contain %q", tc.name, got, tc.want)
		}
	}
}
