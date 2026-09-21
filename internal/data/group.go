package data

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
)

// Group is a named set of Workspace members that can hold Application roles of its own
// (migration 009, mirroring menata-runtime's CAP-O07). Assigning a role to a Group once and then
// managing its membership replaces editing every person's own row.
type Group struct {
	ID          string
	Name        string
	MemberCount int
	// Grants is this Group's role per Application, keyed by Application id -- at most one each,
	// which the table's own primary key enforces.
	Grants map[string]string
}

// EffectiveRoles merges a member's direct role per Application with every role their Groups hold
// there, producing the role *set* per Application.
//
// This is the one place the union rule is expressed, and it is deliberately a pure function with
// no database in sight: CAP-O07 defines effective access as "the union of their direct
// assignment and every role any Group they belong to holds there -- holding a role through either
// path grants it identically", and upstream's own schema comment adds that the set is produced by
// a read-time merge, "not this table". So neither storage shape is multi-valued: a member has one
// direct role per Application (migration 008) and a Group has one per Application (009), and the
// set only ever exists here.
//
// Roles are returned in a stable order -- direct first, then group-granted in the order the
// Groups were given -- so a page's output does not reshuffle between renders. Duplicates collapse:
// holding "approver" directly and through a Group is one role, since either path grants it
// identically.
func EffectiveRoles(direct map[string]string, groups []Group) map[string][]string {
	effective := map[string][]string{}
	add := func(appID, role string) {
		if role == "" {
			return
		}
		if slices.Contains(effective[appID], role) {
			return
		}
		effective[appID] = append(effective[appID], role)
	}
	for appID, role := range direct {
		add(appID, role)
	}
	for _, g := range groups {
		for appID, role := range g.Grants {
			add(appID, role)
		}
	}
	return effective
}

// ListGroups returns every Group in workspaceID, with its member count and grants.
func (s *Store) ListGroups(ctx context.Context, workspaceID string) ([]Group, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT g.id, g.name, count(m.user_record_id)
		FROM workspace_groups g
		LEFT JOIN workspace_group_members m ON m.group_id = g.id
		WHERE g.workspace_id = $1
		GROUP BY g.id, g.name
		ORDER BY g.name
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.MemberCount); err != nil {
			return nil, fmt.Errorf("scan group: %w", err)
		}
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return s.attachGrants(ctx, workspaceID, groups)
}

// attachGrants fills Grants for every group in one query rather than one per group -- the same
// N+1 avoidance ListMembers already applies to member roles.
func (s *Store) attachGrants(ctx context.Context, workspaceID string, groups []Group) ([]Group, error) {
	if len(groups) == 0 {
		return groups, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT r.group_id, r.application_id, r.role
		FROM workspace_group_app_roles r
		JOIN workspace_groups g ON g.id = r.group_id
		WHERE g.workspace_id = $1
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list group grants: %w", err)
	}
	defer rows.Close()

	byGroup := map[string]map[string]string{}
	for rows.Next() {
		var groupID, appID, role string
		if err := rows.Scan(&groupID, &appID, &role); err != nil {
			return nil, fmt.Errorf("scan group grant: %w", err)
		}
		if byGroup[groupID] == nil {
			byGroup[groupID] = map[string]string{}
		}
		byGroup[groupID][appID] = role
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range groups {
		if g := byGroup[groups[i].ID]; g != nil {
			groups[i].Grants = g
		} else {
			groups[i].Grants = map[string]string{}
		}
	}
	return groups, nil
}

// GetGroup returns one Group, or ErrRecordNotFound. workspaceID is part of the lookup, not just a
// filter: a group id from another Workspace must not resolve here.
func (s *Store) GetGroup(ctx context.Context, workspaceID, groupID string) (*Group, error) {
	g := &Group{ID: groupID}
	err := s.pool.QueryRow(ctx, `
		SELECT g.name, count(m.user_record_id)
		FROM workspace_groups g
		LEFT JOIN workspace_group_members m ON m.group_id = g.id
		WHERE g.workspace_id = $1 AND g.id = $2
		GROUP BY g.name
	`, workspaceID, groupID).Scan(&g.Name, &g.MemberCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("get group: %w", err)
	}
	filled, err := s.attachGrants(ctx, workspaceID, []Group{*g})
	if err != nil {
		return nil, err
	}
	return &filled[0], nil
}

// GroupMemberIDs returns the user record ids belonging to one Group.
func (s *Store) GroupMemberIDs(ctx context.Context, groupID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT user_record_id FROM workspace_group_members WHERE group_id = $1
	`, groupID)
	if err != nil {
		return nil, fmt.Errorf("list group members: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan group member: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GroupIDsForMember returns the ids of the Groups userRecordID belongs to in one Workspace, as a
// set ready for domain.Actor.
//
// One narrow query rather than GroupsByMember below, because the caller is every authenticated
// request: this runs on the indexed member column (idx_workspace_group_members_user) and returns
// ids only -- no names, no member counts, no grants. GroupsByMember answers a different question
// (who is in what, for the whole Members list) and would read every Group in the Workspace to
// answer this one.
//
// Workspace-scoped through the join, not filtered afterwards: a Group id from another Workspace
// must not resolve here even if the membership row somehow named it, since a Permission gate reads
// the result.
func (s *Store) GroupIDsForMember(ctx context.Context, workspaceID, userRecordID string) (map[string]bool, error) {
	if userRecordID == "" {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT m.group_id
		FROM workspace_group_members m
		JOIN workspace_groups g ON g.id = m.group_id
		WHERE m.user_record_id = $1 AND g.workspace_id = $2
	`, userRecordID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list groups for member: %w", err)
	}
	defer rows.Close()

	ids := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan group id: %w", err)
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

// GroupsByMember returns every member's Groups for one Workspace, keyed by user record id.
//
// One query, stitched in Go: the Members list needs a Source per row, and asking per member would
// be exactly the N+1 ListMembers already avoids for roles.
func (s *Store) GroupsByMember(ctx context.Context, workspaceID string) (map[string][]Group, error) {
	groups, err := s.ListGroups(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]Group, len(groups))
	for _, g := range groups {
		byID[g.ID] = g
	}

	rows, err := s.pool.Query(ctx, `
		SELECT m.user_record_id, m.group_id
		FROM workspace_group_members m
		JOIN workspace_groups g ON g.id = m.group_id
		WHERE g.workspace_id = $1
		ORDER BY g.name
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list memberships by group: %w", err)
	}
	defer rows.Close()

	byMember := map[string][]Group{}
	for rows.Next() {
		var userRecordID, groupID string
		if err := rows.Scan(&userRecordID, &groupID); err != nil {
			return nil, fmt.Errorf("scan group membership: %w", err)
		}
		if g, ok := byID[groupID]; ok {
			byMember[userRecordID] = append(byMember[userRecordID], g)
		}
	}
	return byMember, rows.Err()
}

// CreateGroup adds a Group to workspaceID and returns it.
func (s *Store) CreateGroup(ctx context.Context, workspaceID, name string) (*Group, error) {
	g := &Group{ID: newID("grp_"), Name: name, Grants: map[string]string{}}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO workspace_groups (id, workspace_id, name) VALUES ($1, $2, $3)
	`, g.ID, workspaceID, name)
	if err != nil {
		return nil, fmt.Errorf("create group: %w", err)
	}
	return g, nil
}

