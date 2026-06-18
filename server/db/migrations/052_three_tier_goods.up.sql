-- Migration 052: W4a — 3-lagersvaror. Komplettera katalogen med prestige/iteration-
-- varorna purple (kustmurex), pottery (verkstadsbulk) och luxury (förädlad prestige).
--
-- 3-skiktsmappning (dokumentation — category-kolumnen bär den):
--   bas        = grain, fish, oil, cedar, timber, stone, horses
--   strategisk = copper, tin, bronze, pottery
--   prestige   = wine, purple, luxury, silver
--
-- purple + pottery får produktionsregler här. luxury craftas via recept (mig 053).

INSERT INTO goods (key, name, tier, category, base_value, weight) VALUES
    ('purple',  'Purple',  'commodity',    'prestige', 18.0, 1.0),
    ('pottery', 'Pottery', 'commodity',    'strategic', 2.5, 2.0),
    ('luxury',  'Luxury',  'manufactured', 'prestige', 30.0, 1.0)
ON CONFLICT (key) DO NOTHING;

-- purple: kustnära murex (granne till hav). Labor-skalad som alla varor.
INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_min, requires_coastal, requires_deposit)
VALUES (NULL, NULL, 'purple', 0.015, TRUE, NULL);

-- pottery: verkstadsbulk — kräver market (urban verkstad/handelshub). Terräng-agnostisk.
INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_min, requires_coastal, requires_deposit)
VALUES (NULL, 'market', 'pottery', 0.03, FALSE, NULL);
