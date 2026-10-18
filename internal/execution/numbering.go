package execution

import (
	"context"
	"fmt"
	"time"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// NumberingLockKey names the lock two creates of a numbered Field queue on: each reads the highest number in use and writes
// the next, so the second must see what the first committed.
func NumberingLockKey(workspaceID, machineID, fieldID string) string {
	return fmt.Sprintf("numbering|%s|%s|%s", workspaceID, machineID, fieldID)
}

// Numbered says m has a Field the runtime numbers, so a create of it belongs in a transaction.
func Numbered(m *domain.Machine) bool {
	for _, f := range m.Fields {
		if f.Numbering != nil {
			return true
		}
	}
	return false
}

// AssignNumbers writes every numbered Field of m (domain.FieldNumbering) into the values of a record about to be created,
// overwriting whatever was submitted for it: a number is the runtime's, not a client's. It must run inside the create's
// transaction -- it takes the Field's lock and reads the highest number under the same prefix, so two creates never write one
// number. now is the clock the pattern's `{year}` reads.
func AssignNumbers(ctx context.Context, store *data.Store, m *domain.Machine, values map[string]any, now time.Time) error {
	workspaceID, _ := data.WorkspaceScope(ctx)
	for _, f := range m.Fields {
		if f.Numbering == nil {
			continue
		}
		prefix, width, err := f.Numbering.NumberPrefix(values, now.Year())
		if err != nil {
			return fmt.Errorf("%s %s: %w", m.ID, f.ID, err)
		}
		if err := store.Lock(ctx, NumberingLockKey(workspaceID, m.ID, f.ID)); err != nil {
			return err
		}
		highest, err := store.HighestNumber(ctx, m.ID, f.ID, prefix)
		if err != nil {
			return err
		}
		values[f.ID] = domain.Number(prefix, width, highest+1)
	}
	return nil
}
