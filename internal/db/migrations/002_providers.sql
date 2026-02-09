-- 002_providers.sql
-- Provider configurations: each row is a configured MDM tenant connection.

CREATE TABLE IF NOT EXISTS provider_configs (
    id             TEXT PRIMARY KEY,
    name           TEXT NOT NULL UNIQUE,
    type           TEXT NOT NULL CHECK(type IN ('uem', 'intune')),
    base_url       TEXT NOT NULL DEFAULT '',
    tenant_id      TEXT NOT NULL DEFAULT '',
    sync_interval  TEXT NOT NULL DEFAULT '15m',
    enabled        INTEGER NOT NULL DEFAULT 1,
    created_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_provider_configs_type ON provider_configs(type);
