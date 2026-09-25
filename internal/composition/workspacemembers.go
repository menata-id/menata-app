package composition

import (
	"strings"

	"menata.app/internal/data"
)

// FilterMembers narrows members to those whose display name (from names, keyed by
// UserRecordID) or email contains q, case-insensitively. An empty q returns members unchanged --
// the Members screen's own "search or show everything" convention, matching every other filter on
// this app (an unset ?filter=/?status= means "all"). Never returns nil for a non-nil, non-matching
// input: an empty *slice* renders "no matches" correctly, while a nil one is indistinguishable from
// "search not attempted" to a caller checking len().
func FilterMembers(members []data.Membership, names map[string]string, q string) []data.Membership {
	if q == "" {
		return members
	}
	needle := strings.ToLower(q)
	out := make([]data.Membership, 0, len(members))
	for _, m := range members {
		if strings.Contains(strings.ToLower(names[m.UserRecordID]), needle) || strings.Contains(strings.ToLower(m.Email), needle) {
			out = append(out, m)
		}
	}
	return out
}

// FilterPendingInvites is FilterMembers' own counterpart for the "Waiting to accept" list, which
// carries no display name of its own (data.PendingInvite -- an invitation is not a membership,
// project_identity-model's own rule) -- only the invited email to match against.
func FilterPendingInvites(pending []data.PendingInvite, q string) []data.PendingInvite {
	if q == "" {
		return pending
	}
	needle := strings.ToLower(q)
	out := make([]data.PendingInvite, 0, len(pending))
	for _, p := range pending {
		if strings.Contains(strings.ToLower(p.Email), needle) {
			out = append(out, p)
		}
	}
	return out
}
