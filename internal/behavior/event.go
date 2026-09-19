package behavior

import (
	"fmt"

	"menata.app/internal/domain"
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
