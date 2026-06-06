-- Bronsålderns terrassodling: hills får baseline grain-produktion.
-- Rate 0.01/min (1/3 av plains 0.03) — spelbar start utan att trivialisera handel.
INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_min)
VALUES ('hills', NULL, 'grain', 0.01);

-- Initialisera settlement_goods + settlement_labor för befintliga hills-bosättningar
-- så att varan syns direkt (rate=0 tills Wanax allokerar medborgare).
INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
SELECT s.id, 'grain', 0, 0, 1000, now()
FROM settlements s
JOIN provinces p ON p.id = s.province_id
WHERE p.terrain_type = 'hills'
  AND s.owner_id IS NOT NULL
ON CONFLICT (settlement_id, good_key) DO NOTHING;

INSERT INTO settlement_labor (settlement_id, good_key, citizens)
SELECT s.id, 'grain', 0
FROM settlements s
JOIN provinces p ON p.id = s.province_id
WHERE p.terrain_type = 'hills'
  AND s.owner_id IS NOT NULL
ON CONFLICT (settlement_id, good_key) DO NOTHING;
