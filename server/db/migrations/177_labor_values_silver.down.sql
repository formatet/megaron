DROP TABLE divine_labor_valuations;
ALTER TABLE founder_phase DROP COLUMN is_refugee;
UPDATE settlement_goods SET amount=GREATEST(0,settled(amount,rate,calc_tick)),
    calc_tick=current_world_tick(), rate=rate*2.304 WHERE good_key='silver' AND rate>0;
UPDATE production_rules SET rate_per_tick=CASE terrain_type
    WHEN 'hills' THEN 2.304/1.7 WHEN 'mountain_limestone' THEN 5.76/1.7 END
WHERE good_key='silver' AND building_type='mine' AND terrain_type IN ('hills','mountain_limestone');
ALTER TABLE goods DROP COLUMN labor_value;
COMMENT ON COLUMN goods.base_value IS NULL;
-- Existing silver balances are conserved; do not restore old caps below stock.
