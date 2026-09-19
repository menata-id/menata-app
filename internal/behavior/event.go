package behavior

import (
	"fmt"

	"menata.app/internal/domain"
)

// MatchedEvents returns every Event on m that fires for a write whose new field values are
// newValues, given oldValues (the record's pre-write values, nil for a record that did not exist
// before this write). Pure: no I/O, mirroring CheckConstraints's own posture -- the caller
// fetches oldValues; this package never does.
func MatchedEvents(m *domain.Machine, oldValues, newValues map[string]any) []domain.Event {
	var out []domain.Event
	for _, e := range m.Events {
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
