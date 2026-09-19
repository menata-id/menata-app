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
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, first.ID) })

	second, err := store.CreateWorkspace(ctx, "Acme Two", "acme-workspace-test")
	if err != nil {
		t.Fatalf("CreateWorkspace(second, colliding slug): %v", err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, second.ID) })

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

	if err := store.CreateCredential(ctx, email, "hashed-value", false); err != nil {
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
