-- +goose Up
-- 004_workspaces.sql
-- Workspace boundary reaches storage (ROADMAP.md Phase 21 Step 2). `domain.Workspace` has been
-- parsed from app.yaml since Phase 2 but never enforced anywhere -- every record of every Machine
-- lived in one unscoped pool. Backfilling every existing row to 'ws_default' (metadata/app.yaml's
-- own hardcoded Workspace id) means no live data moves or changes meaning; Principle #11 (Data
-- Preservation) applied literally, per this repo's own established practice (e.g. 002_sort_order.sql).

CREATE TABLE IF NOT EXISTS workspaces (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    slug       TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO workspaces (id, name, slug) VALUES ('ws_default', 'Default Workspace', 'default')
    ON CONFLICT (id) DO NOTHING;

ALTER TABLE records ADD COLUMN workspace_id TEXT NOT NULL DEFAULT 'ws_default';

DROP INDEX IF EXISTS idx_records_machine_sort;
CREATE INDEX IF NOT EXISTS idx_records_workspace_machine_sort ON records (workspace_id, machine_id, sort_order);
CREATE INDEX IF NOT EXISTS idx_records_workspace_machine ON records (workspace_id, machine_id);

-- +goose Down
DROP INDEX IF EXISTS idx_records_workspace_machine;
DROP INDEX IF EXISTS idx_records_workspace_machine_sort;
CREATE INDEX IF NOT EXISTS idx_records_machine_sort ON records (machine_id, sort_order);
ALTER TABLE records DROP COLUMN workspace_id;
DROP TABLE IF EXISTS workspaces;
