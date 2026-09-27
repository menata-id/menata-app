-- +goose Up
-- 015_notify_sla_breach_preference.sql
-- The SLA-breach reminder's own preference column (Flow 2 canvas re-audit, 2026-09-27) -- the
-- third notify_* column on credentials, same shape as migration 014's two: identity-level, keyed
-- by email, default true. Gates only whether the reminder ALSO sends an email; the in-app
-- mch_notification row is written unconditionally, same as the other two triggers.
ALTER TABLE credentials ADD COLUMN notify_sla_breach boolean NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE credentials DROP COLUMN notify_sla_breach;
