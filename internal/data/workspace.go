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
// WorkspaceRole gates the Workspace itself (admin/member), while AppRoles says what they may be
// *within each Application*, keyed by Application id.
//
// An Application absent from the map means no role there -- "none" is the absence of a row, not a
// stored empty string, so "has no role here" and "has a blank role" cannot become two states
// meaning the same thing.
//
// AppRole is the pre-Fase-3b single-Application column, kept and still written alongside AppRoles
// until migration 008's own note says the column may be dropped. Read AppRoles; AppRole exists so
// a rollback loses nothing.
type Membership struct {
	WorkspaceID   string
	UserRecordID  string
	Email         string
	WorkspaceRole string
	AppRole       string
	AppRoles      map[string]string
}

// appRolesFor reads the per-Application roles of one member.
func (s *Store) appRolesFor(ctx context.Context, workspaceID, userRecordID string) (map[string]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT application_id, role FROM workspace_member_app_roles
		WHERE workspace_id = $1 AND user_record_id = $2
	`, workspaceID, userRecordID)
	if err != nil {
		return nil, fmt.Errorf("list member app roles: %w", err)
	}
	defer rows.Close()

	roles := map[string]string{}
	for rows.Next() {
		var appID, role string
		if err := rows.Scan(&appID, &role); err != nil {
			return nil, fmt.Errorf("scan member app role: %w", err)
		}
		roles[appID] = role
	}
	return roles, rows.Err()
}

// appRolesByMember reads every member's roles for one Workspace in a single query, keyed by
// user record id. ListMembers already returns every member, so asking per member would be a
// self-inflicted N+1 -- this is read once and stitched in Go instead.
func (s *Store) appRolesByMember(ctx context.Context, workspaceID string) (map[string]map[string]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT user_record_id, application_id, role FROM workspace_member_app_roles
		WHERE workspace_id = $1
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list workspace app roles: %w", err)
	}
	defer rows.Close()

	byMember := map[string]map[string]string{}
	for rows.Next() {
		var userRecordID, appID, role string
		if err := rows.Scan(&userRecordID, &appID, &role); err != nil {
			return nil, fmt.Errorf("scan workspace app role: %w", err)
		}
		if byMember[userRecordID] == nil {
			byMember[userRecordID] = map[string]string{}
		}
		byMember[userRecordID][appID] = role
	}
	return byMember, rows.Err()
}

// SetMemberAppRole assigns (or clears) one member's role in one Application. An empty role
// DELETEs the row rather than storing "": absence is how "no role here" is said, so the two can
// never drift into separate states meaning the same thing.
func (s *Store) SetMemberAppRole(ctx context.Context, workspaceID, userRecordID, applicationID, role string) error {
	if role == "" {
		_, err := s.pool.Exec(ctx, `
			DELETE FROM workspace_member_app_roles
			WHERE workspace_id = $1 AND user_record_id = $2 AND application_id = $3
		`, workspaceID, userRecordID, applicationID)
		if err != nil {
			return fmt.Errorf("clear member app role: %w", err)
		}
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO workspace_member_app_roles (workspace_id, user_record_id, application_id, role)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (workspace_id, user_record_id, application_id) DO UPDATE SET role = EXCLUDED.role
	`, workspaceID, userRecordID, applicationID, role)
	if err != nil {
		return fmt.Errorf("set member app role: %w", err)
	}
	return nil
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
	roles, err := s.appRolesFor(ctx, workspaceID, userRecordID)
	if err != nil {
		return nil, err
	}
	m.AppRoles = roles
	return m, nil
}

// ListMemberships returns every Workspace email belongs to -- login's own source of truth for
// whether a signed-in identity has exactly one Workspace (skip straight in) or several (Choose
// Workspace, ROADMAP.md Phase 21 Step 4).
//
// AppRoles is deliberately left nil here, unlike GetMembership/ListMembers: its two callers show
// a Workspace name and Workspace role only, and filling it would mean querying per-Application
// roles across every Workspace an identity belongs to for data no screen reads.
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
	if err := rows.Err(); err != nil {
		return nil, err
	}

	byMember, err := s.appRolesByMember(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for i := range memberships {
		if roles := byMember[memberships[i].UserRecordID]; roles != nil {
			memberships[i].AppRoles = roles
		} else {
			memberships[i].AppRoles = map[string]string{}
		}
	}
	return memberships, nil
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
