package data

import "menata.app/internal/domain"

// ApplyDefaults fills in each Field's declared Default for a new record, wherever values leaves
// that Field empty (absent, nil, or an explicit empty string -- the same "empty" ValidateRecord
// already uses for required-ness). Create-only by convention: callers apply this before
// ValidateRecord on a create path, never on an update, so clearing a field back to empty is never
// silently re-filled -- the same distinction SQL's own DEFAULT makes (001 Principle #5,
// "Convention over Configuration").
func ApplyDefaults(m *domain.Machine, values map[string]any) {
	for _, f := range m.Fields {
		if f.Default == nil {
			continue
		}
		v, present := values[f.ID]
		if present && v != nil && v != "" {
			continue
		}
		values[f.ID] = f.Default
	}
}
