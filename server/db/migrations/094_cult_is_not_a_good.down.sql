-- Återställer kult som producerad vara (mig 055: tempel-labor → cult).
-- settlement_goods-raderna återskapas av RecomputeProduction vid nästa tick.
INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_tick, requires_coastal, requires_deposit)
VALUES (NULL, 'temple', 'cult', 0.05, FALSE, NULL);
