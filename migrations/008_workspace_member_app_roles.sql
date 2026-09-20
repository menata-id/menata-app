-- +goose Up
-- 008_workspace_member_app_roles.sql
-- One role per member *per Application* (ROADMAP.md Case 03 Fase 3b). Until now a membership
-- carried a single workspace_members.app_role, which was correct only while a Workspace held
-- exactly one Application; Fase 3a made `applications:` a real list, so one column can no longer
-- say which Application a role belongs to.
--
-- application_id is the Application's declared id (app_xxx), never the file path its declaration
-- lives in (004 Stable Identity) -- renaming metadata/applications/*.yaml must not orphan a
-- member's roles. It is deliberately NOT a foreign key: Applications are metadata, not rows, so
-- there is no table to point at, and the same is already true of machine_id on records.
--
-- role is likewise a declared vocabulary word (an Application's own roles: list), validated at the
-- write path against the Application that owns it rather than by a database constraint -- the
-- vocabulary lives in YAML and can change without a migration.
--
-- "No role" is the ABSENCE of a row, not a sentinel: SetMemberAppRole deletes rather than storing
-- ''. That keeps "has no role here" and "has a role that happens to be empty" from being two
-- different states meaning the same thing, which is what the old NULLIF($5, '') column had to
-- paper over.
CREATE TABLE IF NOT EXISTS workspace_member_app_roles (
    workspace_id   TEXT NOT NULL REFERENCES workspaces(id),
    user_record_id TEXT NOT NULL,
    application_id TEXT NOT NULL,
    role           TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (workspace_id, user_record_id, application_id)
);

-- Backfill: every existing app_role belongs to Document Approval. That is not a guess -- the
-- values stored are approver/submitter/reviewer, which is precisely the vocabulary
-- metadata/applications/document-approval.yaml declares, and the only Application that existed
-- when they were written. Project Management starts with no role rows, which is the honest state:
-- nobody has ever been assigned one.
INSERT INTO workspace_member_app_roles (workspace_id, user_record_id, application_id, role)
SELECT workspace_id, user_record_id, 'app_document_approval', app_role
FROM workspace_members
WHERE app_role IS NOT NULL AND app_role <> ''
ON CONFLICT DO NOTHING;

-- workspace_members.app_role is deliberately left in place, still written alongside the new table
-- by the data layer. Dropping it here would make this migration irreversible in practice: `goose
-- down` would restore the column but not its values. It is dropped in a later migration, once the
-- new table has run in anger.

-- +goose Down
DROP TABLE IF EXISTS workspace_member_app_roles;
