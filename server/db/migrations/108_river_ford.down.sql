-- Rollback of 108: drop the river_ford fish rule. Does NOT revert the coastal
-- backfill (a superset of TRUE values that were already correct under the old
-- definition is harmless to leave set) and does NOT touch any river/river_ford
-- terrain values — this migration's up side never wrote terrain, only
-- production_rules and the coastal flag (same contract as mig 101's down).
DELETE FROM production_rules
 WHERE terrain_type = 'river_ford' AND good_key = 'fish';
