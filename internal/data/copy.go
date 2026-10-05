package data

import "menata.app/internal/domain"

// CopyableValues is what a copy of a record starts from: its stored values, minus the Fields a copy must not
// carry over. A computed or stamped Field is written by the runtime for the new record (ApplyComputed,
// ApplyStamps), and an uploaded file would otherwise be one stored object referenced by two records, so
// removing either from one would break the other.
func CopyableValues(m *domain.Machine, values map[string]any) map[string]any {
	out := make(map[string]any, len(values))
	for _, f := range m.Fields {
		if f.Compute != nil || f.Stamp != "" || f.Type == domain.FieldTypeFile {
			continue
		}
		if v, ok := values[f.ID]; ok {
			out[f.ID] = v
		}
	}
	return out
}
