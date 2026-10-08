package data

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrSoleWorkspaceAdmin is returned when anonymizing an identity would leave a live Workspace with
// no active admin. SoleAdminOf says which, so the page can name them.
var ErrSoleWorkspaceAdmin = errors.New("identity is the only active admin of a workspace")

// DeletedUserName is what a name resolves to once its owner has deleted their account. It is the
// credential's own full_name, so every screen that already resolves a person through
// credentials (MemberNames, viewerNameFor) shows it with no code of its own.
const DeletedUserName = "Deleted user"

// DeletedEmailDomain is the reserved ".invalid" TLD (RFC 2606): an anonymized login can never
// receive mail or collide with a real address.
const DeletedEmailDomain = "deleted.invalid"

// soleAdminSQL selects the live (non-archived) Workspaces in which $1 is an active admin and no
// other active admin exists. One statement, used both as the pre-check and again inside the
// anonymizing transaction, so a second admin being removed between the two cannot slip through.
const soleAdminSQL = `
	SELECT w.name
	FROM workspace_members wm
	JOIN workspaces w ON w.id = wm.workspace_id
	WHERE wm.email = $1 AND wm.workspace_role = 'admin' AND wm.deactivated_at IS NULL AND NOT w.archived
	  AND NOT EXISTS (
	      SELECT 1 FROM workspace_members o
	      WHERE o.workspace_id = wm.workspace_id AND o.user_record_id <> wm.user_record_id
	        AND o.workspace_role = 'admin' AND o.deactivated_at IS NULL)
	ORDER BY w.name`

// SoleAdminOf names the Workspaces email cannot leave without orphaning them.
func (s *Store) SoleAdminOf(ctx context.Context, email string) ([]string, error) {
	readLogFrom(ctx).record("sole admin workspaces")
	return soleAdminNames(ctx, s.pool, email)
}

type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func soleAdminNames(ctx context.Context, q queryer, email string) ([]string, error) {
	rows, err := q.Query(ctx, soleAdminSQL, email)
	if err != nil {
		return nil, fmt.Errorf("sole admin of: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan sole admin workspace: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// AnonymizeIdentity is account deletion (Google Play's account-deletion policy; 2026-10-08, owner
// decision "anonimkan"). It removes the login and everything that identifies the person, and keeps
// the work: records they authored and the name snapshots stored on decisions stay, because an
// approval history that vanishes with its approver is not a history. Their mch_user records stay
// too, as inert rows that now resolve to DeletedUserName.
//
// In one transaction: the credential is renamed to an unreachable address, stripped of its name and
// password hash, and its notification flags cleared; every membership follows the new address and
// is deactivated; direct and group role grants go; invitations addressed to the old address go; AI
// conversation turns (free text the person typed) go; and every session subject is bumped so a
// cookie issued before this cannot be replayed.
//
// The old address is free afterwards: registering it again creates a new identity with none of this
// history, which is what "delete my account" means to the person asking.
//
// Returns the mch_user record ids the identity held, so a caller has the list it needs for
// anything keyed by record id that this package does not own (stored signature images).
func (s *Store) AnonymizeIdentity(ctx context.Context, email string) ([]string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("anonymize identity: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var locked string
	if err := tx.QueryRow(ctx, `SELECT email FROM credentials WHERE email = $1 FOR UPDATE`, email).Scan(&locked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCredentialNotFound
		}
		return nil, fmt.Errorf("anonymize identity: lock credential: %w", err)
	}
	sole, err := soleAdminNames(ctx, tx, email)
	if err != nil {
		return nil, err
	}
	if len(sole) > 0 {
		return nil, ErrSoleWorkspaceAdmin
	}

	suffix := make([]byte, 12)
	if _, err := rand.Read(suffix); err != nil {
		return nil, fmt.Errorf("anonymize identity: random address: %w", err)
	}
	anon := "deleted-" + hex.EncodeToString(suffix) + "@" + DeletedEmailDomain

	rows, err := tx.Query(ctx, `SELECT user_record_id FROM workspace_members WHERE email = $1`, email)
	if err != nil {
		return nil, fmt.Errorf("anonymize identity: list memberships: %w", err)
	}
	var recordIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("anonymize identity: scan membership: %w", err)
		}
		recordIDs = append(recordIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("anonymize identity: list memberships: %w", err)
	}

	steps := []struct {
		what string
		sql  string
		args []any
	}{
		{"direct roles", `DELETE FROM workspace_member_app_roles WHERE user_record_id = ANY($1)`, []any{recordIDs}},
		{"group membership", `DELETE FROM workspace_group_members WHERE user_record_id = ANY($1)`, []any{recordIDs}},
		{"invitations", `DELETE FROM pending_invites WHERE email = $1`, []any{email}},
		{"ai turns", `DELETE FROM ai_session_turns WHERE session_id IN (SELECT id FROM ai_sessions WHERE actor_user_id = ANY($1))`, []any{recordIDs}},
		{"memberships", `UPDATE workspace_members SET email = $2, deactivated_at = COALESCE(deactivated_at, now()) WHERE email = $1`, []any{email, anon}},
		{"credential", `UPDATE credentials SET email = $2, full_name = $3, password_hash = '', email_verified = false,
			notify_assigned = false, notify_decided = false, notify_sla_breach = false WHERE email = $1`, []any{email, anon, DeletedUserName}},
	}
	for _, st := range steps {
		if _, err := tx.Exec(ctx, st.sql, st.args...); err != nil {
			return nil, fmt.Errorf("anonymize identity: %s: %w", st.what, err)
		}
	}
	for _, id := range recordIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO session_generations (subject, generation) VALUES ($1, 1)
			ON CONFLICT (subject) DO UPDATE SET generation = session_generations.generation + 1`, id); err != nil {
			return nil, fmt.Errorf("anonymize identity: sessions: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("anonymize identity: commit: %w", err)
	}
	return recordIDs, nil
}
