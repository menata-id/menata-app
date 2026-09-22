-- +goose Up
-- 011_pending_invites.sql
-- An invitation is not a membership (owner decision, 2026-09-22).
--
-- Until now `submitInviteMember` wrote the `workspace_members` row *and* an `mch_user` record the
-- moment an invite was sent, and accepting only created the credential -- `submitAcceptInvite`
-- actually *required* the membership to already exist. So someone who had agreed to nothing was
-- already a member: listed on Workspace Members, selectable as an approver, and carrying a record
-- with their name on it typed by the admin who invited them.
--
-- Membership now begins when the invitation is accepted. What sits here in the meantime is the
-- invitation itself: who was asked, into which Workspace, with which roles waiting for them.
--
-- A separate table rather than a status column on workspace_members, and the reason is specific
-- to this codebase: workspace_members is the table authorization reads (requireWorkspaceAdmin via
-- GetMembership, requireApplicationAccess via the actor's roles). A pending row living there
-- would mean every authorization path must remember to exclude it, forever, and this repo has
-- already been bitten by exactly that shape -- see internal/web/middleware.go's own account of
-- requireWorkspaceAdmin having failed open, "the shape of an authorization gate that admits
-- whatever it fails to identify". Here there is no filter to forget: an unaccepted invitation is
-- not in the members table at all.
--
-- This also retires migration 005's own stated reason for the old design ("no email-sending
-- infrastructure exists to drive a token-based invite flow"). It does now: internal/mail sends a
-- real, workspace-bound, 7-day invite token (internal/web/invite.go).
--
-- app_roles is JSONB ({application_id: role}) rather than a child table: it is written once when
-- the invite is sent and read once when it is accepted, never queried across rows, and it moves
-- into workspace_member_app_roles at acceptance -- which is where per-Application roles are
-- actually queried.

CREATE TABLE IF NOT EXISTS pending_invites (
    workspace_id   TEXT NOT NULL REFERENCES workspaces(id),
    email          TEXT NOT NULL,
    workspace_role TEXT NOT NULL,
    app_roles      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (workspace_id, email)
);
CREATE INDEX IF NOT EXISTS idx_pending_invites_email ON pending_invites (email);

-- Existing memberships whose email holds no credential are exactly the old model's unaccepted
-- invitations, so they become rows here and stop being memberships. Their mch_user records and
-- role rows go with them: under the new model those are created at acceptance, and leaving them
-- behind would orphan a record no identity stands behind.
INSERT INTO pending_invites (workspace_id, email, workspace_role, app_roles, created_at)
SELECT wm.workspace_id,
       wm.email,
       wm.workspace_role,
       COALESCE((
           SELECT jsonb_object_agg(ar.application_id, ar.role)
           FROM workspace_member_app_roles ar
           WHERE ar.workspace_id = wm.workspace_id AND ar.user_record_id = wm.user_record_id
       ), '{}'::jsonb),
       wm.created_at
FROM workspace_members wm
WHERE NOT EXISTS (SELECT 1 FROM credentials c WHERE c.email = wm.email)
ON CONFLICT (workspace_id, email) DO NOTHING;

DELETE FROM workspace_member_app_roles ar
WHERE EXISTS (
    SELECT 1 FROM workspace_members wm
    WHERE wm.workspace_id = ar.workspace_id
      AND wm.user_record_id = ar.user_record_id
      AND NOT EXISTS (SELECT 1 FROM credentials c WHERE c.email = wm.email)
);

-- Keyed by (group_id, user_record_id) with no workspace_id of its own, so this matches on the
-- record id alone -- which is globally unique, so no Workspace correlation is needed.
DELETE FROM workspace_group_members gm
WHERE EXISTS (
    SELECT 1 FROM workspace_members wm
    WHERE wm.user_record_id = gm.user_record_id
      AND NOT EXISTS (SELECT 1 FROM credentials c WHERE c.email = wm.email)
);

DELETE FROM records r
WHERE r.machine_id = 'mch_user'
  AND EXISTS (
      SELECT 1 FROM workspace_members wm
      WHERE wm.user_record_id = r.id
        AND NOT EXISTS (SELECT 1 FROM credentials c WHERE c.email = wm.email)
  );

DELETE FROM workspace_members wm
WHERE NOT EXISTS (SELECT 1 FROM credentials c WHERE c.email = wm.email);

-- +goose Down
-- Invitations cannot become memberships again on the way down: the mch_user record a membership
-- row must name is created at acceptance under the new model, and inventing one here would
-- fabricate a member nobody invited into existence. Unaccepted invitations are simply dropped.
DROP TABLE IF EXISTS pending_invites;
