package composition

import (
	"testing"

	"menata.app/internal/data"
)

func TestFilterMembers(t *testing.T) {
	members := []data.Membership{
		{UserRecordID: "u1", Email: "silvia@menata.id"},
		{UserRecordID: "u2", Email: "rina.nur@gmail.com"},
		{UserRecordID: "u3", Email: "maya.puspita@yahoo.com"},
	}
	names := map[string]string{
		"u1": "Silvia Indah Rini",
		"u2": "Rina Nur",
		// u3 deliberately has no resolved name (the shared admin credential's own case) -- must
		// still match on email alone.
	}

	cases := []struct {
		name string
		q    string
		want []string // expected UserRecordIDs, in order
	}{
		{"empty query returns everything unfiltered", "", []string{"u1", "u2", "u3"}},
		{"matches by name, case-insensitively", "SILVIA", []string{"u1"}},
		{"matches by email when no name is resolved", "maya.puspita", []string{"u3"}},
		{"matches nothing returns an empty, not nil, slice", "nobody-like-this", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := FilterMembers(members, names, c.q)
			if got == nil {
				t.Fatal("FilterMembers returned nil, want a non-nil slice (possibly empty)")
			}
			if len(got) != len(c.want) {
				t.Fatalf("got %d members, want %d: %+v", len(got), len(c.want), got)
			}
			for i, id := range c.want {
				if got[i].UserRecordID != id {
					t.Errorf("got[%d] = %q, want %q", i, got[i].UserRecordID, id)
				}
			}
		})
	}

	// The empty-query case is the one where returning the input slice unchanged (rather than a
	// copy) is intentional -- confirm that path doesn't accidentally allocate a nil result either.
	if got := FilterMembers(nil, names, ""); got != nil {
		t.Errorf("FilterMembers(nil, ..., \"\") = %v, want nil unchanged", got)
	}
}

func TestFilterPendingInvites(t *testing.T) {
	pending := []data.PendingInvite{
		{Email: "budi.santoso@menata.id"},
		{Email: "siti.wulandari@gmail.com"},
	}

	if got := FilterPendingInvites(pending, ""); len(got) != 2 {
		t.Errorf("empty query = %v, want both invites unchanged", got)
	}
	got := FilterPendingInvites(pending, "budi")
	if len(got) != 1 || got[0].Email != "budi.santoso@menata.id" {
		t.Errorf("FilterPendingInvites(..., \"budi\") = %+v, want just budi.santoso@menata.id", got)
	}
	if got := FilterPendingInvites(pending, "nobody"); len(got) != 0 {
		t.Errorf("no-match query = %v, want an empty slice", got)
	}
}
