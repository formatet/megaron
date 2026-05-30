-- Reverse of 018: restore food/lumber/stone columns (does not restore data).
ALTER TABLE settlements
    ADD COLUMN IF NOT EXISTS food_amount   DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS food_rate     DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS food_cap      DOUBLE PRECISION NOT NULL DEFAULT 1000,
    ADD COLUMN IF NOT EXISTS food_calc_at  TIMESTAMPTZ      NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS lumber_amount DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS lumber_rate   DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS lumber_cap    DOUBLE PRECISION NOT NULL DEFAULT 500,
    ADD COLUMN IF NOT EXISTS lumber_calc_at TIMESTAMPTZ     NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS stone_amount  DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS stone_rate    DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS stone_cap     DOUBLE PRECISION NOT NULL DEFAULT 500,
    ADD COLUMN IF NOT EXISTS stone_calc_at TIMESTAMPTZ      NOT NULL DEFAULT now();

DELETE FROM production_rules WHERE building_type IN ('mine', 'stonequarry') AND good_key = 'stone';
DELETE FROM production_rules WHERE terrain_type IN ('forest', 'hills', 'mountain') AND building_type IS NULL AND good_key IN ('cedar', 'stone');

UPDATE production_rules SET rate_per_min = 0.05
    WHERE building_type = 'farm' AND good_key = 'grain';
UPDATE production_rules SET rate_per_min = 0.05
    WHERE building_type = 'lumbermill' AND good_key = 'cedar';

DELETE FROM goods WHERE key = 'stone';
