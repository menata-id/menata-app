package execution

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool/cleanupTest mirror internal/web's own authTestPool/cleanupAuthTest exactly (same
// Postgres integration-test convention, t.Skip without DATABASE_URL) -- duplicated rather than
// shared because internal/web cannot be imported here (it depends on internal/execution, not the
// other way around) and the two packages have no third place both could import it from that would
// be worth the indirection for a handful of lines.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("set DATABASE_URL to run internal/execution's integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func cleanupTest(t *testing.T, pool *pgxpool.Pool, workspaceID, email string) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		if _, err := pool.Exec(ctx, `DELETE FROM workspace_member_app_roles WHERE workspace_id = $1`, workspaceID); err != nil {
			t.Errorf("cleanup workspace_member_app_roles: %v", err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM workspace_members WHERE workspace_id = $1`, workspaceID); err != nil {
			t.Errorf("cleanup workspace_members: %v", err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM pending_invites WHERE workspace_id = $1`, workspaceID); err != nil {
			t.Errorf("cleanup pending_invites: %v", err)
		}
		if _, err := pool.Exec(ctx, `
			DELETE FROM session_generations
			WHERE subject IN (SELECT id FROM records WHERE workspace_id = $1)
		`, workspaceID); err != nil {
			t.Errorf("cleanup session_generations: %v", err)
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
