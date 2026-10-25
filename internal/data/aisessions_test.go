package data

import (
	"context"
	"testing"
	"time"
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

	if err := store.UpdateAISessionStatus(ctx, workspaceID, session.ID, AISessionStatusPublished); err != nil {
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
	if err := store.RecordAICapabilityGap(ctx, session.ID, workspaceID, AICapabilityGap{
		Kind: "runtime", Requested: "multi-person voting approval", Note: "user asked for three people to vote on a request",
		CapabilityKey: "multi-person-voting-approval", Class: "workflow_logic",
		IdealCapability: "Several people each cast a vote and the request moves when a declared quorum is reached.",
	}); err != nil {
		t.Fatalf("RecordAICapabilityGap: %v", err)
	}

	var count int
	var key, class, ideal string
	if err := pool.QueryRow(ctx, `SELECT count(*), max(capability_key), max(class), max(ideal_capability) FROM ai_capability_gaps WHERE session_id = $1 AND requested_capability = $2`,
		session.ID, "multi-person voting approval").Scan(&count, &key, &class, &ideal); err != nil {
		t.Fatalf("query capability gap: %v", err)
	}
	if count != 1 {
		t.Errorf("capability gap row count = %d, want 1", count)
	}
	if key != "multi-person-voting-approval" || class != "workflow_logic" || ideal == "" {
		t.Errorf("stored key/class/ideal = %q/%q/%q, want the detail columns written", key, class, ideal)
	}
}

// KindNewApplicationForTest avoids this test file depending on internal/aiassist just to spell
// "new_application" -- the literal is this table's own free-text convention (migration 012's own
// comment: "new_application", or the id of an Application being extended), not a foreign key to
// anything aiassist declares.
const KindNewApplicationForTest = "new_application"

// WU: a session's publication carries when and which application, and an application's origin is the
// earliest new_application publication; an update session never counts as the origin.
func TestStore_AISessionPublication_originIsTheFirstNewApplicationPublish(t *testing.T) {
	pool := storePool(t)
	store := NewStore(pool)
	ctx := context.Background()
	workspaceID := newAISessionTestWorkspace(t, store)

	if _, ok, err := store.AIApplicationOrigin(ctx, workspaceID, "app_w"); err != nil || ok {
		t.Fatalf("origin of an unpublished application = ok %v err %v, want unknown", ok, err)
	}
	created, err := store.CreateAISession(ctx, workspaceID, "usr_test", KindNewApplicationForTest)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkAISessionPublished(ctx, workspaceID, created.ID, "app_w"); err != nil {
		t.Fatalf("MarkAISessionPublished: %v", err)
	}
	updated, err := store.CreateAISession(ctx, workspaceID, "usr_test", "app_w")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkAISessionPublished(ctx, workspaceID, updated.ID, "app_w"); err != nil {
		t.Fatal(err)
	}
	var createdAt, updatedAt time.Time
	if err := pool.QueryRow(ctx, `SELECT published_at FROM ai_sessions WHERE id = $1`, created.ID).Scan(&createdAt); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT published_at FROM ai_sessions WHERE id = $1`, updated.ID).Scan(&updatedAt); err != nil {
		t.Fatal(err)
	}
	origin, ok, err := store.AIApplicationOrigin(ctx, workspaceID, "app_w")
	if err != nil || !ok {
		t.Fatalf("origin = ok %v err %v, want known", ok, err)
	}
	if !origin.Equal(createdAt) {
		t.Errorf("origin = %v, want the new_application session's publication %v (not the update's %v)", origin, createdAt, updatedAt)
	}
	// An application only ever updated by the assistant (handwritten, or installed from the library) has no origin.
	onlyUpdated, err := store.CreateAISession(ctx, workspaceID, "usr_test", "app_hand")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkAISessionPublished(ctx, workspaceID, onlyUpdated.ID, "app_hand"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.AIApplicationOrigin(ctx, workspaceID, "app_hand"); err != nil || ok {
		t.Errorf("an application the assistant only updated reported an origin (ok %v, err %v): its first update would read as age zero", ok, err)
	}
	if err := store.MarkAISessionPublished(ctx, "ws_other", created.ID, "app_w"); err != ErrRecordNotFound {
		t.Errorf("publishing through another workspace = %v, want ErrRecordNotFound", err)
	}
}

// The publish route's update path reads the application's origin; that read names itself, or the
// request is logged ANOMALY(unnamed) on every publish of an update (2026-10-25 log review).
func TestStore_AIApplicationOrigin_namesItsRead(t *testing.T) {
	pool := storePool(t)
	store := NewStore(pool)
	ctx, rl := WithReadLog(context.Background())
	workspaceID := newAISessionTestWorkspace(t, store)
	if _, _, err := store.AIApplicationOrigin(ctx, workspaceID, "app_w"); err != nil {
		t.Fatal(err)
	}
	if rl.byTarget["ai application origin"] != 1 {
		t.Errorf("the origin read did not name itself: %v", rl.byTarget)
	}
}
