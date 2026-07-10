-- Migration 079: stone rate rebalance
--
-- The building stone rates were 10-20x every other building's output and pegged
-- stone at cap in every soak: mine 60/tick and stonequarry 120/tick against
-- farm grain 6/tick. That made stone a non-decision (always abundant) and
-- crowded the signal in economy readouts.
--
-- Lower the two building rules to a moderate band (~2x/4x grain): stone stays
-- the cheap bulk building material but stops pegging, so allocating labour to a
-- quarry becomes a real choice. Terrain baselines (hills 0.6, mountain 1.2) are
-- left untouched. No settlement_goods rewrite: the next RecomputeProduction for
-- each settlement rewrites its stone rate from these rules (no stock is lost -
-- settled() extrapolates on read using the stored rate until then).
--
--   mine         60 -> 12
--   stonequarry  120 -> 24

UPDATE production_rules SET rate_per_tick = 12 WHERE building_type = 'mine'        AND good_key = 'stone';
UPDATE production_rules SET rate_per_tick = 24 WHERE building_type = 'stonequarry' AND good_key = 'stone';
