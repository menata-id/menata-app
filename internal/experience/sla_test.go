package experience

import (
	"testing"
	"time"

	"menata.app/internal/domain"
)

func TestEvaluateSLA(t *testing.T) {
	now := time.Date(2026, 9, 15, 14, 30, 0, 0, time.UTC) // mid-afternoon, to prove day-truncation

	cases := []struct {
		name       string
		due        time.Time
		wantStatus SLAStatus
		wantLabel  string
	}{
		{"yesterday is overdue", now.AddDate(0, 0, -1), SLAOverdue, "OVERDUE"},
		{"today is not overdue despite the time-of-day difference", time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC), SLAOK, "Due today"},
		{"tomorrow", now.AddDate(0, 0, 1), SLAOK, "1 day left"},
		{"in 5 days", now.AddDate(0, 0, 5), SLAOK, "5 days left"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, label := EvaluateSLA(c.due, now)
			if status != c.wantStatus || label != c.wantLabel {
				t.Errorf("EvaluateSLA() = (%q, %q), want (%q, %q)", status, label, c.wantStatus, c.wantLabel)
			}
		})
	}
}

// TestResolveRecordSLABadge_aFinishedRecordIsNeverOverdue: a paid instalment past its due date shows its date,
// good-toned; an unfinished one past its date is still OVERDUE.
func TestResolveRecordSLABadge_aFinishedRecordIsNeverOverdue(t *testing.T) {
	now := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	done := ResolveRecordSLABadge("2026-10-05", true, now)
	if !done.Present || done.Label != "5 Oct 2026" || done.Tone != domain.ToneGood || done.Urgent {
		t.Errorf("finished = %+v, want its date, good-toned, not urgent", done)
	}
	open := ResolveRecordSLABadge("2026-10-05", false, now)
	if open.Label != "OVERDUE" || open.Tone != domain.ToneBad {
		t.Errorf("unfinished = %+v, want OVERDUE", open)
	}
}
