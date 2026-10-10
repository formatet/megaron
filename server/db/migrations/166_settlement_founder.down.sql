DROP INDEX IF EXISTS idx_settlements_founder;
ALTER TABLE settlements DROP COLUMN founded_tick, DROP COLUMN founder_id;
