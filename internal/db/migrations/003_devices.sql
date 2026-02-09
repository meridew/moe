-- 003_devices.sql
-- Devices table: cached device records synced from MDM providers.

CREATE TABLE IF NOT EXISTS devices (
    id              TEXT PRIMARY KEY,
    provider_name   TEXT NOT NULL,
    provider_type   TEXT NOT NULL CHECK(provider_type IN ('uem', 'intune')),
    source_id       TEXT NOT NULL DEFAULT '',
    device_name     TEXT NOT NULL DEFAULT '',
    os              TEXT NOT NULL DEFAULT '',
    os_version      TEXT NOT NULL DEFAULT '',
    model           TEXT NOT NULL DEFAULT '',
    user_name       TEXT NOT NULL DEFAULT '',
    user_email      TEXT NOT NULL DEFAULT '',
    compliance      TEXT NOT NULL DEFAULT 'unknown' CHECK(compliance IN ('compliant', 'non-compliant', 'unknown')),
    last_seen       DATETIME,
    last_synced_at  DATETIME,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(provider_name, source_id)
);

CREATE INDEX idx_devices_provider   ON devices(provider_name);
CREATE INDEX idx_devices_os         ON devices(os);
CREATE INDEX idx_devices_compliance ON devices(compliance);
CREATE INDEX idx_devices_user_email ON devices(user_email);
