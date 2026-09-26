package data

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func cleanupWorkspaceTest(t *testing.T, pool *pgxpool.Pool, workspaceID, email string) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		// Before the workspaces row itself: workspace_member_app_roles references workspaces, so
		// leaving its rows behind turns cleanup into a foreign-key error (Fase 3b, migration 008).
		if _, err := pool.Exec(ctx, `DELETE FROM workspace_member_app_roles WHERE workspace_id = $1`, workspaceID); err != nil {
			t.Errorf("cleanup workspace_member_app_roles: %v", err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM workspace_members WHERE workspace_id = $1`, workspaceID); err != nil {
			t.Errorf("cleanup workspace_members: %v", err)
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

func TestStore_CreateWorkspace_slugCollisionRetries(t *testing.T) {
	pool := storePool(t)
	store := NewStore(pool)
	ctx := context.Background()

	first, err := store.CreateWorkspace(ctx, "Acme", "acme-workspace-test")
	if err != nil {
		t.Fatalf("CreateWorkspace(first): %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, first.ID); err != nil {
			t.Errorf("cleanup workspaces (first): %v", err)
		}
	})

	second, err := store.CreateWorkspace(ctx, "Acme Two", "acme-workspace-test")
	if err != nil {
		t.Fatalf("CreateWorkspace(second, colliding slug): %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, second.ID); err != nil {
			t.Errorf("cleanup workspaces (second): %v", err)
		}
	})

	if second.Slug == first.Slug {
		t.Errorf("CreateWorkspace(second).Slug = %q, want it to differ from the first (%q)", second.Slug, first.Slug)
	}
}

func TestStore_MembershipLifecycle(t *testing.T) {
	pool := storePool(t)
	store := NewStore(pool)
	ctx := context.Background()
	const email = "member_test@example.com"

	ws, err := store.CreateWorkspace(ctx, "Membership Test", "membership-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupWorkspaceTest(t, pool, ws.ID, email)

	if err := store.CreateCredential(ctx, email, "Test Person", "hashed-value", false); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}

	user, err := store.CreateRecord(WithWorkspaceScope(ctx, ws.ID), "mch_user", map[string]any{"fld_name": "Test Member", "fld_email": email})
	if err != nil {
		t.Fatalf("CreateRecord(mch_user): %v", err)
	}

	if err := store.AddMember(ctx, ws.ID, user.ID, email, "admin", ""); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	memberships, err := store.ListMemberships(ctx, email)
	if err != nil {
		t.Fatalf("ListMemberships: %v", err)
	}
	if len(memberships) != 1 || memberships[0].WorkspaceID != ws.ID || memberships[0].UserRecordID != user.ID {
		t.Fatalf("ListMemberships = %+v, want one membership in %s for %s", memberships, ws.ID, user.ID)
	}
	if memberships[0].WorkspaceRole != "admin" {
		t.Errorf("ListMemberships[0].WorkspaceRole = %q, want %q", memberships[0].WorkspaceRole, "admin")
	}

	members, err := store.ListMembers(ctx, ws.ID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members) != 1 || members[0].Email != email {
		t.Fatalf("ListMembers = %+v, want one member with email %s", members, email)
	}

	membership, err := store.GetMembership(ctx, ws.ID, user.ID)
	if err != nil {
		t.Fatalf("GetMembership: %v", err)
	}
	if membership.WorkspaceRole != "admin" || membership.Email != email {
		t.Errorf("GetMembership = %+v, want workspace_role=admin email=%s", membership, email)
	}

	resolved, err := store.ResolveUserWorkspace(ctx, user.ID)
	if err != nil {
		t.Fatalf("ResolveUserWorkspace: %v", err)
	}
	if resolved != ws.ID {
		t.Errorf("ResolveUserWorkspace() = %q, want %q", resolved, ws.ID)
	}

	if err := store.UpdateMemberRole(ctx, ws.ID, user.ID, "member", "approver"); err != nil {
		t.Fatalf("UpdateMemberRole: %v", err)
	}
	updated, err := store.ListMembers(ctx, ws.ID)
	if err != nil {
		t.Fatalf("ListMembers after update: %v", err)
	}
	if updated[0].WorkspaceRole != "member" || updated[0].AppRole != "approver" {
		t.Errorf("ListMembers after update = %+v, want workspace_role=member app_role=approver", updated[0])
	}
}

// TestStore_ListMemberships_reflectsArchivedState is Choose Workspace's own read path: the same
// join that already returns WorkspaceName must also return Archived/ArchivedAt, since
// loadWorkspaceChoices (internal/web/auth.go) has no second query to fall back on.
func TestStore_ListMemberships_reflectsArchivedState(t *testing.T) {
	pool := storePool(t)
	store := NewStore(pool)
	ctx := context.Background()
	const email = "archived_membership_test@example.com"

	ws, err := store.CreateWorkspace(ctx, "Archived Membership Test", "archived-membership-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupWorkspaceTest(t, pool, ws.ID, email)

	if err := store.CreateCredential(ctx, email, "Test Person", "hashed-value", false); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}
	user, err := store.CreateRecord(WithWorkspaceScope(ctx, ws.ID), "mch_user", map[string]any{"fld_name": "Test Member", "fld_email": email})
	if err != nil {
		t.Fatalf("CreateRecord(mch_user): %v", err)
	}
	if err := store.AddMember(ctx, ws.ID, user.ID, email, "admin", ""); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	before, err := store.ListMemberships(ctx, email)
	if err != nil {
		t.Fatalf("ListMemberships (before archive): %v", err)
	}
	if len(before) != 1 || before[0].Archived || before[0].ArchivedAt != nil {
		t.Fatalf("ListMemberships (before archive) = %+v, want one live (unarchived) membership", before)
	}

	if err := store.ArchiveWorkspace(ctx, ws.ID); err != nil {
		t.Fatalf("ArchiveWorkspace: %v", err)
	}

	after, err := store.ListMemberships(ctx, email)
	if err != nil {
		t.Fatalf("ListMemberships (after archive): %v", err)
	}
	if len(after) != 1 || !after[0].Archived || after[0].ArchivedAt == nil {
		t.Fatalf("ListMemberships (after archive) = %+v, want one archived membership with ArchivedAt set", after)
	}
}

func TestStore_GetMembership_notFound(t *testing.T) {
	pool := storePool(t)
	store := NewStore(pool)

	_, err := store.GetMembership(context.Background(), "ws_does_not_exist", "rec_does_not_exist")
	if !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("GetMembership(missing) error = %v, want ErrRecordNotFound", err)
	}
}

func TestStore_ResolveUserWorkspace_notFound(t *testing.T) {
	pool := storePool(t)
	store := NewStore(pool)

	_, err := store.ResolveUserWorkspace(context.Background(), "rec_does_not_exist")
	if !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("ResolveUserWorkspace(missing) error = %v, want ErrRecordNotFound", err)
	}
}

func TestStore_UpdateMemberRole_notFound(t *testing.T) {
	pool := storePool(t)
	store := NewStore(pool)

	err := store.UpdateMemberRole(context.Background(), "ws_does_not_exist", "rec_does_not_exist", "member", "")
	if !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("UpdateMemberRole(missing) error = %v, want ErrRecordNotFound", err)
	}
}

// TestStore_ArchiveRestoreWorkspace is Tahap 7's own round-trip: archive sets Archived/ArchivedAt,
// restore clears both, and each refuses (ErrRecordNotFound) when the Workspace is already in the
// state being asked for -- the guard that stops a re-archive from silently bumping archived_at to
// a later moment.
func TestStore_ArchiveRestoreWorkspace(t *testing.T) {
	pool := storePool(t)
	store := NewStore(pool)
	ctx := context.Background()

	ws, err := store.CreateWorkspace(ctx, "Archive Test", "archive-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, ws.ID); err != nil {
			t.Errorf("cleanup workspaces: %v", err)
		}
	})

	if err := store.ArchiveWorkspace(ctx, ws.ID); err != nil {
		t.Fatalf("ArchiveWorkspace: %v", err)
	}
	archived, err := store.GetWorkspace(ctx, ws.ID)
	if err != nil {
		t.Fatalf("GetWorkspace after archive: %v", err)
	}
	if !archived.Archived || archived.ArchivedAt == nil {
		t.Fatalf("GetWorkspace after archive = %+v, want Archived=true and ArchivedAt set", archived)
	}

	if err := store.ArchiveWorkspace(ctx, ws.ID); !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("ArchiveWorkspace(already archived) error = %v, want ErrRecordNotFound", err)
	}

	if err := store.RestoreWorkspace(ctx, ws.ID); err != nil {
		t.Fatalf("RestoreWorkspace: %v", err)
	}
	restored, err := store.GetWorkspace(ctx, ws.ID)
	if err != nil {
		t.Fatalf("GetWorkspace after restore: %v", err)
	}
	if restored.Archived || restored.ArchivedAt != nil {
		t.Fatalf("GetWorkspace after restore = %+v, want Archived=false and ArchivedAt nil", restored)
	}

	if err := store.RestoreWorkspace(ctx, ws.ID); !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("RestoreWorkspace(already live) error = %v, want ErrRecordNotFound", err)
	}
}

func TestStore_ArchiveWorkspace_notFound(t *testing.T) {
	pool := storePool(t)
	store := NewStore(pool)

	err := store.ArchiveWorkspace(context.Background(), "ws_does_not_exist")
	if !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("ArchiveWorkspace(missing) error = %v, want ErrRecordNotFound", err)
	}
}

func TestStore_RestoreWorkspace_notFound(t *testing.T) {
	pool := storePool(t)
	store := NewStore(pool)

	err := store.RestoreWorkspace(context.Background(), "ws_does_not_exist")
	if !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("RestoreWorkspace(missing) error = %v, want ErrRecordNotFound", err)
	}
}