// DeleteGroup removes a Group. Its members and grants go with it via ON DELETE CASCADE, which is
// the honest behaviour: a grant belongs to the Group that holds it, and a membership to the Group
// it is in -- neither means anything once the Group is gone.
func (s *Store) DeleteGroup(ctx context.Context, workspaceID, groupID string) error {
	ct, err := s.pool.Exec(ctx, `
		DELETE FROM workspace_groups WHERE workspace_id = $1 AND id = $2
	`, workspaceID, groupID)
	if err != nil {
		return fmt.Errorf("delete group: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrRecordNotFound
	}
	return nil
}

// SetGroupMembers replaces a Group's membership wholesale, which is what the ported detail screen
// submits: its member chips and add-member checkboxes are one form describing the intended final
// set, not a sequence of add/remove operations.
func (s *Store) SetGroupMembers(ctx context.Context, groupID string, userRecordIDs []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("set group members: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM workspace_group_members WHERE group_id = $1`, groupID); err != nil {
		return fmt.Errorf("clear group members: %w", err)
	}
	for _, id := range userRecordIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO workspace_group_members (group_id, user_record_id) VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, groupID, id); err != nil {
			return fmt.Errorf("add group member: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// SetGroupAppRole assigns or clears a Group's role in one Application. An empty role DELETEs the
// row rather than storing "", exactly as SetMemberAppRole does -- absence is how "no role here" is
// said on both sides.
func (s *Store) SetGroupAppRole(ctx context.Context, groupID, applicationID, role string) error {
	if role == "" {
		_, err := s.pool.Exec(ctx, `
			DELETE FROM workspace_group_app_roles WHERE group_id = $1 AND application_id = $2
		`, groupID, applicationID)
		if err != nil {
			return fmt.Errorf("clear group app role: %w", err)
		}
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO workspace_group_app_roles (group_id, application_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (group_id, application_id) DO UPDATE SET role = EXCLUDED.role
	`, groupID, applicationID, role)
	if err != nil {
		return fmt.Errorf("set group app role: %w", err)
	}
	return nil
}

// ActorMembership returns everything a domain.Actor needs beyond its own id: the Groups this
// member belongs to (each with its per-Application grants) and their own direct per-Application
// roles. The caller merges the two through EffectiveRoles, which stays the one place CAP-O07's
// union rule is expressed.
//
// It replaces GroupIDsForMember at the one call site that needs roles as well (internal/web's
// currentActor, on every permission-checking request), and is deliberately two narrow queries
// rather than ListGroups + attachGrants: those read *every* Group in the Workspace to answer a
// question about one member, which is the N+1-in-reverse this method exists to avoid. The grants
// come back on the same row as the Group through a LEFT JOIN, so a Group holding no grant at all
// still appears -- it still gates a CAP-F24 approver_group even when it grants no role.
func (s *Store) ActorMembership(ctx context.Context, workspaceID, userRecordID string) ([]Group, map[string]string, error) {
	if userRecordID == "" {
		return nil, nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT g.id, g.name, r.application_id, r.role
		FROM workspace_group_members m
		JOIN workspace_groups g ON g.id = m.group_id
		LEFT JOIN workspace_group_app_roles r ON r.group_id = g.id
		WHERE m.user_record_id = $1 AND g.workspace_id = $2
		ORDER BY g.name
	`, userRecordID, workspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("list actor groups: %w", err)
	}
	defer rows.Close()

	var groups []Group
	byID := map[string]int{}
	for rows.Next() {
		var id, name string
		var appID, role *string
		if err := rows.Scan(&id, &name, &appID, &role); err != nil {
			return nil, nil, fmt.Errorf("scan actor group: %w", err)
		}
		i, seen := byID[id]
		if !seen {
			groups = append(groups, Group{ID: id, Name: name, Grants: map[string]string{}})
			i = len(groups) - 1
			byID[id] = i
		}
		if appID != nil && role != nil {
			groups[i].Grants[*appID] = *role
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	direct, err := s.appRolesFor(ctx, workspaceID, userRecordID)
	if err != nil {
		return nil, nil, err
	}
	return groups, direct, nil
}
