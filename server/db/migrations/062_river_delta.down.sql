-- Revert migration 062: remove river_delta production rules.
-- Note: existing map_tiles with terrain='river_delta' remain but produce nothing
-- (no matching production rule) — this is safe for rollback.
DELETE FROM production_rules WHERE terrain_type = 'river_delta';
