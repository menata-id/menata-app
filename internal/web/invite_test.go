package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
)

// TestSubmitAcceptInvite_validTokenCreatesCredentialAndLogsIn is the mirror-image regression test
// of TestAuthenticateMember_invitedEmailWithNoCredentialIsRejected: the only place a credential may
// now be created for an invited email is through a valid, workspace-bound invite token.
func TestSubmitAcceptInvite_validTokenCreatesCredentialAndLogsIn(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: "accept-invite-test-secret", SecureCookies: false}
	ctx := context.Background()
	const email = "accept_invite_test@example.com"

	ws, err := store.CreateWorkspace(ctx, "Accept Invite Test", "accept-invite-test-workspace")
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

	token := authorization.NewInviteToken(cfg.SessionSecret, email, ws.ID)
	form := strings.NewReader("token=" + token + "&password=a-real-invite-password")
	req := httptest.NewRequest(http.MethodPost, "/accept-invite", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	submitAcceptInvite(store, cfg)(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("submitAcceptInvite(valid token) status = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}
	if outcome := authenticateMember(ctx, store, email, "a-real-invite-password"); outcome != loginOK {
		t.Errorf("authenticateMember(password just set via accept-invite) = %v, want loginOK", outcome)
	}
	cred, err := store.GetCredential(ctx, email)
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if !cred.EmailVerified {
		t.Error("credential created via accept-invite has EmailVerified = false, want true (an admin already vouched for this email)")
	}
}

// TestSubmitAcceptInvite_alreadyAcceptedTokenIsRejected covers replay: a token, once used to
// create a credential, must not be usable again to overwrite it with a different password.
// Now that submitAcceptInvite verifies against an existing credential rather than rejecting it
// outright (CAP-O10 follow-up), a wrong-password replay re-renders the form (200, "Incorrect
// password") instead of the token-level 400 -- the credential still isn't overwritten either way,
// which is the actual property this test guards.
func TestSubmitAcceptInvite_alreadyAcceptedTokenIsRejected(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: "accept-invite-replay-test-secret", SecureCookies: false}
	ctx := context.Background()
	const email = "accept_invite_replay_test@example.com"

	ws, err := store.CreateWorkspace(ctx, "Accept Invite Replay Test", "accept-invite-replay-test-workspace")
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

	token := authorization.NewInviteToken(cfg.SessionSecret, email, ws.ID)
	post := func(password string) *httptest.ResponseRecorder {
		form := strings.NewReader("token=" + token + "&password=" + password)
		req := httptest.NewRequest(http.MethodPost, "/accept-invite", form)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		submitAcceptInvite(store, cfg)(rec, req)
		return rec
	}

	if rec := post("first-real-password"); rec.Code != http.StatusSeeOther {
		t.Fatalf("first accept status = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}
	rec := post("an-attackers-second-password")
	if rec.Code != http.StatusOK {
		t.Fatalf("replayed accept (wrong password) status = %d, want 200 (re-rendered form); body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Incorrect password") {
		t.Errorf("replayed accept (wrong password) body missing the incorrect-password message; body=%s", rec.Body.String())
	}
	if outcome := authenticateMember(ctx, store, email, "an-attackers-second-password"); outcome != loginRejected {
		t.Error("authenticateMember(attacker's replayed password) succeeded -- replay must not overwrite the credential")
	}
	// The original password set by the first accept must still be the real one.
	if outcome := authenticateMember(ctx, store, email, "first-real-password"); outcome != loginOK {
		t.Error("authenticateMember(original password) failed after a wrong-password replay -- credential must not have changed")
	}
}

// TestSubmitAcceptInvite_wrongSecretTokenIsRejected covers a tampered/forged token.
func TestSubmitAcceptInvite_wrongSecretTokenIsRejected(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: "accept-invite-tamper-test-secret", SecureCookies: false}
	ctx := context.Background()
	const email = "accept_invite_tamper_test@example.com"

	ws, err := store.CreateWorkspace(ctx, "Accept Invite Tamper Test", "accept-invite-tamper-test-workspace")
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

	forgedToken := authorization.NewInviteToken("a-different-secret", email, ws.ID)
	form := strings.NewReader("token=" + forgedToken + "&password=whatever-password")
	req := httptest.NewRequest(http.MethodPost, "/accept-invite", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	submitAcceptInvite(store, cfg)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("submitAcceptInvite(wrong-secret token) status = %d, want 400", rec.Code)
	}
	if _, err := store.GetCredential(ctx, email); err == nil {
		t.Error("GetCredential() succeeded after a rejected forged token, want ErrCredentialNotFound still")
	}
}

// TestSubmitAcceptInvite_membershipRemovedSinceIsRejected covers the case the workspaceID binding
// exists for: an invite token that was valid when sent, but whose membership has since been
// removed (e.g. the admin undid the invite) must not still be honorable just because the token
// itself hasn't expired yet.
func TestSubmitAcceptInvite_membershipRemovedSinceIsRejected(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: "accept-invite-removed-test-secret", SecureCookies: false}
	ctx := context.Background()
	const email = "accept_invite_removed_test@example.com"

	ws, err := store.CreateWorkspace(ctx, "Accept Invite Removed Test", "accept-invite-removed-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)

	// A token minted for a Workspace this email never actually has a membership row in (stands in
	// for "the membership was removed after the invite email went out").
	token := authorization.NewInviteToken(cfg.SessionSecret, email, ws.ID)
	form := strings.NewReader("token=" + token + "&password=whatever-password")
	req := httptest.NewRequest(http.MethodPost, "/accept-invite", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	submitAcceptInvite(store, cfg)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("submitAcceptInvite(no current membership) status = %d, want 400", rec.Code)
	}
	if _, err := store.GetCredential(ctx, email); err == nil {
		t.Error("GetCredential() succeeded although the invite's membership no longer exists")
	}
}

// TestSubmitAcceptInvite_existingCredentialVerifiesExistingPassword is the regression test for the
// CAP-O10 follow-up (writing-guide-reference.md §7): an email invited to a second Workspace while
// already holding a credential elsewhere must be able to accept by confirming that existing
// password -- not be rejected outright the way a replayed/expired token is.
func TestSubmitAcceptInvite_existingCredentialVerifiesExistingPassword(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: "accept-invite-existing-cred-test-secret", SecureCookies: false}
	ctx := context.Background()
	const email = "accept_invite_existing_cred_test@example.com"

	// An identity that already registered (or accepted an earlier invite) elsewhere.
	existingHash, err := authorization.HashPassword("the-existing-real-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := store.CreateCredential(ctx, email, existingHash, true); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM credentials WHERE email = $1`, email)
	})

	// A second Workspace invites the same email -- submitInviteMember's own shape: a membership
	// row with no bearing on whether a credential already exists elsewhere.
	ws, err := store.CreateWorkspace(ctx, "Accept Invite Existing Cred Test", "accept-invite-existing-cred-test-workspace")
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

	token := authorization.NewInviteToken(cfg.SessionSecret, email, ws.ID)
	form := strings.NewReader("token=" + token + "&password=the-existing-real-password")
	req := httptest.NewRequest(http.MethodPost, "/accept-invite", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	submitAcceptInvite(store, cfg)(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("submitAcceptInvite(existing credential, correct password) status = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}
	cred, err := store.GetCredential(ctx, email)
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if cred.PasswordHash != existingHash {
		t.Error("credential hash changed after accepting a second-Workspace invite -- an existing credential must be confirmed, not overwritten")
	}
}

// TestSubmitAcceptInvite_existingCredentialWrongPasswordRejected is
// TestSubmitAcceptInvite_existingCredentialVerifiesExistingPassword's negative case: a wrong
// guess must be rejected with a specific message (the token itself is valid here, only the
// password is wrong -- a different situation from an invalid/expired/replayed token).
func TestSubmitAcceptInvite_existingCredentialWrongPasswordRejected(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: "accept-invite-existing-cred-wrong-test-secret", SecureCookies: false}
	ctx := context.Background()
	const email = "accept_invite_existing_cred_wrong_test@example.com"

	existingHash, err := authorization.HashPassword("the-existing-real-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := store.CreateCredential(ctx, email, existingHash, true); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM credentials WHERE email = $1`, email)
	})

	ws, err := store.CreateWorkspace(ctx, "Accept Invite Existing Cred Wrong Test", "accept-invite-existing-cred-wrong-test-workspace")
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

	token := authorization.NewInviteToken(cfg.SessionSecret, email, ws.ID)
	form := strings.NewReader("token=" + token + "&password=a-wrong-guess")
	req := httptest.NewRequest(http.MethodPost, "/accept-invite", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	submitAcceptInvite(store, cfg)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("submitAcceptInvite(existing credential, wrong password) status = %d, want 200 (re-rendered form); body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Incorrect password") {
		t.Errorf("body missing the incorrect-password message; body=%s", rec.Body.String())
	}
	cred, err := store.GetCredential(ctx, email)
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if cred.PasswordHash != existingHash {
		t.Error("credential hash changed after a wrong-password accept attempt")
	}
}

// TestSubmitAcceptInvite_tooShortPasswordDoesNotCreateCredential mirrors
// TestAuthenticateMember_invitedEmailWithNoCredentialIsRejected's own "no side effect" assertion,
// for the accept-invite path's own length check.
func TestSubmitAcceptInvite_tooShortPasswordDoesNotCreateCredential(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: "accept-invite-short-test-secret", SecureCookies: false}
	ctx := context.Background()
	const email = "accept_invite_short_test@example.com"

	ws, err := store.CreateWorkspace(ctx, "Accept Invite Short Test", "accept-invite-short-test-workspace")
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

	token := authorization.NewInviteToken(cfg.SessionSecret, email, ws.ID)
	form := strings.NewReader("token=" + token + "&password=short")
	req := httptest.NewRequest(http.MethodPost, "/accept-invite", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	submitAcceptInvite(store, cfg)(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("submitAcceptInvite(short password) status = %d, want 200 (re-rendered form)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "at least 8 characters") {
		t.Errorf("submitAcceptInvite(short password) body missing the length error; body=%s", rec.Body.String())
	}
	if _, err := store.GetCredential(ctx, email); err == nil {
		t.Error("GetCredential() succeeded after a rejected too-short password")
	}
}
