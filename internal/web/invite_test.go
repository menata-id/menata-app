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
	"menata.app/internal/domain"
)

// inviteTestMachines is what admitInvitedMember needs to create the Workspace's own mch_user
// record on acceptance. It declares no name or email Field, which is the point: since 2026-09-22
// those live on the identity, and a record created for a new member carries only what is
// Workspace-scoped.
func inviteTestMachines() map[string]*domain.Machine {
	return map[string]*domain.Machine{
		domain.UserMachineID: {
			ID:   domain.UserMachineID,
			Name: "User",
			Fields: []domain.Field{
				{ID: "fld_weekly_capacity", Name: "Weekly Capacity (hours)", Type: domain.FieldTypeNumber},
			},
		},
	}
}

// inviteTestWorkspace creates a Workspace with a live invitation for email and returns both --
// the new model's setup, where an invitation is a pending_invites row and nothing else. No
// membership and no mch_user record exist until the invite is accepted.
func inviteTestWorkspace(t *testing.T, store *data.Store, name, slug, email string) *data.Workspace {
	t.Helper()
	ctx := context.Background()
	ws, err := store.CreateWorkspace(ctx, name, slug)
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, authTestPool(t), ws.ID, email)
	if err := store.CreatePendingInvite(ctx, data.PendingInvite{
		WorkspaceID: ws.ID, Email: email, WorkspaceRole: "member",
	}); err != nil {
		t.Fatalf("CreatePendingInvite: %v", err)
	}
	return ws
}

func postAcceptInvite(store *data.Store, cfg config.Config, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/accept-invite", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	submitAcceptInvite(inviteTestMachines(), store, cfg)(rec, req)
	return rec
}

// TestSubmitAcceptInvite_validTokenCreatesIdentityAndMembership is the mirror-image regression test
// of TestAuthenticateMember_invitedEmailWithNoCredentialIsRejected: the only place a credential may
// be created for an invited email is through a valid, workspace-bound invite token.
//
// Since 2026-09-22 it also asserts the other half of the owner's model: accepting is what *creates*
// the membership, and the full name the invitee typed lands on their identity rather than on the
// Workspace record.
func TestSubmitAcceptInvite_validTokenCreatesIdentityAndMembership(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: "accept-invite-test-secret", SecureCookies: false}
	ctx := context.Background()
	const email = "accept_invite_test@example.com"

	ws := inviteTestWorkspace(t, store, "Accept Invite Test", "accept-invite-test-workspace", email)

	// Nothing exists in the Workspace before acceptance -- that is the model, not a detail.
	if _, ok, err := resolveWorkspaceMembership(ctx, store, email, ws.ID); err != nil {
		t.Fatalf("resolveWorkspaceMembership: %v", err)
	} else if ok {
		t.Fatal("an invited email holds a membership before accepting -- an invitation is not a membership")
	}

	token := authorization.NewInviteToken(cfg.SessionSecret, email, ws.ID)
	rec := postAcceptInvite(store, cfg, "token="+token+"&full_name=Sri+Wahyuni&password=a-real-invite-password")

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
	if cred.FullName != "Sri Wahyuni" {
		t.Errorf("credential FullName = %q, want %q -- the name the invitee typed belongs to their identity", cred.FullName, "Sri Wahyuni")
	}

	// Membership now exists, and the invitation is spent.
	userRecordID, ok, err := resolveWorkspaceMembership(ctx, store, email, ws.ID)
	if err != nil {
		t.Fatalf("resolveWorkspaceMembership: %v", err)
	}
	if !ok {
		t.Fatal("accepting an invitation did not create a membership")
	}
	if _, err := store.GetPendingInvite(ctx, ws.ID, email); err == nil {
		t.Error("the invitation still exists after being accepted -- it must be consumed")
	}
	// The name is resolved from the identity, not stored on the record.
	names, err := store.MemberNames(ctx, ws.ID)
	if err != nil {
		t.Fatalf("MemberNames: %v", err)
	}
	if names[userRecordID] != "Sri Wahyuni" {
		t.Errorf("MemberNames[%s] = %q, want %q", userRecordID, names[userRecordID], "Sri Wahyuni")
	}
}

