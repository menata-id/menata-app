-- +goose Up
-- 003_credentials.sql
-- Real per-user login credentials (ROADMAP.md Phase 21 Step 1). Deliberately not a Machine
-- Field on mch_user: a password_hash Field would flow through the generic create/edit form and
-- the generic record detail page, leaking the hash and letting any authenticated identity
-- overwrite any user's password via the existing generic PUT route. Keyed by email rather than
-- a record id because login identity is workspace-independent (a person can belong to more than
-- one Workspace, ROADMAP.md Phase 21 Step 4) while a Workspace's own mch_user record is not.

CREATE TABLE IF NOT EXISTS credentials (
    email         TEXT PRIMARY KEY,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS credentials;
