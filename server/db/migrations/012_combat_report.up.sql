-- Migration 012: Add combat_report text column to marching_armies.
-- Stores a human-readable battle summary generated once at resolution.
ALTER TABLE marching_armies ADD COLUMN IF NOT EXISTS combat_report TEXT;
