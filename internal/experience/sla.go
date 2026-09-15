package experience

import (
	"fmt"
	"time"
)

// SLAStatus is the urgency of a due-date Field's value relative to now (ROADMAP.md Phase 13,
// matching document-approval.html's own real OVERDUE / "N day(s) left" badges).
type SLAStatus string

const (
	SLAOverdue SLAStatus = "overdue"
	SLAOK      SLAStatus = "ok"
)

// EvaluateSLA compares due against now (both truncated to the day, so "due today" isn't overdue
// by a few hours) and returns the badge status plus its display label.
func EvaluateSLA(due, now time.Time) (SLAStatus, string) {
	d := truncateToDay(due)
	n := truncateToDay(now)
	days := int(d.Sub(n).Hours() / 24)

	if days < 0 {
		return SLAOverdue, "OVERDUE"
	}
	if days == 0 {
		return SLAOK, "Due today"
	}
	if days == 1 {
		return SLAOK, "1 day left"
	}
	return SLAOK, fmt.Sprintf("%d days left", days)
}

func truncateToDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
