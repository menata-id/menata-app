-- +goose Up
-- 023_app_drafts_base_version.sql
-- The state of the Workspace's installation (builder.Version) the first change of a draft was staged against. Publishing a draft whose base is no longer the live
-- version is refused, so a draft never replays on top of what another admin published since, silently overwriting it.
ALTER TABLE app_drafts ADD COLUMN IF NOT EXISTS base_version TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE app_drafts DROP COLUMN IF EXISTS base_version;
