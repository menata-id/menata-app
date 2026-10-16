package web

import (
	"context"
	"testing"

	"menata.app/internal/data"
)

// A Group id is a handle into one Workspace. GroupMemberIDs used to select on the id alone, so any caller holding another
// Workspace's Group id (a URL parameter, say) read that Workspace's membership. The Workspace is in the statement now.
func TestGroupMemberIDsDoesNotCrossWorkspaces(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	mine, err := store.CreateWorkspace(ctx, "Group Scope Mine", "group-scope-mine")
	if err != nil {
		t.Fatal(err)
	}
	cleanupAuthTest(t, pool, mine.ID, "group_scope_mine@example.com")
	theirs, err := store.CreateWorkspace(ctx, "Group Scope Theirs", "group-scope-theirs")
	if err != nil {
		t.Fatal(err)
	}
	cleanupAuthTest(t, pool, theirs.ID, "group_scope_theirs@example.com")

	group := newTestGroup(t, ctx, store, theirs.ID, "Theirs")
	if err := store.SetGroupMembers(ctx, group.ID, []string{"rec_member_a", "rec_member_b"}); err != nil {
		t.Fatal(err)
	}

	own, err := store.GroupMemberIDs(ctx, theirs.ID, group.ID)
	if err != nil || len(own) != 2 {
		t.Fatalf("a Group's own Workspace reads %v, %v; want its two members", own, err)
	}
	across, err := store.GroupMemberIDs(ctx, mine.ID, group.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(across) != 0 {
		t.Errorf("another Workspace read this Group's members: %v", across)
	}
	if none, _ := store.GroupMemberIDs(ctx, "", group.ID); len(none) != 0 {
		t.Errorf("an empty Workspace id read members: %v", none)
	}
}
