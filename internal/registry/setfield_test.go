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

func TestValidateNotifyRecipients(t *testing.T) {
	m := &domain.Machine{ID: "mch_x"}
	fields := map[string]domain.Field{
		"fld_person": {ID: "fld_person", Type: domain.FieldTypePerson},
		"fld_group":  {ID: "fld_group", Type: domain.FieldTypeGroup},
		"fld_text":   {ID: "fld_text", Type: domain.FieldTypeText},
	}
	ev := func(n domain.Notify) domain.Event {
		n.PreferenceKey = "assigned"
		return domain.Event{ID: "evt_x", Then: domain.Service{Name: domain.ServiceSendNotification, Notify: &n}}
	}
	for _, ok := range []domain.Notify{{RecipientField: "fld_person"}, {RecipientRole: "koordinator"}, {RecipientGroupField: "fld_group"}} {
		if p := validateNotify(m, ev(ok), fields); len(p) != 0 {
			t.Errorf("%+v was refused: %v", ok, p)
		}
	}
	for name, tc := range map[string]struct {
		n    domain.Notify
		want string
	}{
		"nobody":             {domain.Notify{}, "exactly one"},
		"two at once":        {domain.Notify{RecipientField: "fld_person", RecipientRole: "koordinator"}, "exactly one"},
		"three at once":      {domain.Notify{RecipientField: "fld_person", RecipientRole: "r", RecipientGroupField: "fld_group"}, "exactly one"},
		"a missing field":    {domain.Notify{RecipientField: "fld_nope"}, "not a field"},
		"a missing group":    {domain.Notify{RecipientGroupField: "fld_nope"}, "not a field"},
		"a text group field": {domain.Notify{RecipientGroupField: "fld_text"}, "must be a group field"},
	} {
		if got := strings.Join(validateNotify(m, ev(tc.n), fields), "; "); !strings.Contains(got, tc.want) {
			t.Errorf("%s: got %q, want it to contain %q", name, got, tc.want)
		}
	}
}
