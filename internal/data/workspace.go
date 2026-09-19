package data

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Workspace is a real, storage-backed Workspace (ROADMAP.md Phase 21 Step 3) -- distinct from
// domain.Workspace, which is Phase 2's metadata-only, exactly-one-hardcoded-instance type. A
// Workspace created here is what registration produces; every record a Store scoped to its id
// creates or reads belongs to it (WithWorkspace).
type Workspace struct {
	ID   string
	Name string
	Slug string
}

// CreateWorkspace inserts a new Workspace, retrying with a numeric suffix on a slug collision
// (the sole UNIQUE constraint that can fail here) rather than asking the caller to pre-resolve
// one -- the same posture as newRecordID's random id, just for a human-chosen value that can
// collide.
func (s *Store) CreateWorkspace(ctx context.Context, name, baseSlug string) (*Workspace, error) {
	const maxAttempts = 20
	for attempt := 0; attempt < maxAttempts; attempt++ {
		slug := baseSlug
		if attempt > 0 {
			slug = fmt.Sprintf("%s-%d", baseSlug, attempt+1)
		}
		w := &Workspace{ID: newID("ws_"), Name: name, Slug: slug}
		_, err := s.pool.Exec(ctx, `INSERT INTO workspaces (id, name, slug) VALUES ($1, $2, $3)`, w.ID, w.Name, w.Slug)
		if err == nil {
			return w, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	return nil, fmt.Errorf("create workspace: no unique slug found for %q after %d attempts", baseSlug, maxAttempts)
}

// GetWorkspace returns one Workspace by id, for Choose Workspace's own labels (a membership row
// names a workspace_id, not a display name).
func (s *Store) GetWorkspace(ctx context.Context, id string) (*Workspace, error) {
	w := &Workspace{ID: id}
	err := s.pool.QueryRow(ctx, `SELECT name, slug FROM workspaces WHERE id = $1`, id).Scan(&w.Name, &w.Slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("get workspace: %w", err)
	}
	return w, nil
}

// Membership is one identity's role within one Workspace (ROADMAP.md Phase 21 Step 3/8):
// WorkspaceRole gates the Workspace itself (admin/member); AppRole is this slice's single-
// Application role vocabulary (approver/submitter/reviewer/none), consulted only for a member --
// an admin's access does not depend on it.
type Membership struct {
	WorkspaceID   string
	UserRecordID  string
	Email         string
	WorkspaceRole string
	AppRole       string
}

// AddMember records email's membership in workspaceID, naming the mch_user record (created in
// that Workspace's own scoped Store) that represents them there.
func (s *Store) AddMember(ctx context.Context, workspaceID, userRecordID, email, workspaceRole, appRole string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO workspace_members (workspace_id, user_record_id, email, workspace_role, app_role)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''))
	`, workspaceID, userRecordID, email, workspaceRole, appRole)
	if err != nil {
		return fmt.Errorf("add member: %w", err)
	}
	return nil
}

// GetMembership returns one identity's membership in one Workspace, or ErrRecordNotFound -- the
// Workspace Home / "Your access" panel's own lookup (ROADMAP.md Phase 21 Step 5), given a
// userRecordID rather than an email (the caller usually only has the former, from an already-
// resolved session).
func (s *Store) GetMembership(ctx context.Context, workspaceID, userRecordID string) (*Membership, error) {
	m := &Membership{WorkspaceID: workspaceID, UserRecordID: userRecordID}
	err := s.pool.QueryRow(ctx, `
		SELECT email, workspace_role, COALESCE(app_role, '')
		FROM workspace_members
		WHERE workspace_id = $1 AND user_record_id = $2
	`, workspaceID, userRecordID).Scan(&m.Email, &m.WorkspaceRole, &m.AppRole)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("get membership: %w", err)
	}
	return m, nil
}

// ListMemberships returns every Workspace email belongs to -- login's own source of truth for
// whether a signed-in identity has exactly one Workspace (skip straight in) or several (Choose
// Workspace, ROADMAP.md Phase 21 Step 4).
func (s *Store) ListMemberships(ctx context.Context, email string) ([]Membership, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT workspace_id, user_record_id, email, workspace_role, COALESCE(app_role, '')
		FROM workspace_members
		WHERE email = $1
		ORDER BY workspace_id
	`, email)
	if err != nil {
		return nil, fmt.Errorf("list memberships: %w", err)
	}
	defer rows.Close()

	var memberships []Membership
	for rows.Next() {
		var m Membership
		if err := rows.Scan(&m.WorkspaceID, &m.UserRecordID, &m.Email, &m.WorkspaceRole, &m.AppRole); err != nil {
			return nil, fmt.Errorf("scan membership: %w", err)
		}
		memberships = append(memberships, m)
	}
	return memberships, rows.Err()
}

// ListMembers returns every member of workspaceID, for the Workspace Members screen (ROADMAP.md
// Phase 21 Step 6).
func (s *Store) ListMembers(ctx context.Context, workspaceID string) ([]Membership, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT workspace_id, user_record_id, email, workspace_role, COALESCE(app_role, '')
		FROM workspace_members
		WHERE workspace_id = $1
		ORDER BY email
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()

	var memberships []Membership
	for rows.Next() {
		var m Membership
		if err := rows.Scan(&m.WorkspaceID, &m.UserRecordID, &m.Email, &m.WorkspaceRole, &m.AppRole); err != nil {
			return nil, fmt.Errorf("scan membership: %w", err)
		}
		memberships = append(memberships, m)
	}
	return memberships, rows.Err()
}

// UpdateMemberRole changes a member's WorkspaceRole/AppRole (ROADMAP.md Phase 21 Step 6).
func (s *Store) UpdateMemberRole(ctx context.Context, workspaceID, userRecordID, workspaceRole, appRole string) error {
	ct, err := s.pool.Exec(ctx, `
		UPDATE workspace_members
		SET workspace_role = $3, app_role = NULLIF($4, '')
		WHERE workspace_id = $1 AND user_record_id = $2
	`, workspaceID, userRecordID, workspaceRole, appRole)
	if err != nil {
		return fmt.Errorf("update member role: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrRecordNotFound
	}
	return nil
}

// ResolveUserWorkspace returns the Workspace a real mch_user record belongs to, by its record id
// alone -- deliberately unscoped (records scoped by workspace_id and asked for by id from a
// different Workspace would find nothing), since this is the one lookup that must run *before* a
// request's own Workspace is known: an incoming session cookie names a user id, not a Workspace,
// so resolving which Workspace it belongs to is the auth middleware's very first step
// (ROADMAP.md Phase 21 Step 4).
func (s *Store) ResolveUserWorkspace(ctx context.Context, userRecordID string) (string, error) {
	var workspaceID string
	err := s.pool.QueryRow(ctx, `
		SELECT workspace_id FROM records WHERE machine_id = 'mch_user' AND id = $1
	`, userRecordID).Scan(&workspaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrRecordNotFound
		}
		return "", fmt.Errorf("resolve user workspace: %w", err)
	}
	return workspaceID, nil
}
