package expression

import "testing"

// TestKnownSentinel_failsClosed is the assertion 007 §9.2 asks for by name: "Access outside this
// context must fail closed."
//
// A sentinel the runtime does not know must be rejected, not compared as a literal. The failure it
// prevents is quiet: a filter comparing a Field against the string "$current_usr" matches nothing,
// renders an empty list, and is indistinguishable from a screen that legitimately has no rows.
func TestKnownSentinel_failsClosed(t *testing.T) {
	known := []string{"$current_user", "$parameters.document", "$parameters.x", "$today", "$today+7", "$today-1", "$today+0"}
	for _, v := range known {
		if !KnownSentinel(v) {
			t.Errorf("KnownSentinel(%q) = false, want true", v)
		}
	}

	unknown := []string{
		"$current_usr",     // a typo -- the case this exists for
		"$currentuser",     //
		"$parameters.",     // no name after the dot
		"$parameters",      // the prefix alone names nothing
		"$Today",           // case matters
		"$now",             //
		"$record.fld_x",    // §9.2 names `record`; not built either
		"$",                //
		"$anything_at_all", //
		"$today+",          // an offset with no number
		"$today-",          //
		"$today+7d",        // a unit: a day is the only grain
		"$today + 7",       // spaces
		"$today+-7",        //
		"$today+1.5",       // whole days
		"$todays",          // a longer name, not an offset
		"$today+36501",     // past MaxTodayOffsetDays
	}
	for _, v := range unknown {
		if KnownSentinel(v) {
			t.Errorf("KnownSentinel(%q) = true -- an unknown sentinel must fail closed, or it becomes "+
				"a literal that silently matches nothing", v)
		}
	}
}

// TestResolve_leavesOrdinaryValuesAlone: every declaration that exists today compares against a
// literal, and none of them may change meaning.
func TestResolve_leavesOrdinaryValuesAlone(t *testing.T) {
	c := Context{CurrentUser: "usr_1"}
	for _, v := range []string{"done", "pending", "", "10", "a value with $ inside"} {
		got, ok := c.Resolve(v)
		if !ok || got != v {
			t.Errorf("Resolve(%q) = %q, %v; want the value unchanged", v, got, ok)
		}
	}
}

func TestResolve_sentinels(t *testing.T) {
	c := Context{CurrentUser: "usr_1", Parameters: map[string]string{"document": "rec_9"}}

	if got, ok := c.Resolve("$current_user"); !ok || got != "usr_1" {
		t.Errorf("Resolve($current_user) = %q, %v; want usr_1, true", got, ok)
	}
	if got, ok := c.Resolve("$parameters.document"); !ok || got != "rec_9" {
		t.Errorf("Resolve($parameters.document) = %q, %v; want rec_9, true", got, ok)
	}
}

// TestResolve_unresolvableIsNotAnEmptyString: a filter comparing against "" is a *different query*
// than the one declared -- it matches records whose Field is absent -- and it would do it quietly.
// The same distinction domain.Resolution draws between "undeclared" and "input unavailable".
func TestResolve_unresolvableIsNotAnEmptyString(t *testing.T) {
	noViewer := Context{}
	if got, ok := noViewer.Resolve("$current_user"); ok {
		t.Errorf("Resolve($current_user) with no viewer = %q, true; want not-ok, so the caller cannot "+
			"silently run a filter against the empty string", got)
	}

	c := Context{CurrentUser: "usr_1", Parameters: map[string]string{"document": ""}}
	if _, ok := c.Resolve("$parameters.document"); ok {
		t.Error("an empty parameter resolved successfully; an absent value must fail rather than filter on \"\"")
	}
	if _, ok := c.Resolve("$parameters.missing"); ok {
		t.Error("a missing parameter resolved successfully")
	}
}

// $today is the request's date, injected: this package may not read the clock (007 §4.6). A caller that
// supplies none gets not-ok, never "" -- a filter on "" would be a different query, quietly.
func TestResolve_today(t *testing.T) {
	if got, ok := (Context{Today: "2026-10-10"}).Resolve("$today"); !ok || got != "2026-10-10" {
		t.Errorf("Resolve($today) = %q, %v; want 2026-10-10, true", got, ok)
	}
	if got, ok := (Context{}).Resolve("$today"); ok {
		t.Errorf("Resolve($today) with no date = %q, true; want not-ok", got)
	}
}

