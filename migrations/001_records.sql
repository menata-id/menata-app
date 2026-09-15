-- +goose Up
-- 001_records.sql
-- Generic Data Plane record storage: one row per Machine record, field values stored as JSONB.
-- A logical field may be physically realized as a JSONB value, an expression index, a generated
-- column, or another strategy without changing business metadata
-- (007-composable-runtime-architecture.md SS22 "Physical Storage Strategy"). JSONB is the
-- starting physical strategy; per-Machine schema generation is a later runtime concern.

CREATE TABLE IF NOT EXISTS records (
    id         TEXT PRIMARY KEY,
    machine_id TEXT NOT NULL,
    data       JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_records_machine_id ON records (machine_id);

-- +goose Down
DROP TABLE IF EXISTS records;
