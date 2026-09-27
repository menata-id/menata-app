package behavior

import (
	"fmt"
	"time"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/experience"
)

// MatchedEvents returns every field-change Event on m (domain.Event.OnCreate false) that fires
// for a write whose new field values are newValues, given oldValues (the record's pre-write
// values, nil for a record that did not exist before this write). Pure: no I/O, mirroring
// CheckConstraints's own posture -- the caller fetches oldValues; this package never does.
func MatchedEvents(m *domain.Machine, oldValues, newValues map[string]any) []domain.Event {
	var out []domain.Event
	for _, e := range m.Events {
		if e.OnCreate {
			continue
		}
		newVal, oldVal := fmt.Sprint(newValues[e.On]), fmt.Sprint(oldValues[e.On])
		if newVal == oldVal {
			continue
		}
		if e.WhenEquals != "" && newVal != e.WhenEquals {
			continue
		}
		out = append(out, e)
	}
	return out
}

// MatchedCreateEvents returns every Event on m declared OnCreate -- fires once, unconditionally,
// the moment a new record exists, so unlike MatchedEvents there is no old/new comparison to make
// at all. Pure: no I/O, same posture as MatchedEvents.
func MatchedCreateEvents(m *domain.Machine) []domain.Event {
	var out []domain.Event
	for _, e := range m.Events {
		if e.OnCreate {
			out = append(out, e)
		}
	}
	return out
}

// MatchedScheduleEvents returns every schedule-shaped Event on m whose condition record currently
// satisfies, evaluated against now -- the third shape (domain.Event's own doc comment), fired on
// the passage of time rather than on any write, so unlike MatchedEvents/MatchedCreateEvents there
// is no old/new write to compare against at all; internal/execution.RunScheduledEvents is the only
// caller, on a ticker. Pure: no I/O, same posture as the other two.
//
// A record with no value yet in Schedule.DateField (or one that fails to parse) never matches --
// there is nothing to be overdue against. Reuses experience.EvaluateSLA rather than
// reimplementing its day-truncation rule, the same convention every other date-field reader in
// this codebase already follows.
func MatchedScheduleEvents(m *domain.Machine, record *data.Record, now time.Time) []domain.Event {
	var out []domain.Event
	for _, e := range m.Events {
		s := e.Schedule
		if s == nil {
			continue
		}
		due, err := time.Parse("2006-01-02", fmt.Sprint(record.Values[s.DateField]))
		if err != nil {
			continue
		}
		status, _ := experience.EvaluateSLA(due, now)
		if s.When == domain.ScheduleWhenOverdue && status != experience.SLAOverdue {
			continue
		}
		if s.GuardField != "" && fmt.Sprint(record.Values[s.GuardField]) != s.GuardEquals {
			continue
		}
		out = append(out, e)
	}
	return out
}
