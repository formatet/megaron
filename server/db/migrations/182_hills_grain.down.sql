-- Recompute owned settlements after rollback too; see the up migration.
UPDATE production_rules SET rate_per_tick = 1.0 / 3.0
 WHERE good_key = 'grain' AND terrain_type = 'hills' AND building_type IS NULL;
