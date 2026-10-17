-- Timothy's inland-food order, Claude 2026-10-17: an ordinary inland city
-- must feed ten workers with at most seven on food. Tylissos already has
-- its L1 farm; the shortfall is hills' base grain rate, not a missing gift.
-- Keep the shared hex rule, all other terrain/goods and labor_value unchanged.
-- DEPLOY: run cmd/recompute-all after migration (also on rollback). Stored
-- settlement_goods rates are projections and are not read from this catalogue
-- until RecomputeProduction runs. It settles accrued output at the OLD rate.
UPDATE production_rules SET rate_per_tick = 0.5
 WHERE good_key = 'grain' AND terrain_type = 'hills' AND building_type IS NULL;
