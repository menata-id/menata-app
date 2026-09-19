-- +goose Up
-- 005_workspace_members.sql
-- Joins a login identity (credentials, keyed by email) to a real mch_user record within one
-- Workspace, with that Workspace's own role plus this slice's single-Application role
-- (ROADMAP.md Phase 21 Step 3). Login identity is workspace-independent -- a person can belong to
-- more than one Workspace (Phase 21 Step 4's Choose Workspace) -- while a Workspace's own
-- mch_user record is not, hence the join rather than a column on either side.
--
-- email is deliberately NOT a foreign key into credentials: an invited member (Phase 21 Step 6)
-- gets a membership row before they have ever set a password, by design (no email-sending
-- infrastructure exists to drive a token-based invite flow) -- their first login attempt, finding
-- no credential, is routed to "set your password" instead of "invalid password".

CREATE TABLE IF NOT EXISTS workspace_members (
    workspace_id   TEXT NOT NULL REFERENCES workspaces(id),
    user_record_id TEXT NOT NULL,
    email          TEXT NOT NULL,
    workspace_role TEXT NOT NULL,
    app_role       TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (workspace_id, user_record_id)
);
CREATE INDEX IF NOT EXISTS idx_workspace_members_email ON workspace_members (email);

-- +goose Down
DROP TABLE IF EXISTS workspace_members;
