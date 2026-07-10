-- Revert migration 079: restore pre-rebalance stone building rates (post-071 x60 values).

UPDATE production_rules SET rate_per_tick = 60  WHERE building_type = 'mine'        AND good_key = 'stone';
UPDATE production_rules SET rate_per_tick = 120 WHERE building_type = 'stonequarry' AND good_key = 'stone';
