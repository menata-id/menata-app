package data

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// GetAppDraft is the pending Application Builder operations (JSON) one admin holds for one Application, or nil when they hold none.
func (s *Store) GetAppDraft(ctx context.Context, workspaceID, appID, userID string) ([]byte, error) {
	readLogFrom(ctx).record("app draft")
	var ops []byte
	err := s.pool.QueryRow(ctx, `SELECT ops FROM app_drafts WHERE workspace_id = $1 AND app_id = $2 AND user_id = $3`, workspaceID, appID, userID).Scan(&ops)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get app draft: %w", err)
	}
	return ops, nil
}

// SaveAppDraft replaces the pending operations one admin holds for one Application.
func (s *Store) SaveAppDraft(ctx context.Context, workspaceID, appID, userID string, ops []byte) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO app_drafts (workspace_id, app_id, user_id, ops) VALUES ($1, $2, $3, $4)
		ON CONFLICT (workspace_id, app_id, user_id) DO UPDATE SET ops = EXCLUDED.ops, updated_at = now()
	`, workspaceID, appID, userID, ops)
	if err != nil {
		return fmt.Errorf("save app draft: %w", err)
	}
	return nil
}

// DeleteAppDraft drops the pending operations one admin holds for one Application.
func (s *Store) DeleteAppDraft(ctx context.Context, workspaceID, appID, userID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM app_drafts WHERE workspace_id = $1 AND app_id = $2 AND user_id = $3`, workspaceID, appID, userID)
	if err != nil {
		return fmt.Errorf("delete app draft: %w", err)
	}
	return nil
}

// ListAppDrafts is every Application's pending operations (JSON) that one admin holds in a Workspace, keyed by Application id.
func (s *Store) ListAppDrafts(ctx context.Context, workspaceID, userID string) (map[string][]byte, error) {
	readLogFrom(ctx).record("app drafts")
	rows, err := s.pool.Query(ctx, `SELECT app_id, ops FROM app_drafts WHERE workspace_id = $1 AND user_id = $2`, workspaceID, userID)
	if err != nil {
		return nil, fmt.Errorf("list app drafts: %w", err)
	}
	defer rows.Close()
	out := map[string][]byte{}
	for rows.Next() {
		var app string
		var ops []byte
		if err := rows.Scan(&app, &ops); err != nil {
			return nil, fmt.Errorf("scan app draft: %w", err)
		}
		out[app] = ops
	}
	return out, rows.Err()
}
