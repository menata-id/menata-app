package data

import (
	"context"
	"fmt"
	"time"
)

// AISessionBrief is what Workspace Home needs of an assistant session of a new Application: whether it is a draft awaiting review (generated) or
// was published, which Application it published and when.
type AISessionBrief struct {
	ID            string
	Status        string
	ApplicationID string
	PublishedAt   *time.Time
}

// ListAISessionsForHome is every new-Application session Home may show, in one statement: the drafts (generated) and the publications since
// `since` (those can still be taken back). Home's draft rows and its Take back rows both read it, through the Loader's memo, so the page pays once.
func (s *Store) ListAISessionsForHome(ctx context.Context, workspaceID string, since time.Time) ([]AISessionBrief, error) {
	readLogFrom(ctx).record("ai sessions for home")
	rows, err := s.pool.Query(ctx, `
		SELECT id, status, COALESCE(application_id, ''), published_at FROM ai_sessions
		 WHERE workspace_id = $1 AND target = 'new_application'
		   AND (status = 'generated' OR (status = 'published' AND published_at >= $2))
		 ORDER BY created_at
	`, workspaceID, since)
	if err != nil {
		return nil, fmt.Errorf("list ai sessions for home: %w", err)
	}
	defer rows.Close()
	var out []AISessionBrief
	for rows.Next() {
		var b AISessionBrief
		if err := rows.Scan(&b.ID, &b.Status, &b.ApplicationID, &b.PublishedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
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
