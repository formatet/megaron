-- Migration 021: store player-chosen colony name on the march
ALTER TABLE marching_armies ADD COLUMN colony_name TEXT;
