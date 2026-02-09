-- 005_uem_credentials.sql
-- Add UEM-specific credential columns for basic auth (username/password).

ALTER TABLE provider_configs ADD COLUMN username TEXT NOT NULL DEFAULT '';
ALTER TABLE provider_configs ADD COLUMN password TEXT NOT NULL DEFAULT '';
