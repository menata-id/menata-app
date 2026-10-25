package data

import (
	"context"
	"fmt"
	"time"

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

// ListAISessionsByStatus returns every session in workspaceID matching target and status, turns
// not included (GetAISession loads those, per id) -- Workspace Home's own draft-Application row
// (Flow 2 canvas re-audit, ROADMAP.md, 2026-09-27), which needs to know a proposal exists and its
// id, not its full conversation. target is a plain string rather than an aiassist.KindNewApplication
// reference: internal/data must not import internal/aiassist (business logic), so the caller
// supplies the value it already knows.
func (s *Store) ListAISessionsByStatus(ctx context.Context, workspaceID, target, status string) ([]AISession, error) {
	readLogFrom(ctx).record("ai sessions by status")
	rows, err := s.pool.Query(ctx, `
		SELECT id, workspace_id, actor_user_id, target, status
		FROM ai_sessions
		WHERE workspace_id = $1 AND target = $2 AND status = $3
		ORDER BY created_at
	`, workspaceID, target, status)
	if err != nil {
		return nil, fmt.Errorf("list ai sessions by status: %w", err)
	}
	defer rows.Close()

	var sessions []AISession
	for rows.Next() {
		var session AISession
		if err := rows.Scan(&session.ID, &session.WorkspaceID, &session.ActorUserID, &session.Target, &session.Status); err != nil {
			return nil, fmt.Errorf("scan ai session: %w", err)
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
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
// **workspaceID is in the WHERE, not just in the caller.** Until 2026-09-29 this was `WHERE id = $1`,
// and discardNewApplication passed the id straight from its URL with no GetAISession first -- so a
// Workspace admin could discard another Workspace's draft Application by id. Its four sibling handlers
// all resolved a scoped session first and were unaffected, which is exactly why the store is the right
// place for the predicate: relying on every caller to scope is a rule that holds until one does not,
// and one did not. Same posture installer.RefuseIfExists states for itself -- a guard that holds even
// when a future caller is built wrong.
func (s *Store) UpdateAISessionStatus(ctx context.Context, workspaceID, id, status string) error {
	ct, err := s.pool.Exec(ctx, `
		UPDATE ai_sessions SET status = $3, updated_at = NOW() WHERE id = $2 AND workspace_id = $1
	`, workspaceID, id, status)
	if err != nil {
		return fmt.Errorf("update ai session status: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrRecordNotFound
	}
	return nil
}

// MarkAISessionPublished moves a session to 'published' and records when, and which application it
// created or updated (migration 018). It is UpdateAISessionStatus for the one transition that carries
// facts of its own, kept separate so the facts cannot be forgotten at a call site that only meant to
// change the status.
func (s *Store) MarkAISessionPublished(ctx context.Context, workspaceID, id, applicationID string) error {
	ct, err := s.pool.Exec(ctx, `
		UPDATE ai_sessions
		   SET status = $4, updated_at = NOW(), published_at = COALESCE(published_at, NOW()), application_id = $3
		 WHERE id = $2 AND workspace_id = $1
	`, workspaceID, id, applicationID, AISessionStatusPublished)
	if err != nil {
		return fmt.Errorf("mark ai session published: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrRecordNotFound
	}
	return nil
}

// AIApplicationOrigin is when the assistant first published an application in this Workspace: the
// earliest publication of a new_application session for it. ok is false for an application the
// assistant did not create (a library install, a hand-written file) or one published before migration
// 018 -- the age of those is unknown, and the caller must say so rather than report zero.
func (s *Store) AIApplicationOrigin(ctx context.Context, workspaceID, applicationID string) (published time.Time, ok bool, err error) {
	var t *time.Time
	if err := s.pool.QueryRow(ctx, `
		SELECT min(published_at) FROM ai_sessions
		 WHERE workspace_id = $1 AND application_id = $2 AND target = 'new_application' AND status = 'published'
	`, workspaceID, applicationID).Scan(&t); err != nil {
		return time.Time{}, false, fmt.Errorf("ai application origin: %w", err)
	}
	if t == nil {
		return time.Time{}, false, nil
	}
	return *t, true, nil
}

// AICapabilityGap is one gap as stored: what the assistant could not build, written as a specification
// for whoever builds the capability (migration 021). Kind is "runtime", "assistant" or "unclassified"
// and Class is one of migration 021's classes or "unclassified"; the CHECKs refuse anything else.
type AICapabilityGap struct {
	Kind            string
	Requested       string
	Note            string
	CapabilityKey   string
	Class           string
	IdealCapability string
	WantedShape     string
	Where           string
	Intent          string
	Benchmark       string
}

// RecordAICapabilityGap writes one structured "the runtime can't say this yet" row (the kajian's
// own §3.4 ask) -- never updated or deleted once written, so a later count across many
// conversations reflects real, unedited history.
func (s *Store) RecordAICapabilityGap(ctx context.Context, sessionID, workspaceID string, g AICapabilityGap) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO ai_capability_gaps (id, session_id, workspace_id, kind, requested_capability, note,
			capability_key, class, ideal_capability, wanted_shape, location, intent, benchmark)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, newID("aigap_"), sessionID, workspaceID, g.Kind, g.Requested, g.Note,
		g.CapabilityKey, g.Class, g.IdealCapability, g.WantedShape, g.Where, g.Intent, g.Benchmark)
	if err != nil {
		return fmt.Errorf("record ai capability gap: %w", err)
	}
	return nil
}
