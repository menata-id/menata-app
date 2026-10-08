package data

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestAnonymizeIdentity_removesTheLoginAndKeepsTheWorkspaceUsable(t *testing.T) {
	pool := storePool(t)
	store := NewStore(pool)
	ctx := context.Background()

	ws, err := store.CreateWorkspace(ctx, "Anonymize Test", "anonymize-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	const admin, leaver = "anon-admin@example.test", "anon-leaver@example.test"
	var anon string
	cleanupWorkspaceTest(t, pool, ws.ID, admin)
	t.Cleanup(func() {
		for _, e := range []string{leaver, anon} {
			if e == "" {
				continue
			}
			_, _ = pool.Exec(ctx, `DELETE FROM credentials WHERE email = $1`, e)
		}
		_, _ = pool.Exec(ctx, `DELETE FROM session_generations WHERE subject IN ('usr_anon_admin','usr_anon_leaver')`)
	})

	for _, e := range []string{admin, leaver} {
		if err := store.CreateCredential(ctx, e, "Person "+e, "hash-"+e, true); err != nil {
			t.Fatalf("CreateCredential %s: %v", e, err)
		}
	}
	if err := store.AddMember(ctx, ws.ID, "usr_anon_admin", admin, "admin", ""); err != nil {
		t.Fatalf("AddMember admin: %v", err)
	}
	if err := store.AddMember(ctx, ws.ID, "usr_anon_leaver", leaver, "admin", ""); err != nil {
		t.Fatalf("AddMember leaver: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO pending_invites (workspace_id, email, workspace_role) VALUES ($1, $2, 'member')`, ws.ID, leaver); err != nil {
		t.Fatalf("seed invite: %v", err)
	}

	// With a second active admin present, neither is the sole admin.
	if sole, err := store.SoleAdminOf(ctx, leaver); err != nil || len(sole) != 0 {
		t.Fatalf("SoleAdminOf(leaver) = %v, %v; want none", sole, err)
	}

	ids, err := store.AnonymizeIdentity(ctx, leaver)
	if err != nil {
		t.Fatalf("AnonymizeIdentity: %v", err)
	}
	if len(ids) != 1 || ids[0] != "usr_anon_leaver" {
		t.Fatalf("record ids = %v, want [usr_anon_leaver]", ids)
	}

	// The old address no longer resolves, so it can neither sign in nor be mistaken for the person.
	if _, err := store.GetCredential(ctx, leaver); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("GetCredential(old email) err = %v, want ErrCredentialNotFound", err)
	}

	names, err := store.MemberNames(ctx, ws.ID)
	if err != nil {
		t.Fatalf("MemberNames: %v", err)
	}
	if got := names["usr_anon_leaver"]; got != DeletedUserName {
		t.Errorf("name for deleted member = %q, want %q", got, DeletedUserName)
	}
	if got := names["usr_anon_admin"]; got != "Person "+admin {
		t.Errorf("name for remaining member = %q, want it untouched", got)
	}

	if err := pool.QueryRow(ctx, `SELECT email FROM workspace_members WHERE workspace_id = $1 AND user_record_id = 'usr_anon_leaver'`, ws.ID).Scan(&anon); err != nil {
		t.Fatalf("read anonymized email: %v", err)
	}
	if !strings.HasSuffix(anon, "@"+DeletedEmailDomain) || strings.Contains(anon, "leaver") {
		t.Errorf("anonymized email = %q, want a random address under %s", anon, DeletedEmailDomain)
	}
	cred, err := store.GetCredential(ctx, anon)
	if err != nil {
		t.Fatalf("GetCredential(anonymized): %v", err)
	}
	if cred.PasswordHash != "" || cred.EmailVerified || cred.NotifyAssigned || cred.NotifyDecided || cred.NotifySLABreach {
		t.Errorf("anonymized credential still active: %+v", cred)
	}

	var deactivated bool
	if err := pool.QueryRow(ctx, `SELECT deactivated_at IS NOT NULL FROM workspace_members WHERE user_record_id = 'usr_anon_leaver'`).Scan(&deactivated); err != nil || !deactivated {
		t.Errorf("membership deactivated = %v, err %v; want true", deactivated, err)
	}
	var invites int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM pending_invites WHERE email = $1`, leaver).Scan(&invites)
	if invites != 0 {
		t.Errorf("pending invites for old address = %d, want 0", invites)
	}
	var gen int
	if err := pool.QueryRow(ctx, `SELECT generation FROM session_generations WHERE subject = 'usr_anon_leaver'`).Scan(&gen); err != nil || gen < 1 {
		t.Errorf("session generation = %d, err %v; want bumped", gen, err)
	}

	// The remaining admin is now the only one, and the store says so.
	sole, err := store.SoleAdminOf(ctx, admin)
	if err != nil || len(sole) != 1 || sole[0] != "Anonymize Test" {
		t.Errorf("SoleAdminOf(admin) = %v, %v; want [Anonymize Test]", sole, err)
	}
	if _, err := store.AnonymizeIdentity(ctx, admin); !errors.Is(err, ErrSoleWorkspaceAdmin) {
		t.Errorf("AnonymizeIdentity(sole admin) err = %v, want ErrSoleWorkspaceAdmin", err)
	}
	if _, err := store.GetCredential(ctx, admin); err != nil {
		t.Errorf("a refused deletion must leave the credential intact: %v", err)
	}
}
