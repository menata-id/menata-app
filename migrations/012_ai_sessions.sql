-- +goose Up
-- 012_ai_sessions.sql
-- The AI Metadata Assistant's own conversation history (Flow 2 gap study Tahap 8,
-- menata-app-document's audits/2026-09-25-kajian-new-application-ai.md).
--
-- Three bespoke tables, not the generic records JSONB store: a conversation turn is not a Machine
-- record (it has no declared Field shape and never passes through ValidateRecord), so it follows
-- workspace_groups/workspace_members's own precedent instead -- a real table, its own Store
-- methods (internal/data/aisessions.go).
--
-- ai_sessions/ai_session_turns mirror this repo's existing "persist early as a real, incomplete
-- row, then GetRecord/UpdateRecord across requests" convention (the Document submit wizard's own
-- draft -> continue-submit shape) -- a conversation is a real row from its first message, not an
-- ephemeral blob threaded through hidden form fields.
--
-- ai_capability_gaps is append-only by construction (nothing in internal/data ever updates or
-- deletes a row here) -- the structured record the kajian's own §3.4 asked for: every time the
-- assistant tells a user "the runtime can't say this yet", one row, separate from the free-text
-- turn, so a pattern across many conversations is a GROUP BY away rather than a prose-parsing
-- exercise. Deliberately NOT scoped to one workspace_id column for its own queries (though the
-- column exists, for context) -- the whole point of this table is noticing a request repeated
-- *across* workspaces, the same "Application-level, not Workspace-level" posture
-- metadata-hot-reload-safety.md's own reload trigger already takes.
CREATE TABLE IF NOT EXISTS ai_sessions (
    id             TEXT PRIMARY KEY,
    workspace_id   TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    actor_user_id  TEXT NOT NULL,
    -- target is "new_application", or the id of the Application an extend_application session is
    -- proposing a change to -- see aiassist.GeneratedChange.Kind/TargetAppID. Free text rather than
    -- a foreign key: an Application id names a metadata file, not a database row (the same reason
    -- workspace_member_app_roles.application_id is not a foreign key either).
    target         TEXT NOT NULL,
    -- status: open (conversation in progress) -> generated (a validated GeneratedChange exists) ->
    -- published (written to disk and reloaded) or discarded.
    status         TEXT NOT NULL DEFAULT 'open',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ai_session_turns (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL REFERENCES ai_sessions(id) ON DELETE CASCADE,
    role        TEXT NOT NULL, -- "user" or "model"
    content     TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ai_capability_gaps (
    id                    TEXT PRIMARY KEY,
    session_id            TEXT NOT NULL REFERENCES ai_sessions(id) ON DELETE CASCADE,
    workspace_id          TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    requested_capability  TEXT NOT NULL,
    note                  TEXT NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ai_sessions_workspace ON ai_sessions (workspace_id);
CREATE INDEX IF NOT EXISTS idx_ai_session_turns_session ON ai_session_turns (session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_ai_capability_gaps_capability ON ai_capability_gaps (requested_capability);

-- +goose Down
DROP TABLE IF EXISTS ai_capability_gaps;
DROP TABLE IF EXISTS ai_session_turns;
DROP TABLE IF EXISTS ai_sessions;
