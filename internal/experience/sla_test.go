package experience

import (
	"testing"
	"time"
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
