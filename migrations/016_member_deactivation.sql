-- +goose Up
-- 016_member_deactivation.sql
-- Member deactivation (Flow 2 canvas re-audit, ROADMAP.md, 2026-09-27) -- the same shape
-- migrations/013_workspace_archive.sql already established for a Workspace, applied here to one
-- membership row instead: deactivated_at is nullable and cleared on reactivate, not a log of
-- every deactivate/reactivate cycle, since Edit Member wants exactly one current state.
ALTER TABLE workspace_members ADD COLUMN deactivated_at timestamptz;

-- +goose Down
ALTER TABLE workspace_members DROP COLUMN deactivated_at;
