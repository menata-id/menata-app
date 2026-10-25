-- +goose Up
-- 021_ai_capability_gap_detail.sql
-- A gap is a specification for the people who build the capability, not a note of refusal (plan
-- 2026-10-25, phase A1; owner decisions 2026-10-15 log-only, 2026-10-25 ideal capability required).
-- Each column holds what the assistant records for one thing it could not build:
--   capability_key   -- stable slug, the unit gaps are counted by (the free-text requested_capability varies);
--   class            -- ui_component | workflow_logic | data_model | configuration | integration;
--   ideal_capability -- what the runtime ideally should be able to do, in general terms;
--   wanted_shape     -- a sketch of how metadata would declare it;
--   location         -- the Machine or screen where the need arose (the word "where" is reserved in SQL);
--   intent           -- the person's own words, briefly;
--   benchmark        -- the counterpart in a common UI framework or workflow engine, when there is one.
-- The table stays append-only: rows written before this migration keep empty strings and class
-- 'unclassified'; nothing is rewritten and nothing is guessed for them.
ALTER TABLE ai_capability_gaps
    ADD COLUMN capability_key   text NOT NULL DEFAULT '',
    ADD COLUMN class            text NOT NULL DEFAULT 'unclassified'
        CHECK (class IN ('ui_component', 'workflow_logic', 'data_model', 'configuration', 'integration', 'unclassified')),
    ADD COLUMN ideal_capability text NOT NULL DEFAULT '',
    ADD COLUMN wanted_shape     text NOT NULL DEFAULT '',
    ADD COLUMN location         text NOT NULL DEFAULT '',
    ADD COLUMN intent           text NOT NULL DEFAULT '',
    ADD COLUMN benchmark        text NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_ai_capability_gaps_key ON ai_capability_gaps (capability_key);

-- +goose Down
DROP INDEX IF EXISTS idx_ai_capability_gaps_key;
ALTER TABLE ai_capability_gaps
    DROP COLUMN benchmark, DROP COLUMN intent, DROP COLUMN location, DROP COLUMN wanted_shape,
    DROP COLUMN ideal_capability, DROP COLUMN class, DROP COLUMN capability_key;
