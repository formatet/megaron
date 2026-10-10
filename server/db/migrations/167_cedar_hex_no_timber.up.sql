-- Cederskog ger långt mer virke (megaron_plan_cedar_virke.md, Timothy 2026-10-10):
-- a cedar hex is worked for CEDAR only (3 base places, economy/hex_rules.go). The
-- field-timber row (mig 102, 155) gave timber a fallback of 2 places on the same
-- hex, so one gubbe could be set on both goods. Cedar now covers a timber
-- shortfall in costs instead (api/handlers deductGoods).

-- Gubbar already placed on timber there go to the pool (the player re-places them).
DELETE FROM settlement_placement sp
 USING settlements s, provinces p
 WHERE sp.settlement_id = s.id
   AND p.world_id = s.world_id AND p.map_q = sp.hex_q AND p.map_r = sp.hex_r
   AND sp.target_kind = 'hex' AND sp.good_key = 'timber'
   AND p.terrain_type = 'forest_cedar';

DELETE FROM production_rules
 WHERE terrain_type = 'forest_cedar' AND building_type IS NULL AND good_key = 'timber';
