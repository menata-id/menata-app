package data

import "menata.app/internal/domain"

// ApplyComputed sets every computed Field (domain.Field.Compute) from the record's other values,
// overwriting whatever was submitted for it and removing it when nothing can be computed yet. Called
// by every route that writes a Machine allowed to declare one: internal/metadata refuses a computed
// Field on any Machine written elsewhere (validateComputedFieldsAreGenericallyWritten).
//
// Runs on the full set of values about to be stored, after defaults, so an update computes from
// the record as it will be, never from a partial submission.
func ApplyComputed(m *domain.Machine, values map[string]any) {
	ApplyComputedWith(m, values, nil)
}

// ApplyComputedWith is ApplyComputed with what a computed value may read beyond the record (domain.ComputeEnv): the
// record a relation points at, and the records pointing at this one. Fields are computed in dependency order
// (Machine.ComputeOrder), so a value computed from another computed value reads it fresh.
func ApplyComputedWith(m *domain.Machine, values map[string]any, env domain.ComputeEnv) {
	for _, f := range m.ComputeOrder() {
		if v, ok := f.Compute.EvaluateWith(values, env); ok {
			values[f.ID] = v
		} else {
			delete(values, f.ID)
		}
	}
}
