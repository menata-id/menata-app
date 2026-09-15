-- +goose Up
-- 002_sort_order.sql
-- Explicit ordering within a Machine's records, needed for meaningfully-ordered child
-- collections (checklist items, approval steps -- ROADMAP.md Phase 9) and, later, manual
-- drag-reorder (Phase 10). Defaults every existing row to 0; new rows get the next value per
-- Machine (Store.CreateRecord).

ALTER TABLE records ADD COLUMN sort_order BIGINT NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_records_machine_sort ON records (machine_id, sort_order);

-- +goose Down
DROP INDEX IF EXISTS idx_records_machine_sort;
ALTER TABLE records DROP COLUMN sort_order;
