-- +goose Up
-- 007_session_generation.sql
-- Server-side session revocation (security audit 2026-09-19, M2): the session cookie is a
-- stateless signed value with no server-side store, so there was previously no way to invalidate
-- one already issued -- logout only cleared the cookie client-side (a copied/stolen cookie
-- remained valid), and a password reset didn't invalidate whatever session existed before it.
--
-- Keyed by the session cookie's own "subject" string (an mch_user record id, or the shared admin
-- credential's placeholder identity) rather than by email: a subject is exactly what a cookie
-- carries and what requireAuth checks on every request, and a single email can hold more than one
-- subject (one mch_user record per Workspace membership) -- tracking by subject instead avoids
-- needing to resolve email->all-its-subjects on every authenticated request, only when a
-- credential actually changes (BumpSessionGeneration is called once per affected subject then,
-- not on every read).
CREATE TABLE IF NOT EXISTS session_generations (
    subject    TEXT PRIMARY KEY,
    generation INTEGER NOT NULL DEFAULT 0
);

-- +goose Down
DROP TABLE IF EXISTS session_generations;