// TestSubmitAcceptInvite_missingFullNameDoesNotCreateIdentity guards the new-identity branch's own
// required field: a brand-new person must state their own name, because nobody else ever will.
func TestSubmitAcceptInvite_missingFullNameDoesNotCreateIdentity(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: "accept-invite-noname-test-secret", SecureCookies: false}
	ctx := context.Background()
	const email = "accept_invite_noname_test@example.com"

	ws := inviteTestWorkspace(t, store, "Accept Invite No Name Test", "accept-invite-noname-test-workspace", email)
	token := authorization.NewInviteToken(cfg.SessionSecret, email, ws.ID)
	rec := postAcceptInvite(store, cfg, "token="+token+"&password=a-real-invite-password")

	if rec.Code != http.StatusOK {
		t.Fatalf("submitAcceptInvite(no full name) status = %d, want 200 (re-rendered form); body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "full name is required") {
		t.Errorf("body missing the required-name message; body=%s", rec.Body.String())
	}
	if _, err := store.GetCredential(ctx, email); err == nil {
		t.Error("GetCredential() succeeded although no name was given")
	}
}

// TestSubmitAcceptInvite_alreadyAcceptedTokenIsRejected covers replay: a token, once used, must not
// be usable again. Under the pending-invite model the second attempt fails at the invitation
// itself -- it was consumed by the first accept -- which is a stronger stop than the old
// credential-level check it replaces.
func TestSubmitAcceptInvite_alreadyAcceptedTokenIsRejected(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: "accept-invite-replay-test-secret", SecureCookies: false}
	ctx := context.Background()
	const email = "accept_invite_replay_test@example.com"

	ws := inviteTestWorkspace(t, store, "Accept Invite Replay Test", "accept-invite-replay-test-workspace", email)
	token := authorization.NewInviteToken(cfg.SessionSecret, email, ws.ID)

	if rec := postAcceptInvite(store, cfg, "token="+token+"&full_name=Replay+Tester&password=first-real-password"); rec.Code != http.StatusSeeOther {
		t.Fatalf("first accept status = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}
	rec := postAcceptInvite(store, cfg, "token="+token+"&full_name=Someone+Else&password=an-attackers-second-password")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("replayed accept status = %d, want 400 (the invitation was consumed); body=%s", rec.Code, rec.Body.String())
	}
	if outcome := authenticateMember(ctx, store, email, "an-attackers-second-password"); outcome != loginRejected {
		t.Error("authenticateMember(attacker's replayed password) succeeded -- replay must not overwrite the credential")
	}
	if outcome := authenticateMember(ctx, store, email, "first-real-password"); outcome != loginOK {
		t.Error("authenticateMember(original password) failed after a replay -- the credential must not have changed")
	}
	cred, err := store.GetCredential(ctx, email)
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if cred.FullName != "Replay Tester" {
		t.Errorf("FullName = %q after a replay, want it unchanged at %q", cred.FullName, "Replay Tester")
	}
}

// TestSubmitAcceptInvite_wrongSecretTokenIsRejected covers a tampered/forged token.
func TestSubmitAcceptInvite_wrongSecretTokenIsRejected(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: "accept-invite-tamper-test-secret", SecureCookies: false}
	ctx := context.Background()
	const email = "accept_invite_tamper_test@example.com"

	ws := inviteTestWorkspace(t, store, "Accept Invite Tamper Test", "accept-invite-tamper-test-workspace", email)
	forged := authorization.NewInviteToken("a-different-secret", email, ws.ID)
	rec := postAcceptInvite(store, cfg, "token="+forged+"&full_name=Forger&password=whatever-password")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("submitAcceptInvite(wrong-secret token) status = %d, want 400", rec.Code)
	}
	if _, err := store.GetCredential(ctx, email); err == nil {
		t.Error("GetCredential() succeeded after a rejected forged token, want ErrCredentialNotFound still")
	}
}

