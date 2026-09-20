-- +goose Up
-- 009_workspace_groups.sql
-- Groups: assign a role to a Group once, then add and remove members, instead of touching every
-- person's own row (ROADMAP.md Case 03 Fase 4).
--
-- Mirrors menata-runtime's own shipped 023_groups.sql (CAP-O07), adapted to this app: ids follow
-- the newID("grp_") convention rather than UUIDs (data.CreateWorkspace sets that precedent with
-- newID("ws_")), and application_id is not a foreign key because Applications here are metadata
-- files, not rows -- the same reason records.machine_id and workspace_member_app_roles's own
-- application_id are not.
--
-- ONE ROLE PER GROUP PER APPLICATION (the UNIQUE below), exactly as upstream has it. A person can
-- still end up holding several roles in one Application -- by belonging to several Groups, or by
-- one direct assignment plus Group membership -- but that set is produced by MERGING at read time
-- (data.EffectiveRoles), never stored. Upstream's own comment on this table says the same thing:
-- "see UserStore's session-resolution merge, not this table". That is why 008's
-- workspace_member_app_roles keeps its one-row-per-(member, application) shape unchanged.
--
-- ON DELETE CASCADE throughout, as upstream has it. Worth noting because 008's table is the odd
-- one out: it has no cascade, so deleting a workspace fails on a foreign key unless its role rows
-- are cleared first -- which is exactly what broke the test cleanup helpers and sent Fase 3b's
-- first push red. These three tables add no such burden.
CREATE TABLE IF NOT EXISTS workspace_groups (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS workspace_group_members (
    group_id       TEXT NOT NULL REFERENCES workspace_groups(id) ON DELETE CASCADE,
    user_record_id TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (group_id, user_record_id)
);

CREATE TABLE IF NOT EXISTS workspace_group_app_roles (
    group_id       TEXT NOT NULL REFERENCES workspace_groups(id) ON DELETE CASCADE,
    application_id TEXT NOT NULL,
    role           TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (group_id, application_id)
);

CREATE INDEX IF NOT EXISTS idx_workspace_groups_workspace ON workspace_groups (workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspace_group_members_user ON workspace_group_members (user_record_id);

-- +goose Down
DROP TABLE IF EXISTS workspace_group_app_roles;
DROP TABLE IF EXISTS workspace_group_members;
DROP TABLE IF EXISTS workspace_groups;
