-- +goose Up
-- 024_event_runs.sql
-- What a rule (an Event of a Machine) did each time it ran (Apps Builder plan, Fase 10): when, on which record, whether it ran, did nothing because there was nobody
-- or nothing to act on, or failed, and a line saying what. It is the history that tells a rule that is quiet from one that is broken. Written after the rule's Service has run,
-- best effort (a rule never fails the write it follows); the newest rows per Machine are kept and the rest trimmed, so the table does not grow without bound.
CREATE TABLE IF NOT EXISTS event_runs (
    id            BIGSERIAL PRIMARY KEY,
    workspace_id  TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    machine_id    TEXT NOT NULL,
    event_id      TEXT NOT NULL,
    record_id     TEXT NOT NULL,
    outcome       TEXT NOT NULL,
    detail        TEXT NOT NULL DEFAULT '',
    ran_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS event_runs_by_rule ON event_runs (workspace_id, machine_id, event_id, ran_at DESC);

-- +goose Down
DROP TABLE IF EXISTS event_runs;
