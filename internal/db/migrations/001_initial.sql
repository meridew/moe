-- 001_initial.sql
-- Base schema: settings table for app-level config and future use.

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO settings (key, value) VALUES ('schema_version', '1');
