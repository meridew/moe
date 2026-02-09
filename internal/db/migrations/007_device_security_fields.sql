-- 007_device_security_fields.sql
-- Add security posture fields from Intune managedDevice (available at no extra API cost).
-- Provider-agnostic: UEM can populate equivalent fields later.

ALTER TABLE devices ADD COLUMN is_encrypted  BOOLEAN NOT NULL DEFAULT 0;
ALTER TABLE devices ADD COLUMN jail_broken   TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN is_supervised BOOLEAN NOT NULL DEFAULT 0;
ALTER TABLE devices ADD COLUMN threat_state  TEXT NOT NULL DEFAULT '';
