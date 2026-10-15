package data

import (
	"fmt"
	"time"

	"menata.app/internal/domain"
)

// ApplyDefaults fills in each Field's declared Default for a new record, wherever values leaves
// that Field empty (absent, nil, or an explicit empty string -- the same "empty" ValidateRecord
// already uses for required-ness). Create-only by convention: callers apply this before
// ValidateRecord on a create path, never on an update, so clearing a field back to empty is never
// silently re-filled -- the same distinction SQL's own DEFAULT makes (001 Principle #5,
// "Convention over Configuration").
func ApplyDefaults(m *domain.Machine, values map[string]any) {
	ApplyDefaultsAt(m, values, time.Now())
}

// ApplyDefaultsAt is ApplyDefaults with the day a derived date counts from named, so a test does not wait for
// the clock. A Field declaring default_from takes today plus the days its watched Field's value maps to; a value
// with no entry derives nothing.
func ApplyDefaultsAt(m *domain.Machine, values map[string]any, now time.Time) {
	empty := func(id string) bool {
		v, present := values[id]
		return !present || v == nil || v == ""
	}
	// Plain defaults first, so a derived date reads the status a default just filled whatever the declaration order.
	for _, f := range m.Fields {
		if f.Default != nil && empty(f.ID) {
			values[f.ID] = f.Default
		}
	}
	for _, f := range m.Fields {
		if f.DefaultFrom == nil || !empty(f.ID) {
			continue
		}
		if days, ok := f.DefaultFrom.Days[fmt.Sprint(values[f.DefaultFrom.Field])]; ok {
			values[f.ID] = now.AddDate(0, 0, days).Format("2006-01-02")
		}
	}
}
