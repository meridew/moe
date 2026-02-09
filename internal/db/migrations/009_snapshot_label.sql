-- 009_snapshot_label.sql
-- Add optional display label to policy snapshots so users can name baselines.

ALTER TABLE policy_snapshots ADD COLUMN label TEXT NOT NULL DEFAULT '';
