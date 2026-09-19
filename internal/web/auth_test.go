package web

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"menata.app/internal/authorization"
	"menata.app/internal/data"
)

func authTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("set DATABASE_URL to run internal/web's auth integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func cleanupAuthTest(t *testing.T, pool *pgxpool.Pool, workspaceID, email string) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		pool.Exec(ctx, `DELETE FROM workspace_members WHERE workspace_id = $1`, workspaceID)
		pool.Exec(ctx, `DELETE FROM records WHERE workspace_id = $1`, workspaceID)
		pool.Exec(ctx, `DELETE FROM credentials WHERE email = $1`, email)
		pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, workspaceID)
	})
}

// newTestMember creates a Workspace + mch_user record + membership for email, returning the
// created user record's id -- the shared setup every authenticateMember test below needs.
func newTestMember(t *testing.T, pool *pgxpool.Pool, store *data.Store, workspaceName, slug, email string) string {
	t.Helper()
	ctx := context.Background()
	ws, err := store.CreateWorkspace(ctx, workspaceName, slug)
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)

	user, err := store.CreateRecord(data.WithWorkspaceScope(ctx, ws.ID), "mch_user", map[string]any{"fld_name": email, "fld_email": email})
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	if err := store.AddMember(ctx, ws.ID, user.ID, email, "member", ""); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	return user.ID
}

func TestAuthenticateMember_activatesAnInvitedEmailOnFirstLogin(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "invited_auth_test@example.com"
	newTestMember(t, pool, store, "Auth Test", "auth-test-workspace", email)

	// No credential exists yet -- the first login attempt should activate the account rather than
	// being rejected as a wrong password, and the activated credential starts verified (an invite
	// is a different trust model than self-registration).
	if outcome := authenticateMember(ctx, store, email, "a-real-first-password"); outcome != loginOK {
		t.Fatalf("authenticateMember(first attempt) = %v, want loginOK", outcome)
	}

	// The password just set must now be the real credential -- a wrong password fails, the same
	// one succeeds again.
	if outcome := authenticateMember(ctx, store, email, "wrong-password"); outcome != loginRejected {
		t.Errorf("authenticateMember(wrong password after activation) = %v, want loginRejected", outcome)
	}
	if outcome := authenticateMember(ctx, store, email, "a-real-first-password"); outcome != loginOK {
		t.Errorf("authenticateMember(same password again) = %v, want loginOK", outcome)
	}
}

func TestAuthenticateMember_tooShortFirstPasswordDoesNotActivate(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "invited_short_test@example.com"
	newTestMember(t, pool, store, "Auth Short Test", "auth-short-test-workspace", email)

	if outcome := authenticateMember(ctx, store, email, "short"); outcome != loginRejected {
		t.Errorf("authenticateMember(too-short first password) = %v, want loginRejected", outcome)
	}
	if _, err := store.GetCredential(ctx, email); err == nil {
		t.Error("GetCredential() succeeded after a rejected activation attempt, want ErrCredentialNotFound still")
	}
}

func TestAuthenticateMember_noMembership(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)

	if outcome := authenticateMember(context.Background(), store, "nobody@example.com", "whatever123"); outcome != loginRejected {
		t.Errorf("authenticateMember(no membership) = %v, want loginRejected", outcome)
	}
}

func TestAuthenticateMember_unverifiedCredentialNeedsVerification(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "unverified_auth_test@example.com"
	newTestMember(t, pool, store, "Unverified Auth Test", "unverified-auth-test-workspace", email)

	hash, err := authorization.HashPassword("a-real-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := store.CreateCredential(ctx, email, hash, false); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}

	if outcome := authenticateMember(ctx, store, email, "a-real-password"); outcome != loginNeedsVerification {
		t.Errorf("authenticateMember(correct password, unverified) = %v, want loginNeedsVerification", outcome)
	}
	// A wrong password against an unverified credential is still just rejected, not conflated with
	// "needs verification" -- that distinction only matters once the password itself is right.
	if outcome := authenticateMember(ctx, store, email, "wrong-password"); outcome != loginRejected {
		t.Errorf("authenticateMember(wrong password, unverified) = %v, want loginRejected", outcome)
	}
}

func TestAuthenticateMember_verifiedCredentialSucceeds(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "verified_auth_test@example.com"
	newTestMember(t, pool, store, "Verified Auth Test", "verified-auth-test-workspace", email)

	hash, err := authorization.HashPassword("a-real-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := store.CreateCredential(ctx, email, hash, true); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}

	if outcome := authenticateMember(ctx, store, email, "a-real-password"); outcome != loginOK {
		t.Errorf("authenticateMember(correct password, verified) = %v, want loginOK", outcome)
	}
}
