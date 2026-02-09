-- 008_policy_snapshots.sql
-- Policy snapshot and items tables for browsing/comparing provider policies.

CREATE TABLE IF NOT EXISTS policy_snapshots (
    id             TEXT PRIMARY KEY,
    provider_name  TEXT NOT NULL,
    provider_type  TEXT NOT NULL,
    taken_at       DATETIME NOT NULL DEFAULT (datetime('now')),
    policy_count   INTEGER NOT NULL DEFAULT 0,
    category_count INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (provider_name) REFERENCES provider_configs(name)
);

CREATE TABLE IF NOT EXISTS policy_items (
    id            TEXT PRIMARY KEY,
    snapshot_id   TEXT NOT NULL,
    category      TEXT NOT NULL,  -- "compliance", "configuration", "app-protection", etc.
    source_id     TEXT NOT NULL,  -- ID within the source system
    policy_name   TEXT NOT NULL,
    policy_type   TEXT NOT NULL DEFAULT '',  -- OData type or classification
    platform      TEXT NOT NULL DEFAULT '',  -- "Windows", "iOS", "Android", "All", ""
    description   TEXT NOT NULL DEFAULT '',
    settings_json TEXT NOT NULL DEFAULT '{}', -- full JSON blob of settings
    FOREIGN KEY (snapshot_id) REFERENCES policy_snapshots(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_policy_items_snapshot ON policy_items(snapshot_id);
CREATE INDEX IF NOT EXISTS idx_policy_items_category ON policy_items(snapshot_id, category);
