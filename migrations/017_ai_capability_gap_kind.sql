-- +goose Up
-- 017_ai_capability_gap_kind.sql
-- Roadmap stage 1, WG (owner decision 2026-10-15: the gap record is read from the log by the development
-- process, never from an admin page). A gap now says WHICH kind of gap it is:
--   runtime    -- the runtime cannot express it yet (a capability to build);
--   assistant  -- the runtime can, but the assistant cannot write it yet (an assistant slot to build);
--   unclassified -- rows written before this column existed (they were never classified, and are not guessed).
-- The table stays append-only; this adds a column and nothing is rewritten.
ALTER TABLE ai_capability_gaps
    ADD COLUMN kind text NOT NULL DEFAULT 'unclassified'
    CHECK (kind IN ('runtime', 'assistant', 'unclassified'));

-- +goose Down
ALTER TABLE ai_capability_gaps DROP COLUMN kind;
