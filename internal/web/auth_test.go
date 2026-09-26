package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
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
		// Before workspace_members, and before workspaces: workspace_member_app_roles references
		// workspaces, so leaving its rows behind makes the final DELETE fail on a foreign key
		// rather than leaving harmless residue (Fase 3b, migration 008).
		if _, err := pool.Exec(ctx, `DELETE FROM workspace_member_app_roles WHERE workspace_id = $1`, workspaceID); err != nil {
			t.Errorf("cleanup workspace_member_app_roles: %v", err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM workspace_members WHERE workspace_id = $1`, workspaceID); err != nil {
			t.Errorf("cleanup workspace_members: %v", err)
		}
		// Invitations live outside workspace_members since migration 011, so they need their own
		// sweep -- and they reference workspaces, same foreign-key reasoning as the line above.
		if _, err := pool.Exec(ctx, `DELETE FROM pending_invites WHERE workspace_id = $1`, workspaceID); err != nil {
			t.Errorf("cleanup pending_invites: %v", err)
		}
		// Before records, and the order is the correctness: a session generation is keyed by
		// subject, which is an mch_user *record id*, so once the records are gone this subquery
		// matches nothing and the rows are orphaned forever. That is what had been happening --
		// the 2026-09-22 query/index study found session_generations holding 369 rows for three
		// credentials, 367 of them orphans, making it the largest table in the dev database,
		// larger than `records` itself (66).
		if _, err := pool.Exec(ctx, `
			DELETE FROM session_generations
			WHERE subject IN (SELECT id FROM records WHERE workspace_id = $1)
		`, workspaceID); err != nil {
			t.Errorf("cleanup session_generations: %v", err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM records WHERE workspace_id = $1`, workspaceID); err != nil {
			t.Errorf("cleanup records: %v", err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM credentials WHERE email = $1`, email); err != nil {
			t.Errorf("cleanup credentials: %v", err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, workspaceID); err != nil {
			t.Errorf("cleanup workspaces: %v", err)
		}
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

// TestAuthenticateMember_invitedEmailWithNoCredentialIsRejected is the core regression test for
// security audit 2026-09-19's H1: before this fix, a membership row with no credential yet let
// *any* first successful "login" attempt activate the account under whatever password was
// POSTed -- an unauthenticated account-takeover path for anyone who knew or guessed the invited
// email. A credential may now only be created through submitAcceptInvite's verified, workspace-
// bound token (invite_test.go) or self-registration -- never through authenticateMember, so an
// invited-but-not-yet-accepted email must simply be rejected, the same as a wrong password would
// be, and no credential must be created as a side effect of the attempt.
func TestAuthenticateMember_invitedEmailWithNoCredentialIsRejected(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "invited_auth_test@example.com"
	newTestMember(t, pool, store, "Auth Test", "auth-test-workspace", email)

	if outcome := authenticateMember(ctx, store, email, "an-attackers-password"); outcome != loginRejected {
		t.Fatalf("authenticateMember(invited, no credential yet) = %v, want loginRejected", outcome)
	}
	if _, err := store.GetCredential(ctx, email); err == nil {
		t.Error("GetCredential() succeeded after a rejected login attempt -- a credential must not be created as a side effect of authenticateMember")
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
	if err := store.CreateCredential(ctx, email, "Test Person", hash, false); err != nil {
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
	if err := store.CreateCredential(ctx, email, "Test Person", hash, true); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}

	if outcome := authenticateMember(ctx, store, email, "a-real-password"); outcome != loginOK {
		t.Errorf("authenticateMember(correct password, verified) = %v, want loginOK", outcome)
	}
}

// TestCompleteLogin_singleArchivedMembershipFallsThroughToChooseWorkspace is Tahap 7's own edge
// case (Flow 2 gap study): the single-membership fast path must not sign an identity straight into
// a Workspace that just became read-only and hidden from ordinary members -- that would strand an
// admin with no visible way to reach Choose Workspace's own Restore. One membership, archived,
// must land on Choose Workspace exactly as if there were several.
func TestCompleteLogin_singleArchivedMembershipFallsThroughToChooseWorkspace(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: "complete-login-archived-test-secret", SecureCookies: false}
	ctx := context.Background()
	const email = "complete_login_archived_test@example.com"

	ws, err := store.CreateWorkspace(ctx, "Complete Login Archived Test", "complete-login-archived-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)
	user, err := store.CreateRecord(data.WithWorkspaceScope(ctx, ws.ID), "mch_user", map[string]any{"fld_name": "Complete Login Archived", "fld_email": email})
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	if err := store.AddMember(ctx, ws.ID, user.ID, email, "admin", ""); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if err := store.ArchiveWorkspace(ctx, ws.ID); err != nil {
		t.Fatalf("ArchiveWorkspace: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	rec := httptest.NewRecorder()
	if err := completeLogin(rec, req, cfg, store, email); err != nil {
		t.Fatalf("completeLogin: %v", err)
	}
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("completeLogin(sole archived membership) status = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/choose-workspace" {
		t.Errorf("completeLogin(sole archived membership) redirected to %q, want /choose-workspace (not /home)", got)
	}
	if cookies := rec.Result().Cookies(); len(cookies) == 0 {
		t.Error("completeLogin(sole archived membership) set no cookie, want the pending-email cookie Choose Workspace reads")
	}
}
