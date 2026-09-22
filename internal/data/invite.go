package data

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrPendingInviteNotFound is returned when no unaccepted invitation matches a Workspace/email
// pair -- which is how acceptance tells "this link is still live" from "this was already accepted,
// revoked, or never sent".
var ErrPendingInviteNotFound = errors.New("pending invite not found")

// PendingInvite is an invitation that has been sent but not yet accepted (migration 011).
//
// It is deliberately NOT a membership: membership begins when the invitation is accepted, so
// nobody who has agreed to nothing appears in a member list, in an approver picker, or in any
// authorization lookup. AppRoles carries the per-Application roles the inviting admin chose,
// parked here until there is a member to attach them to.
type PendingInvite struct {
	WorkspaceID   string
	Email         string
	WorkspaceRole string
	AppRoles      map[string]string
}

// CreatePendingInvite records an invitation, replacing any earlier unaccepted one for the same
// Workspace/email pair -- re-inviting someone is a correction (often of the role), not a second
// invitation, and the emailed token carries no role of its own, so the newest choice is simply
// the one that applies when they accept.
func (s *Store) CreatePendingInvite(ctx context.Context, inv PendingInvite) error {
	appRoles := inv.AppRoles
	if appRoles == nil {
		appRoles = map[string]string{}
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO pending_invites (workspace_id, email, workspace_role, app_roles)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (workspace_id, email)
		DO UPDATE SET workspace_role = EXCLUDED.workspace_role,
		              app_roles = EXCLUDED.app_roles,
		              created_at = NOW()
	`, inv.WorkspaceID, inv.Email, inv.WorkspaceRole, appRoles)
	if err != nil {
		return fmt.Errorf("create pending invite: %w", err)
	}
	return nil
}

// GetPendingInvite returns one unaccepted invitation, or ErrPendingInviteNotFound.
func (s *Store) GetPendingInvite(ctx context.Context, workspaceID, email string) (*PendingInvite, error) {
	inv := &PendingInvite{WorkspaceID: workspaceID, Email: email}
	err := s.pool.QueryRow(ctx, `
		SELECT workspace_role, app_roles
		FROM pending_invites
		WHERE workspace_id = $1 AND email = $2
	`, workspaceID, email).Scan(&inv.WorkspaceRole, &inv.AppRoles)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPendingInviteNotFound
		}
		return nil, fmt.Errorf("get pending invite: %w", err)
	}
	return inv, nil
}

// ListPendingInvites returns every unaccepted invitation into workspaceID, for the "Waiting to
// accept" section of the Workspace Members screen -- shown separately from members, because that
// is exactly what they are not yet.
func (s *Store) ListPendingInvites(ctx context.Context, workspaceID string) ([]PendingInvite, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT email, workspace_role, app_roles
		FROM pending_invites
		WHERE workspace_id = $1
		ORDER BY created_at
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list pending invites: %w", err)
	}
	defer rows.Close()

	var invites []PendingInvite
	for rows.Next() {
		inv := PendingInvite{WorkspaceID: workspaceID}
		if err := rows.Scan(&inv.Email, &inv.WorkspaceRole, &inv.AppRoles); err != nil {
			return nil, fmt.Errorf("scan pending invite: %w", err)
		}
		invites = append(invites, inv)
	}
	return invites, rows.Err()
}

// DeletePendingInvite removes an invitation, whether because it was just accepted (the membership
// it described now exists) or because an admin revoked it. Silent on a missing row: both callers
// are removing something they want gone, and it already is.
func (s *Store) DeletePendingInvite(ctx context.Context, workspaceID, email string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM pending_invites WHERE workspace_id = $1 AND email = $2`, workspaceID, email); err != nil {
		return fmt.Errorf("delete pending invite: %w", err)
	}
	return nil
}