// $today+n / $today-n are whole days from the request's date, resolved by the one place that resolves a
// sentinel, so the SQL realisation and the in-memory one cannot disagree about what a week from now is.
func TestResolve_todayOffset(t *testing.T) {
	c := Context{Today: "2026-10-10"}
	for value, want := range map[string]string{
		"$today":     "2026-10-10",
		"$today+0":   "2026-10-10",
		"$today+7":   "2026-10-17",
		"$today-1":   "2026-10-09",
		"$today+22":  "2026-11-01", // across a month
		"$today-10":  "2026-09-30",
		"$today+100": "2027-01-18", // across a year
	} {
		if got, ok := c.Resolve(value); !ok || got != want {
			t.Errorf("Resolve(%q) = %q, %v; want %q, true", value, got, ok, want)
		}
	}
	if got, ok := (Context{Today: "2028-02-28"}).Resolve("$today+1"); !ok || got != "2028-02-29" {
		t.Errorf("Resolve($today+1) from 2028-02-28 = %q, %v; want the leap day", got, ok)
	}
	for _, value := range []string{"$today+7", "$today-1"} {
		if got, ok := (Context{}).Resolve(value); ok {
			t.Errorf("Resolve(%q) with no date = %q, true; want not-ok", value, got)
		}
	}
	if got, ok := (Context{Today: "not a date"}).Resolve("$today+1"); ok {
		t.Errorf("Resolve($today+1) from an unparsable date = %q, true; want not-ok", got)
	}
}

// $week is the Monday of the request's week, with `?week=N` stepping whole weeks. 2026-10-10 is a Saturday, so
// its week starts on 2026-10-05; every day of the week -- Sunday included, which Go counts as day 0 -- must land
// on the same Monday.
func TestResolve_weekIsTheMondayOfTheSelectedWeek(t *testing.T) {
	for today, want := range map[string]string{
		"2026-10-05": "2026-10-05", // Monday
		"2026-10-06": "2026-10-05",
		"2026-10-10": "2026-10-05", // Saturday
		"2026-10-11": "2026-10-05", // Sunday: the end of this week, not the start of the next
		"2026-10-12": "2026-10-12", // the next Monday
		"2026-03-01": "2026-02-23", // across a month
		"2027-01-01": "2026-12-28", // across a year
	} {
		if got, ok := (Context{Today: today}).Resolve("$week"); !ok || got != want {
			t.Errorf("Resolve($week) on %s = %q, %v; want %q, true", today, got, ok, want)
		}
	}
}

func TestResolve_weekOffsetsAreDaysFromThatMonday(t *testing.T) {
	c := Context{Today: "2026-10-10"}
	for value, want := range map[string]string{
		"$week+0": "2026-10-05",
		"$week+6": "2026-10-11", // the Sunday that closes the window
		"$week+7": "2026-10-12",
		"$week-1": "2026-10-04",
		"$week+4": "2026-10-09",
	} {
		if got, ok := c.Resolve(value); !ok || got != want {
			t.Errorf("Resolve(%q) = %q, %v; want %q, true", value, got, ok, want)
		}
	}
}

func TestResolve_weekParameterStepsWholeWeeksAndFailsSafeToThisWeek(t *testing.T) {
	for param, want := range map[string]string{
		"":                      "2026-10-05",
		"0":                     "2026-10-05",
		"1":                     "2026-10-12",
		"-1":                    "2026-09-28",
		"abc":                   "2026-10-05", // not a number is this week, as the bespoke Calendar read it
		"1.5":                   "2026-10-05",
		"9999":                  "2036-09-22",
		"-9999":                 "2016-10-17",
		"99999999999999999999":  "2036-09-22", // out of int range: still clamped, not read as 0
		"-99999999999999999999": "2016-10-17", // clamped to 520 weeks, not asking time for a date it cannot represent
	} {
		c := Context{Today: "2026-10-10", Parameters: map[string]string{"week": param}}
		if got, ok := c.Resolve("$week"); !ok || got != want {
			t.Errorf("Resolve($week) with ?week=%q = %q, %v; want %q, true", param, got, ok, want)
		}
	}
	// The Sunday of a stepped week is still six days after its own Monday.
	c := Context{Today: "2026-10-10", Parameters: map[string]string{"week": "2"}}
	if got, _ := c.Resolve("$week+6"); got != "2026-10-25" {
		t.Errorf("Resolve($week+6) with ?week=2 = %q; want 2026-10-25", got)
	}
}

