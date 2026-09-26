-- +goose Up
-- 013_workspace_archive.sql
-- Workspace lifecycle (Flow 2 gap study Tahap 7, menata-app-document's
-- audits/2026-09-23-kajian-gap-mockup-flow2.md §5.2) -- archive/restore only, per the owner's own
-- scope decision (2026-09-26): "Transfer ownership" (the mockup's Danger Zone also names it) stays
-- a placeholder, since there is no singular Workspace Owner concept today to transfer, and no
-- second real case forcing one yet.
--
-- archived_at is nullable and cleared on restore (not append-only history) -- Choose Workspace's
-- own archived row wants exactly one date ("archived 12 Jul 2026"), the moment of the *current*
-- archival, not a log of every archive/restore cycle a Workspace has been through.
ALTER TABLE workspaces ADD COLUMN archived boolean NOT NULL DEFAULT false;
ALTER TABLE workspaces ADD COLUMN archived_at timestamptz;

-- +goose Down
ALTER TABLE workspaces DROP COLUMN archived_at;
ALTER TABLE workspaces DROP COLUMN archived;
