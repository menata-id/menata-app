-- +goose Up
-- 020_number_counters.sql
-- The last number the runtime handed out under one prefix of a numbered Field (domain.FieldNumbering). A number is the highest
-- one still on a record plus one, which reuses a number once its record is deleted -- a contract number or a payment plan
-- number that was already printed, signed or paid against. The counter only ever goes up: the next number is the larger of the
-- counter and the highest still held, plus one. A Workspace's counters go with the Workspace.
CREATE TABLE IF NOT EXISTS number_counters (
    workspace_id  TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    machine_id    TEXT NOT NULL,
    field_id      TEXT NOT NULL,
    prefix        TEXT NOT NULL,
    last          BIGINT NOT NULL,
    PRIMARY KEY (workspace_id, machine_id, field_id, prefix)
);

-- +goose Down
DROP TABLE IF EXISTS number_counters;
