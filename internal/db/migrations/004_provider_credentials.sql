-- 004_provider_credentials.sql
-- Add credential columns to provider_configs for API authentication.

ALTER TABLE provider_configs ADD COLUMN client_id     TEXT NOT NULL DEFAULT '';
ALTER TABLE provider_configs ADD COLUMN client_secret TEXT NOT NULL DEFAULT '';
