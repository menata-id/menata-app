package action

import (
	"testing"

	"menata.app/internal/data"
)

// step builds one Approval Step record. Kept here after the sequencing tests moved to
// internal/behavior: delete.go's own rule still reads a step's decision.
func step(id string, seq float64, decision string) *data.Record {
	return &data.Record{
		ID: id,
		Values: map[string]any{
			FieldStepSequence: seq,
			FieldStepDecision: decision,
		},
	}
}

func TestDocumentReference(t *testing.T) {
	cases := []struct {
		sortOrder int64
		want      string
	}{
		{1, "DOC-0001"},
		{91, "DOC-0091"},
		{0, "DOC-0000"},
		{10000, "DOC-10000"},
	}
	for _, c := range cases {
		if got := DocumentReference(c.sortOrder); got != c.want {
			t.Errorf("DocumentReference(%d) = %q, want %q", c.sortOrder, got, c.want)
		}
	}
}
