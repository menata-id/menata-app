-- +goose Up
-- 018_ai_session_publication.sql
-- Roadmap stage 1, WU (owner direction 2026-10-15): what people change in an application soon after the
-- assistant built it shows what its first generation lacked or overdid. That needs two facts the table
-- never held: when a session was published and which application it published or updated.
--   published_at    -- set once, when the session reaches 'published'; NULL for every earlier session
--   application_id  -- the application the session created or updated; NULL until published
-- The age of an application is then the published_at of the earliest new_application session that
-- published it. Nothing is rewritten; existing rows keep NULL and the reader says "unknown", not zero.
ALTER TABLE ai_sessions ADD COLUMN published_at timestamptz;
ALTER TABLE ai_sessions ADD COLUMN application_id text;
CREATE INDEX IF NOT EXISTS idx_ai_sessions_application ON ai_sessions (workspace_id, application_id) WHERE application_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_ai_sessions_application;
ALTER TABLE ai_sessions DROP COLUMN application_id;
ALTER TABLE ai_sessions DROP COLUMN published_at;
