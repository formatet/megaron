-- Revert migration 129: restore post-109 stone building rates (079's placement, ×24 units).

UPDATE production_rules SET rate_per_tick = 288 WHERE building_type = 'mine'        AND good_key = 'stone';
UPDATE production_rules SET rate_per_tick = 576 WHERE building_type = 'stonequarry' AND good_key = 'stone';
