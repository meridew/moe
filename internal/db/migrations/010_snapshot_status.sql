-- 010_snapshot_status.sql
-- Add status tracking for async baseline captures.

ALTER TABLE policy_snapshots ADD COLUMN status TEXT NOT NULL DEFAULT 'complete';
ALTER TABLE policy_snapshots ADD COLUMN status_message TEXT NOT NULL DEFAULT '';
