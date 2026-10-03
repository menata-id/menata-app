package experience

import (
	"fmt"
	"time"

	"menata.app/internal/domain"
)

// SLAStatus is the urgency of a due-date Field's value relative to now (development-history.md Phase 13,
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

// SLABadge is the resolved input for a StatusBadge rendering a due date: a label and a tone, with nothing
// left to parse or decide.
//
// It exists because the two `.templ` files that drew this badge were doing the deciding themselves --
// `rendering.slaBadgePill` took an `any`, parsed `"2006-01-02"` out of it and called
// `EvaluateSLA(due, time.Now())`, and `approvalinbox.parseSLA` did the same thing a second time. That was
// wrong in three separate ways, which is why it is worth naming all of them:
//
//   - **007 §4.6 Determinism, a MUST.** `time.Now()` inside a renderer means identical input does not
//     produce identical output. Unlike the `searchBox` map-ordering breach found the same day, this one
//     changes a *semantic* value: the same record renders "1 day left" and then "OVERDUE" with no input
//     having changed.
//   - **007 §4.4 Projection over Retrieval, and §12.3 Component boundedness.** Composition resolves the
//     shape and a Page renders it; a Component "MUST NOT silently acquire additional business data that is
//     not represented by its contract", and parsing a date out of an `any` is exactly that.
//   - **001 #8 Reference over Duplication.** `internal/composition` already called `EvaluateSLA` in four
//     places with an injected `now`. Rendering was a fifth and sixth evaluation point with a different
//     clock source.
//
// `Present` distinguishes "no due date" from "due today": an unset or unparseable value renders nothing at
// all, which is the behaviour both former renderers already had and the one thing about them worth keeping.
type SLABadge struct {
	Label   string
	Tone    domain.BadgeTone
	Present bool
}

// ResolveSLABadge turns a stored due-date value into a resolved badge. `now` is a parameter, never the
// clock: that is the whole point of the function existing.
//
// It tolerates nil, a non-string, and an unparseable string by returning `Present: false`, because a Field
// declared in the date role is not required to hold a value and a screen must not break when it does not.
func ResolveSLABadge(value any, now time.Time) SLABadge {
	s, ok := value.(string)
	if !ok {
		if value == nil {
			return SLABadge{}
		}
		s = fmt.Sprint(value)
	}
	due, err := time.Parse("2006-01-02", s)
	if err != nil {
		return SLABadge{}
	}
	status, label := EvaluateSLA(due, now)
	tone := domain.ToneNeutral
	if status == SLAOverdue {
		tone = domain.ToneBad
	}
	return SLABadge{Label: label, Tone: tone, Present: true}
}
