package composition

import (
	"testing"

	"menata.app/internal/domain"
)

// TestStatusStepsMarkWhatIsPassedAndLeaveOutTheWayOut: a lifecycle's options before the current one are done, the ones
// after it are to come, and an option categorised `stopped` is a way out drawn only when the record is there.
func TestStatusStepsMarkWhatIsPassedAndLeaveOutTheWayOut(t *testing.T) {
	f := domain.Field{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"Draft", "Active", "Closed", "Cancelled"},
		Categories: map[string]domain.StatusCategory{"Draft": domain.StatusStart, "Active": domain.StatusActive, "Closed": domain.StatusDone, "Cancelled": domain.StatusStopped}}
	m := &domain.Machine{Fields: []domain.Field{f}}
	for current, want := range map[string]string{
		"Draft":     "Draft\tcurrent\nActive\ttodo\nClosed\ttodo",
		"Active":    "Draft\tdone\nActive\tcurrent\nClosed\ttodo",
		"Closed":    "Draft\tdone\nActive\tdone\nClosed\tcurrent",
		"Cancelled": "Draft\ttodo\nActive\ttodo\nClosed\ttodo\nCancelled\tcurrent",
	} {
		if got := statusSteps(m, f, current); got != want {
			t.Errorf("%s: steps\n%q, want\n%q", current, got, want)
		}
	}
}
