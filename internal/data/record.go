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
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "rec_" + hex.EncodeToString(b)
}
