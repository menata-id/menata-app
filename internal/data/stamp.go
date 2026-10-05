package data

import "menata.app/internal/domain"

// ApplyStamps sets every stamped Field (domain.Field.Stamp) on a record about to be created, overwriting
// whatever was submitted for it: a client does not get to name who a record is by. With no acting user the
// Field is left empty rather than invented.
//
// Create only. An edit never re-stamps -- the author of a comment does not become whoever last touched it --
// so the edit routes carry the stored value forward instead (internal/web.carryForwardFixedFields).
func ApplyStamps(m *domain.Machine, values map[string]any, actorID string) {
	for _, f := range m.Fields {
		if f.Stamp != domain.FieldStampCurrentUser {
			continue
		}
		if actorID == "" {
			delete(values, f.ID)
			continue
		}
		values[f.ID] = actorID
	}
}
