package data

import (
	"context"
	"testing"
)

func newAISessionTestWorkspace(t *testing.T, store *Store) string {
	t.Helper()
	ctx := context.Background()
	ws, err := store.CreateWorkspace(ctx, "AI Session Test", "ai-session-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	t.Cleanup(func() {
		// ai_sessions/ai_session_turns/ai_capability_gaps all declare ON DELETE CASCADE against
		// workspaces (migration 012), so deleting the workspace row is the whole cleanup.
		if _, err := store.pool.Exec(context.Background(), `DELETE FROM workspaces WHERE id = $1`, ws.ID); err != nil {
			t.Errorf("cleanup workspace: %v", err)
		}
	})
	return ws.ID
}

func TestStore_AISession_createAppendAndGet(t *testing.T) {
	pool := storePool(t)
	store := NewStore(pool)
	ctx := context.Background()
	workspaceID := newAISessionTestWorkspace(t, store)

	session, err := store.CreateAISession(ctx, workspaceID, "usr_test", KindNewApplicationForTest)
	if err != nil {
		t.Fatalf("CreateAISession: %v", err)
	}
	if session.Status != AISessionStatusOpen {
		t.Errorf("Status = %q, want %q", session.Status, AISessionStatusOpen)
	}

	if err := store.AppendAISessionTurn(ctx, session.ID, "user", "Staff request leave, their supervisor approves it."); err != nil {
		t.Fatalf("AppendAISessionTurn(user): %v", err)
	}
	if err := store.AppendAISessionTurn(ctx, session.ID, "model", `{"message":"How many approval steps?"}`); err != nil {
		t.Fatalf("AppendAISessionTurn(model): %v", err)
	}

	got, err := store.GetAISession(ctx, workspaceID, session.ID)
	if err != nil {
		t.Fatalf("GetAISession: %v", err)
	}
	if len(got.Turns) != 2 {
		t.Fatalf("len(Turns) = %d, want 2", len(got.Turns))
	}
	if got.Turns[0].Role != "user" || got.Turns[1].Role != "model" {
		t.Errorf("turns out of order: %+v", got.Turns)
	}

	if err := store.UpdateAISessionStatus(ctx, session.ID, AISessionStatusPublished); err != nil {
		t.Fatalf("UpdateAISessionStatus: %v", err)
	}
	got, err = store.GetAISession(ctx, workspaceID, session.ID)
	if err != nil {
		t.Fatalf("GetAISession after status update: %v", err)
	}
	if got.Status != AISessionStatusPublished {
		t.Errorf("Status = %q, want %q", got.Status, AISessionStatusPublished)
	}
}

func TestStore_AISession_getWrongWorkspaceNotFound(t *testing.T) {
	pool := storePool(t)
	store := NewStore(pool)
	ctx := context.Background()
	workspaceID := newAISessionTestWorkspace(t, store)
	otherWorkspaceID := newAISessionTestWorkspace(t, store)

	session, err := store.CreateAISession(ctx, workspaceID, "usr_test", KindNewApplicationForTest)
	if err != nil {
		t.Fatalf("CreateAISession: %v", err)
	}

	if _, err := store.GetAISession(ctx, otherWorkspaceID, session.ID); err != ErrRecordNotFound {
		t.Errorf("GetAISession from a different workspace = %v, want ErrRecordNotFound", err)
	}
}

func TestStore_RecordAICapabilityGap(t *testing.T) {
	pool := storePool(t)
	store := NewStore(pool)
	ctx := context.Background()
	workspaceID := newAISessionTestWorkspace(t, store)

	session, err := store.CreateAISession(ctx, workspaceID, "usr_test", KindNewApplicationForTest)
	if err != nil {
		t.Fatalf("CreateAISession: %v", err)
	}
	if err := store.RecordAICapabilityGap(ctx, session.ID, workspaceID, "multi-person voting approval", "user asked for three people to vote on a request"); err != nil {
		t.Fatalf("RecordAICapabilityGap: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ai_capability_gaps WHERE session_id = $1 AND requested_capability = $2`,
		session.ID, "multi-person voting approval").Scan(&count); err != nil {
		t.Fatalf("query capability gap: %v", err)
	}
	if count != 1 {
		t.Errorf("capability gap row count = %d, want 1", count)
	}
}

// KindNewApplicationForTest avoids this test file depending on internal/aiassist just to spell
// "new_application" -- the literal is this table's own free-text convention (migration 012's own
// comment: "new_application", or the id of an Application being extended), not a foreign key to
// anything aiassist declares.
const KindNewApplicationForTest = "new_application"
