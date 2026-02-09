-- Track provider health check and sync state persistently.
ALTER TABLE provider_configs ADD COLUMN last_check_at   TEXT    NOT NULL DEFAULT '';
ALTER TABLE provider_configs ADD COLUMN last_check_ok   INTEGER NOT NULL DEFAULT 0;
ALTER TABLE provider_configs ADD COLUMN last_check_err  TEXT    NOT NULL DEFAULT '';
ALTER TABLE provider_configs ADD COLUMN last_sync_at    TEXT    NOT NULL DEFAULT '';
ALTER TABLE provider_configs ADD COLUMN consec_fails    INTEGER NOT NULL DEFAULT 0;
