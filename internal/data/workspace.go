package data

import (
	"context"
	"errors"
	"fmt"
	"time"

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
	// Archived and ArchivedAt are the Workspace lifecycle (Flow 2 gap study Tahap 7,
	// migrations/013_workspace_archive.sql). ArchivedAt is nil for a live Workspace, and cleared
	// (not historized) on restore -- see the migration's own doc comment for why this is the
	// moment of the *current* archival, not a log of every archive/restore cycle.
	Archived   bool
	ArchivedAt *time.Time
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
// names a workspace_id, not a display name) and -- since migrations/013_workspace_archive.sql --
// for resolveIdentity's own eager read, which is what lets blockWritesToArchivedWorkspace check
// Archived at zero extra query cost: this is already fetched once per request.
func (s *Store) GetWorkspace(ctx context.Context, id string) (*Workspace, error) {
	readLogFrom(ctx).record("workspace by id")
	w := &Workspace{ID: id}
	err := s.pool.QueryRow(ctx, `SELECT name, slug, archived, archived_at FROM workspaces WHERE id = $1`, id).
		Scan(&w.Name, &w.Slug, &w.Archived, &w.ArchivedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("get workspace: %w", err)
	}
	return w, nil
}

// ArchiveWorkspace marks a Workspace read-only and hidden from its ordinary members (Flow 2 gap
// study Tahap 7) -- the Danger Zone action, taken from *inside* the Workspace being archived
// (requireWorkspaceAdmin on the current ctx scope; see submitArchiveWorkspace).
func (s *Store) ArchiveWorkspace(ctx context.Context, id string) error {
	ct, err := s.pool.Exec(ctx, `UPDATE workspaces SET archived = true, archived_at = now() WHERE id = $1 AND archived = false`, id)
	if err != nil {
		return fmt.Errorf("archive workspace: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrRecordNotFound
	}
	return nil
}

// RestoreWorkspace reverses ArchiveWorkspace -- taken from *outside* the Workspace being restored
// (Choose Workspace's own "Archived workspaces" list, per-target-workspace admin check; see
// restoreWorkspaceIfAdmin), since an archived Workspace's own read-only gate would otherwise make
// restoring it from within impossible.
func (s *Store) RestoreWorkspace(ctx context.Context, id string) error {
	ct, err := s.pool.Exec(ctx, `UPDATE workspaces SET archived = false, archived_at = NULL WHERE id = $1 AND archived = true`, id)
	if err != nil {
		return fmt.Errorf("restore workspace: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrRecordNotFound
	}
	return nil
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
	// Groups are the Groups this member belongs to, each carrying its own per-Application grants
	// (Fase 4). Filled by GetMembership and ListMembers; ListMemberships leaves it nil for the
	// same reason it leaves AppRoles nil.
	//
	// AppRoles stays single-valued per Application. A member holding several roles in one
	// Application -- via two Groups, or direct plus a Group -- is produced by EffectiveRoles at
	// read time, never stored; see its doc comment and migration 009's own note.
	Groups []Group
	// WorkspaceName is the display name of the Workspace this membership names. Filled by
	// ListMemberships, which joins it; empty from GetMembership and ListMembers, whose callers
	// already know which Workspace they are in and would be paying for a join to be told.
	//
	// It lives here rather than being fetched per row because fetching it per row is precisely
	// what it replaced -- see ListMemberships' own doc comment.
	WorkspaceName string
	// Archived/ArchivedAt ride along the same join, for Choose Workspace's own live/archived split
	// (Flow 2 gap study Tahap 7) -- filled by ListMemberships only, same as WorkspaceName.
	Archived   bool
	ArchivedAt *time.Time
}

// appRolesFor reads the per-Application roles of one member.
func (s *Store) appRolesFor(ctx context.Context, workspaceID, userRecordID string) (map[string]string, error) {
	readLogFrom(ctx).record("member app roles")
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
	readLogFrom(ctx).record("member app roles (all)")
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

// WorkspaceBySlug finds a Workspace by the slug a metadata manifest names it with
// (metadata/workspaces/<slug>.yaml, 2026-09-22). The slug is what a person can write down; the
// `ws_...` id is generated when the Workspace is created and is not.
func (s *Store) WorkspaceBySlug(ctx context.Context, slug string) (*Workspace, error) {
	ws := &Workspace{Slug: slug}
	readLogFrom(ctx).record("workspace by slug")
	err := s.pool.QueryRow(ctx, `SELECT id, name FROM workspaces WHERE slug = $1`, slug).Scan(&ws.ID, &ws.Name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("workspace by slug: %w", err)
	}
	return ws, nil
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
	readLogFrom(ctx).record("membership")
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
	// GroupsForMember, not GroupsByMember: this method is about one person, and GroupsByMember
	// reads every Group and every group membership in the Workspace to answer that. It was the
	// larger half of GetMembership's four queries, on a method every authenticated request reaches.
	groups, err := s.GroupsForMember(ctx, workspaceID, userRecordID)
	if err != nil {
		return nil, err
	}
	m.Groups = groups
	return m, nil
}

// ListMemberships returns every Workspace email belongs to -- login's own source of truth for
// whether a signed-in identity has exactly one Workspace (skip straight in) or several (Choose
// Workspace, ROADMAP.md Phase 21 Step 4).
//
// AppRoles is deliberately left nil here, unlike GetMembership/ListMembers: its callers show a
// Workspace name and Workspace role only, and filling it would mean querying per-Application
// roles across every Workspace an identity belongs to for data no screen reads.
//
// WorkspaceName comes back on the join rather than being fetched per row, and that is the whole
// point of the join being here. Choose Workspace needs a name per membership, and
// loadWorkspaceChoices used to get it by calling GetWorkspace once per membership -- **the only
// true N+1 the 2026-09-22 query audit found**. It measured as a single repeated read because the
// identity it was measured with belonged to one Workspace; someone in five paid for six queries.
// A count that only looks wrong on data nobody has yet is exactly the kind this app cannot wait
// to be told about, so it is closed by shape rather than by threshold.
func (s *Store) ListMemberships(ctx context.Context, email string) ([]Membership, error) {
	readLogFrom(ctx).record("memberships by email")
	rows, err := s.pool.Query(ctx, `
		SELECT wm.workspace_id, wm.user_record_id, wm.email, wm.workspace_role,
		       COALESCE(wm.app_role, ''), w.name, w.archived, w.archived_at
		FROM workspace_members wm
		JOIN workspaces w ON w.id = wm.workspace_id
		WHERE wm.email = $1
		ORDER BY wm.workspace_id
	`, email)
	if err != nil {
		return nil, fmt.Errorf("list memberships: %w", err)
	}
	defer rows.Close()

	var memberships []Membership
	for rows.Next() {
		var m Membership
		if err := rows.Scan(&m.WorkspaceID, &m.UserRecordID, &m.Email, &m.WorkspaceRole, &m.AppRole, &m.WorkspaceName, &m.Archived, &m.ArchivedAt); err != nil {
			return nil, fmt.Errorf("scan membership: %w", err)
		}
		memberships = append(memberships, m)
	}
	return memberships, rows.Err()
}

// ListMembers returns every member of workspaceID, for the Workspace Members screen (ROADMAP.md
// Phase 21 Step 6).
func (s *Store) ListMembers(ctx context.Context, workspaceID string) ([]Membership, error) {
	groups, err := s.ListGroups(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return s.ListMembersFrom(ctx, workspaceID, groups)
}

// ListMembersFrom is ListMembers over a Group list the caller already holds. Same reasoning as
// GroupsByMemberFrom: a request that needs the Workspace's Groups for something else as well
// should read them once and pass them in, rather than have this method read them again.
func (s *Store) ListMembersFrom(ctx context.Context, workspaceID string, groups []Group) ([]Membership, error) {
	readLogFrom(ctx).record("members")
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
	groupsByMember, err := s.GroupsByMemberFrom(ctx, workspaceID, groups)
	if err != nil {
		return nil, err
	}
	for i := range memberships {
		if roles := byMember[memberships[i].UserRecordID]; roles != nil {
			memberships[i].AppRoles = roles
		} else {
			memberships[i].AppRoles = map[string]string{}
		}
		memberships[i].Groups = groupsByMember[memberships[i].UserRecordID]
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
	readLogFrom(ctx).record("mch_user workspace by id")
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
