-- +goose Up
-- 022_app_drafts.sql
-- The changes an admin has made in the Application Builder and not published yet (Apps Builder plan, D3): one row per Workspace, Application and admin
-- holding the pending operations as JSON, so "3 changes not published" is true after the admin closes the tab and nobody else's draft is touched. The
-- operations are builder.Op values; the row is the only place they live -- publishing replays them through the one write path and deletes the row.
CREATE TABLE IF NOT EXISTS app_drafts (
    workspace_id  TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    app_id        TEXT NOT NULL,
    user_id       TEXT NOT NULL,
    ops           JSONB NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, app_id, user_id)
);

-- +goose Down
DROP TABLE IF EXISTS app_drafts;