// TestSubmitAcceptInvite_revokedInvitationIsRejected covers the case the workspaceID binding exists
// for: a token that was valid when sent, but whose invitation has since been revoked by an admin,
// must not still be honorable just because the token itself has not expired.
func TestSubmitAcceptInvite_revokedInvitationIsRejected(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: "accept-invite-revoked-test-secret", SecureCookies: false}
	ctx := context.Background()
	const email = "accept_invite_revoked_test@example.com"

	ws := inviteTestWorkspace(t, store, "Accept Invite Revoked Test", "accept-invite-revoked-test-workspace", email)
	token := authorization.NewInviteToken(cfg.SessionSecret, email, ws.ID)
	if err := store.DeletePendingInvite(ctx, ws.ID, email); err != nil {
		t.Fatalf("DeletePendingInvite: %v", err)
	}

	rec := postAcceptInvite(store, cfg, "token="+token+"&full_name=Revoked+Person&password=whatever-password")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("submitAcceptInvite(revoked invitation) status = %d, want 400", rec.Code)
	}
	if _, err := store.GetCredential(ctx, email); err == nil {
		t.Error("GetCredential() succeeded although the invitation had been revoked")
	}
	if _, ok, err := resolveWorkspaceMembership(ctx, store, email, ws.ID); err != nil {
		t.Fatalf("resolveWorkspaceMembership: %v", err)
	} else if ok {
		t.Error("a revoked invitation still produced a membership")
	}
}

// TestSubmitAcceptInvite_existingCredentialVerifiesExistingPassword is the regression test for the
// CAP-O10 follow-up (writing-guide-reference.md §7): an email invited to a second Workspace while
// already holding a credential elsewhere accepts by confirming that existing password -- and is
// never asked for its name again, because the identity already carries one.
func TestSubmitAcceptInvite_existingCredentialVerifiesExistingPassword(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: "accept-invite-existing-cred-test-secret", SecureCookies: false}
	ctx := context.Background()
	const email = "accept_invite_existing_cred_test@example.com"

	existingHash, err := authorization.HashPassword("the-existing-real-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := store.CreateCredential(ctx, email, "Dewi Lestari", existingHash, true); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM credentials WHERE email = $1`, email)
	})

	ws := inviteTestWorkspace(t, store, "Accept Invite Existing Cred Test", "accept-invite-existing-cred-test-workspace", email)
	token := authorization.NewInviteToken(cfg.SessionSecret, email, ws.ID)
	rec := postAcceptInvite(store, cfg, "token="+token+"&password=the-existing-real-password")

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
	if cred.FullName != "Dewi Lestari" {
		t.Errorf("FullName = %q, want it untouched at %q -- joining a Workspace must not restate an identity's name", cred.FullName, "Dewi Lestari")
	}

	// The name carried into the new Workspace without being typed there: the whole point of it
	// living on the identity.
	userRecordID, ok, err := resolveWorkspaceMembership(ctx, store, email, ws.ID)
	if err != nil || !ok {
		t.Fatalf("resolveWorkspaceMembership: %v, ok=%v", err, ok)
	}
	names, err := store.MemberNames(ctx, ws.ID)
	if err != nil {
		t.Fatalf("MemberNames: %v", err)
	}
	if names[userRecordID] != "Dewi Lestari" {
		t.Errorf("MemberNames[%s] = %q, want %q", userRecordID, names[userRecordID], "Dewi Lestari")
	}
}

// TestSubmitAcceptInvite_existingCredentialWrongPasswordRejected is the negative case: a wrong
// guess is rejected with a specific message (the token itself is valid here, only the password is
// wrong -- a different situation from an invalid/expired/revoked invitation), and creates nothing.
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
	if err := store.CreateCredential(ctx, email, "Dewi Lestari", existingHash, true); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM credentials WHERE email = $1`, email)
	})

	ws := inviteTestWorkspace(t, store, "Accept Invite Existing Cred Wrong Test", "accept-invite-existing-cred-wrong-test-workspace", email)
	token := authorization.NewInviteToken(cfg.SessionSecret, email, ws.ID)
	rec := postAcceptInvite(store, cfg, "token="+token+"&password=a-wrong-guess")

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
	// A failed password check must not admit anyone: no membership, and the invitation survives
	// so the real invitee can still use it.
	if _, ok, err := resolveWorkspaceMembership(ctx, store, email, ws.ID); err != nil {
		t.Fatalf("resolveWorkspaceMembership: %v", err)
	} else if ok {
		t.Error("a wrong-password accept attempt created a membership")
	}
	if _, err := store.GetPendingInvite(ctx, ws.ID, email); err != nil {
		t.Errorf("the invitation was consumed by a failed attempt: %v", err)
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

	ws := inviteTestWorkspace(t, store, "Accept Invite Short Test", "accept-invite-short-test-workspace", email)
	token := authorization.NewInviteToken(cfg.SessionSecret, email, ws.ID)
	rec := postAcceptInvite(store, cfg, "token="+token+"&full_name=Short+Password&password=short")

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
