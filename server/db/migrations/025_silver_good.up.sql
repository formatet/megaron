-- Add silver as a tradeable good (weight-metal, the Bronze Age payment medium).
-- Base value = 1 (silver is the numeraire — everything else priced in silver).
-- Weight = 2 (heavy, like copper).
-- Expand tier/category constraints to include precious metals.
ALTER TABLE goods DROP CONSTRAINT IF EXISTS goods_tier_check;
ALTER TABLE goods DROP CONSTRAINT IF EXISTS goods_category_check;
ALTER TABLE goods ADD CONSTRAINT goods_tier_check
    CHECK (tier IN ('commodity','manufactured','precious'));
ALTER TABLE goods ADD CONSTRAINT goods_category_check
    CHECK (category IN ('staple','strategic','prestige','bulk','precious'));

INSERT INTO goods (key, name, base_value, weight, tier, category)
VALUES ('silver', 'Silver', 1, 2, 'precious', 'precious')
ON CONFLICT (key) DO NOTHING;

-- Add silver_deposit flag to provinces (analogous to copper_deposit/tin_deposit).
ALTER TABLE provinces ADD COLUMN IF NOT EXISTS silver_deposit BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE map_tiles  ADD COLUMN IF NOT EXISTS silver_deposit BOOLEAN NOT NULL DEFAULT false;

-- Production rules for silver: mine on mountain/hills with silver_deposit.
INSERT INTO production_rules (good_key, building_type, terrain_type, rate_per_min, requires_deposit)
SELECT 'silver', 'mine', 'mountain', 0.02, 'silver'
WHERE NOT EXISTS (
    SELECT 1 FROM production_rules WHERE good_key='silver' AND building_type='mine' AND terrain_type='mountain'
);
INSERT INTO production_rules (good_key, building_type, terrain_type, rate_per_min, requires_deposit)
SELECT 'silver', 'mine', 'hills', 0.008, 'silver'
WHERE NOT EXISTS (
    SELECT 1 FROM production_rules WHERE good_key='silver' AND building_type='mine' AND terrain_type='hills'
);

-- Seed settlement_goods rows for silver (amount=0) for all active settlements.
INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
SELECT s.id, 'silver', 0, 0, 500, now()
FROM settlements s
WHERE s.state != 'sunk'
ON CONFLICT (settlement_id, good_key) DO NOTHING;
