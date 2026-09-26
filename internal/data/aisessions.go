package data

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// AISession is one AI Metadata Assistant conversation (migration 012, Flow 2 gap study Tahap 8).
type AISession struct {
	ID          string
	WorkspaceID string
	ActorUserID string
	Target      string
	Status      string
	Turns       []AISessionTurn
}

// AISessionTurn is one message in an AISession, in either direction.
type AISessionTurn struct {
	ID      string
	Role    string
	Content string
}

const (
	AISessionStatusOpen      = "open"
	AISessionStatusGenerated = "generated"
	AISessionStatusPublished = "published"
	AISessionStatusDiscarded = "discarded"
)

// CreateAISession starts a new conversation -- the same "persist a real, incomplete row from the
// first message" convention the Document submit wizard's own draft already established, rather
// than threading an ephemeral blob through hidden form fields (confirmed by direct read: no
// non-persisted multi-request state exists anywhere else in this codebase).
func (s *Store) CreateAISession(ctx context.Context, workspaceID, actorUserID, target string) (*AISession, error) {
	session := &AISession{ID: newID("aise_"), WorkspaceID: workspaceID, ActorUserID: actorUserID, Target: target, Status: AISessionStatusOpen}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO ai_sessions (id, workspace_id, actor_user_id, target, status) VALUES ($1, $2, $3, $4, $5)
	`, session.ID, workspaceID, actorUserID, target, session.Status)
	if err != nil {
		return nil, fmt.Errorf("create ai session: %w", err)
	}
	return session, nil
}

// GetAISession fetches one session together with its own turns, in order -- session_id plus
// workspace_id so one workspace can never read another's conversation.
func (s *Store) GetAISession(ctx context.Context, workspaceID, id string) (*AISession, error) {
	readLogFrom(ctx).record("ai session")
	var session AISession
	err := s.pool.QueryRow(ctx, `
		SELECT id, workspace_id, actor_user_id, target, status
		FROM ai_sessions WHERE id = $1 AND workspace_id = $2
	`, id, workspaceID).Scan(&session.ID, &session.WorkspaceID, &session.ActorUserID, &session.Target, &session.Status)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("get ai session: %w", err)
	}

	readLogFrom(ctx).record("ai session turns")
	rows, err := s.pool.Query(ctx, `
		SELECT id, role, content FROM ai_session_turns WHERE session_id = $1 ORDER BY created_at
	`, id)
	if err != nil {
		return nil, fmt.Errorf("list ai session turns: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var t AISessionTurn
		if err := rows.Scan(&t.ID, &t.Role, &t.Content); err != nil {
			return nil, fmt.Errorf("scan ai session turn: %w", err)
		}
		session.Turns = append(session.Turns, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list ai session turns: %w", err)
	}
	return &session, nil
}

// AppendAISessionTurn adds one turn to an existing session -- append-only, matching
// mch_activity's own discipline, enforced here at the Go layer since this table is not
// Machine-governed (nothing routes it through domain.Machine.AppendOnly).
func (s *Store) AppendAISessionTurn(ctx context.Context, sessionID, role, content string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO ai_session_turns (id, session_id, role, content) VALUES ($1, $2, $3, $4)
	`, newID("aiturn_"), sessionID, role, content)
	if err != nil {
		return fmt.Errorf("append ai session turn: %w", err)
	}
	return nil
}

// UpdateAISessionStatus moves a session to a new status (see the AISessionStatus* constants) and
// touches updated_at.
func (s *Store) UpdateAISessionStatus(ctx context.Context, id, status string) error {
	ct, err := s.pool.Exec(ctx, `
		UPDATE ai_sessions SET status = $2, updated_at = NOW() WHERE id = $1
	`, id, status)
	if err != nil {
		return fmt.Errorf("update ai session status: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrRecordNotFound
	}
	return nil
}

// RecordAICapabilityGap writes one structured "the runtime can't say this yet" row (the kajian's
// own §3.4 ask) -- never updated or deleted once written, so a later count across many
// conversations reflects real, unedited history.
func (s *Store) RecordAICapabilityGap(ctx context.Context, sessionID, workspaceID, requested, note string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO ai_capability_gaps (id, session_id, workspace_id, requested_capability, note)
		VALUES ($1, $2, $3, $4, $5)
	`, newID("aigap_"), sessionID, workspaceID, requested, note)
	if err != nil {
		return fmt.Errorf("record ai capability gap: %w", err)
	}
	return nil
}
