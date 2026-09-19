package web

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

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

func TestAuthenticateMember_activatesAnInvitedEmailOnFirstLogin(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "invited_auth_test@example.com"

	ws, err := store.CreateWorkspace(ctx, "Auth Test", "auth-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)

	user, err := store.CreateRecord(data.WithWorkspaceScope(ctx, ws.ID), "mch_user", map[string]any{"fld_name": email, "fld_email": email})
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	if err := store.AddMember(ctx, ws.ID, user.ID, email, "member", "approver"); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	// No credential exists yet -- the first login attempt should activate the account rather than
	// being rejected as a wrong password.
	userID, chooseWorkspace, ok := authenticateMember(ctx, store, email, "a-real-first-password")
	if !ok || chooseWorkspace || userID != user.ID {
		t.Fatalf("authenticateMember(first attempt) = (%q, %v, %v), want (%q, false, true)", userID, chooseWorkspace, ok, user.ID)
	}

	// The password just set must now be the real credential -- a wrong password fails, the same
	// one succeeds again.
	if _, _, ok := authenticateMember(ctx, store, email, "wrong-password"); ok {
		t.Error("authenticateMember(wrong password after activation) ok = true, want false")
	}
	if _, _, ok := authenticateMember(ctx, store, email, "a-real-first-password"); !ok {
		t.Error("authenticateMember(same password again) ok = false, want true")
	}
}

func TestAuthenticateMember_tooShortFirstPasswordDoesNotActivate(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "invited_short_test@example.com"

	ws, err := store.CreateWorkspace(ctx, "Auth Short Test", "auth-short-test-workspace")
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

	if _, _, ok := authenticateMember(ctx, store, email, "short"); ok {
		t.Error("authenticateMember(too-short first password) ok = true, want false")
	}
	if _, err := store.GetCredential(ctx, email); err == nil {
		t.Error("GetCredential() succeeded after a rejected activation attempt, want ErrCredentialNotFound still")
	}
}

func TestAuthenticateMember_noMembership(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)

	if _, _, ok := authenticateMember(context.Background(), store, "nobody@example.com", "whatever123"); ok {
		t.Error("authenticateMember(no membership) ok = true, want false")
	}
}
