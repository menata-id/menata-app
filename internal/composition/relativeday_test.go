package composition

import (
	"testing"
	"time"
)

// TestRelativeDaySaysADayAgainstToday: the derived role a K01a overdue line reads ("12 days late").
func TestRelativeDaySaysADayAgainstToday(t *testing.T) {
	now := time.Date(2026, 10, 17, 15, 0, 0, 0, time.UTC)
	for stored, want := range map[string]string{
		"2026-10-05": "12 days late", "2026-10-16": "1 day late", "2026-10-17": "today",
		"2026-10-18": "tomorrow", "2026-10-22": "in 5 days", "soon": "",
	} {
		if got := relativeDay(stored, false, now); got != want {
			t.Errorf("relativeDay(%s) = %q, want %q", stored, got, want)
		}
	}
	if got := relativeDay("2026-10-05", true, now); got != "05 Oct 2026" {
		t.Errorf("a finished record's day = %q, want its date", got)
	}
}
