-- +goose Up
-- 019_document_extractions.sql
-- What a document reader proposed for a form (`extract:` on a page's create Form): the stored document, the proposal as
-- JSON, and who asked. A proposal is not a record -- nothing in it has passed a Machine's rules -- so it is a bespoke table
-- like ai_sessions, with its own Store methods (internal/data/extractions.go). It is what a page opened with
-- ?proposal=<id> fills its form from, which keeps the URL short and the file key out of the browser's hands, and it is the
-- ledger of readings per Workspace (one row per document read) a usage quota would count.
CREATE TABLE IF NOT EXISTS document_extractions (
    id            TEXT PRIMARY KEY,
    workspace_id  TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id       TEXT NOT NULL,
    -- nav_id is the navigation item whose Form asked; a proposal opens only on that page.
    nav_id        TEXT NOT NULL,
    machine_id    TEXT NOT NULL,
    file_key      TEXT NOT NULL,
    file_name     TEXT NOT NULL,
    proposal      JSONB NOT NULL,
    -- status: proposed -> saved (the form was submitted; record_id is what it made).
    status        TEXT NOT NULL DEFAULT 'proposed',
    record_id     TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    saved_at      TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS document_extractions_workspace_created ON document_extractions (workspace_id, created_at);

-- +goose Down
DROP TABLE IF EXISTS document_extractions;
