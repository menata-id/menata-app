-- +goose Up
-- 014_notification_preferences.sql
-- Notifications (Flow 2 gap study Tahap 6) -- email preferences are identity-level, on
-- credentials (keyed by email, not per-Workspace), matching the identity model this repo already
-- holds ("name/email/credential are identity data") and the mockup's own framing (an Account tab,
-- not a Workspace setting). Two triggers, two columns, default true to match the mockup's own
-- default-checked toggles -- the in-app notification is written unconditionally either way; these
-- gate only whether an email is ALSO sent.
ALTER TABLE credentials ADD COLUMN notify_assigned boolean NOT NULL DEFAULT true;
ALTER TABLE credentials ADD COLUMN notify_decided boolean NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE credentials DROP COLUMN notify_decided;
ALTER TABLE credentials DROP COLUMN notify_assigned;
