-- +goose Up
-- 006_email_verified.sql
-- Blocking email verification on self-registration (ROADMAP.md Phase 21 round 2, Step D):
-- closes "anyone can register a workspace with any email" by requiring proof of ownership before
-- the account can be used. Scoped to self-registration only -- an invited member's own first-login
-- activation (Step 6) sets this true immediately, since a Workspace Admin already vouches for that
-- specific email by typing it in themselves; a different trust model, deliberately not given the
-- same friction.

ALTER TABLE credentials ADD COLUMN email_verified BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE credentials DROP COLUMN email_verified;
