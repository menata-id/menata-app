package data

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// Record is one instance of a Machine, its field values keyed by Field ID
// (006-runtime-model.md "Dataset": a Dataset describes available data and its meaning; Record is
// the underlying data it exposes).
type Record struct {
	ID          string
	MachineID   string
	WorkspaceID string
	Values      map[string]any
	SortOrder   int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// newRecordID generates a stable-identity record ID, following the mch_/fld_ prefix convention
// (004-runtime-metadata.md "Stable Identity").
func newRecordID() string {
	return newID("rec_")
}

// newID generates a random opaque id with the given prefix -- the same generator record ids use,
// reused for Workspace ids (ROADMAP.md Phase 21 Step 3) now that a second real caller needs it.
func newID(prefix string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}