func TestResolve_weekNeedsADateAndIsNotAParameterOrToday(t *testing.T) {
	for _, value := range []string{"$week", "$week+6"} {
		if got, ok := (Context{}).Resolve(value); ok {
			t.Errorf("Resolve(%q) with no date = %q, true; want not-ok", value, got)
		}
	}
	if got, ok := (Context{Today: "not a date"}).Resolve("$week"); ok {
		t.Errorf("Resolve($week) from an unparsable date = %q, true; want not-ok", got)
	}
	for _, bad := range []string{"$weeks", "$week+", "$week+x", "$week 1", "$week+99999999", "$weekday"} {
		if KnownSentinel(bad) {
			t.Errorf("KnownSentinel(%q) = true; a malformed week value must be a load error, not a literal", bad)
		}
	}
	for _, good := range []string{"$week", "$week+6", "$week-1"} {
		if !KnownSentinel(good) || IsToday(good) || !IsWeek(good) || !IsDate(good) {
			t.Errorf("%q must be a known week value and not a today value", good)
		}
	}
	if IsWeek("$today+1") {
		t.Error("IsWeek($today+1) = true")
	}
}

func TestRequestWeekIsTheOffsetWeekReadsAndClampsIt(t *testing.T) {
	for param, want := range map[string]int{"": 0, "abc": 0, "1.5": 0, "3": 3, "-2": -2, "9999": MaxWeekOffset, "-9999": -MaxWeekOffset, "99999999999999999999": MaxWeekOffset, "-99999999999999999999": -MaxWeekOffset} {
		if got := RequestWeek(map[string]string{"week": param}); got != want {
			t.Errorf("RequestWeek(?week=%q) = %d, want %d", param, got, want)
		}
	}
	if got := RequestWeek(nil); got != 0 {
		t.Errorf("RequestWeek(nil) = %d, want 0", got)
	}
}

// $month is the first day of the request's month and $month_end its last, with `?month=N` stepping whole months from the one that
// holds today. 2026-10-19 is in October, a 31-day month; February of a leap year ends on the 29th; a step across a year lands on
// the right December/January; and nothing reads the clock.
func TestResolve_monthIsTheFirstAndLastDayOfTheSelectedMonth(t *testing.T) {
	for _, tc := range []struct {
		today, param, first, last string
	}{
		{"2026-10-19", "", "2026-10-01", "2026-10-31"},
		{"2026-10-31", "", "2026-10-01", "2026-10-31"},
		{"2026-10-19", "0", "2026-10-01", "2026-10-31"},
		{"2026-10-19", "1", "2026-11-01", "2026-11-30"},
		{"2026-10-19", "-1", "2026-09-01", "2026-09-30"},
		{"2026-10-19", "3", "2027-01-01", "2027-01-31"},
		{"2028-01-31", "1", "2028-02-01", "2028-02-29"}, // from the 31st: February, not March
		{"2026-01-31", "1", "2026-02-01", "2026-02-28"},
		{"2026-10-19", "abc", "2026-10-01", "2026-10-31"},
		{"2026-10-19", "1.5", "2026-10-01", "2026-10-31"},
	} {
		c := Context{Today: tc.today, Parameters: map[string]string{"month": tc.param}}
		if got, ok := c.Resolve("$month"); !ok || got != tc.first {
			t.Errorf("Resolve($month) on %s with ?month=%q = %q, %v; want %q", tc.today, tc.param, got, ok, tc.first)
		}
		if got, ok := c.Resolve("$month_end"); !ok || got != tc.last {
			t.Errorf("Resolve($month_end) on %s with ?month=%q = %q, %v; want %q", tc.today, tc.param, got, ok, tc.last)
		}
	}
	// Out of bounds is clamped, never a date time cannot represent: ten years is the bound either way.
	for _, param := range []string{"99999", "-99999", "99999999999999999999"} {
		c := Context{Today: "2026-10-19", Parameters: map[string]string{"month": param}}
		got, ok := c.Resolve("$month")
		if !ok {
			t.Fatalf("?month=%s resolved to nothing", param)
		}
		year := got[:4]
		if year != "2036" && year != "2016" {
			t.Errorf("?month=%s = %s, want the bound (2036 or 2016)", param, got)
		}
	}
}

func TestResolve_monthNeedsADateAndIsExactOnly(t *testing.T) {
	for _, value := range []string{"$month", "$month_end"} {
		if got, ok := (Context{}).Resolve(value); ok {
			t.Errorf("Resolve(%q) with no date = %q, true; want not-ok", value, got)
		}
		if !KnownSentinel(value) || !IsDate(value) {
			t.Errorf("%q must be a known date value", value)
		}
	}
	for _, bad := range []string{"$month+1", "$month-1", "$months", "$month_start", "$month_end+1", "$monthly"} {
		if KnownSentinel(bad) {
			t.Errorf("KnownSentinel(%q) = true; a malformed month value must be a load error, not a literal", bad)
		}
	}
}
