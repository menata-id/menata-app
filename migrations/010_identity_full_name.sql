-- +goose Up
-- 010_identity_full_name.sql
-- Full name becomes identity data, not Workspace data (owner decision, 2026-09-22).
--
-- Until now a person's name lived on their `mch_user` record, which is scoped per Workspace, so
-- the same human carried a separately-typed name in every Workspace they belonged to -- entered
-- again at registration, again when creating a second Workspace, and *by someone else* when an
-- admin invited them (the inviting admin typed the invitee's name into the invite form). The
-- owner's rule is that a full name is the right of the person who owns the email: one source of
-- reference for name, email and credential together, and only its owner may change it.
--
-- So the name moves next to the two things that were already identity-level: email (this table's
-- own key) and the password hash. `mch_user` keeps only genuinely Workspace-scoped attributes
-- (weekly capacity) and its role as the referent every assignee relation points at.
--
-- Stripping fld_name/fld_email out of the existing records is NOT optional cleanup, and it must
-- happen in the same migration that removes them from metadata/user.yaml: internal/data's
-- ValidateRecord rejects any value naming a Field the Machine does not declare, so a record still
-- carrying those keys would fail its *next* write -- which, for mch_user, is the Profile screen
-- reading a record and writing it back whole. Nothing is lost by it either: the names are copied
-- into credentials.full_name first, immediately above.
--
-- The backfill picks one name per email deterministically (oldest membership wins) because in
-- principle the same identity could hold differently-typed names in two Workspaces. Checked
-- against the real data at write time, no identity had more than one distinct name, so no actual
-- row was resolved by that tie-break.

ALTER TABLE credentials ADD COLUMN full_name TEXT NOT NULL DEFAULT '';

UPDATE credentials c
SET full_name = sub.full_name
FROM (
    SELECT DISTINCT ON (wm.email) wm.email, r.data->>'fld_name' AS full_name
    FROM workspace_members wm
    JOIN records r ON r.id = wm.user_record_id AND r.machine_id = 'mch_user'
    WHERE COALESCE(r.data->>'fld_name', '') <> ''
    ORDER BY wm.email, wm.created_at
) sub
WHERE c.email = sub.email;

UPDATE records SET data = data - 'fld_name' - 'fld_email' WHERE machine_id = 'mch_user';

-- +goose Down
-- Writes the identity's name and email back onto every mch_user record that a membership links to
-- it, which is where they lived before. A record whose email holds no credential gets nothing
-- back, because there is no longer anywhere that name could have come from.
UPDATE records r
SET data = r.data || jsonb_build_object('fld_name', c.full_name, 'fld_email', c.email)
FROM workspace_members wm
JOIN credentials c ON c.email = wm.email
WHERE r.id = wm.user_record_id AND r.machine_id = 'mch_user';

ALTER TABLE credentials DROP COLUMN full_name;
