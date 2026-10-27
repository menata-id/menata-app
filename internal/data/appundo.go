package data

import (
	"context"
	"fmt"
	"time"
)

// AIApplicationOrigins is AIApplicationOrigin for every Application of a Workspace at once: when the assistant first published each, keyed by
// application id. One statement, so a list of Applications does not cost one read each.
func (s *Store) AIApplicationOrigins(ctx context.Context, workspaceID string) (map[string]time.Time, error) {
	readLogFrom(ctx).record("ai application origins")
	rows, err := s.pool.Query(ctx, `
		SELECT application_id, min(published_at) FROM ai_sessions
		 WHERE workspace_id = $1 AND application_id IS NOT NULL AND published_at IS NOT NULL
		   AND target = 'new_application' AND status = 'published'
		 GROUP BY application_id
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("ai application origins: %w", err)
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var id string
		var at time.Time
		if err := rows.Scan(&id, &at); err != nil {
			return nil, err
		}
		out[id] = at
	}
	return out, rows.Err()
}

// ForgetPublishedApplication drops what the database holds for an Application whose files have been taken out of the Workspace (an undone
// publication): every member's direct role in it, every Group's grant for it, and the assistant session that published it goes back to
// "generated" with no publication, so its proposal can be reviewed and published again. Records of its Machines are not touched; the caller
// refuses to remove an Application that holds any. One transaction.
func (s *Store) ForgetPublishedApplication(ctx context.Context, workspaceID, applicationID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("forget published application: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, stmt := range []string{
		`DELETE FROM workspace_member_app_roles WHERE workspace_id = $1 AND application_id = $2`,
		`DELETE FROM workspace_group_app_roles WHERE application_id = $2
		   AND group_id IN (SELECT id FROM workspace_groups WHERE workspace_id = $1)`,
		`UPDATE ai_sessions SET status = 'generated', published_at = NULL, application_id = NULL, updated_at = NOW()
		  WHERE workspace_id = $1 AND application_id = $2 AND target = 'new_application' AND status = 'published'`,
	} {
		if _, err := tx.Exec(ctx, stmt, workspaceID, applicationID); err != nil {
			return fmt.Errorf("forget published application: %w", err)
		}
	}
	return tx.Commit(ctx)
}
