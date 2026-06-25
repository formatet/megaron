-- Down: reverse migration 067 — restore wall-clock substrate.

-- Restore sim_config singleton (wall-clock time-scale factor).
CREATE TABLE IF NOT EXISTS sim_config (
    id     int PRIMARY KEY DEFAULT 1,
    factor float NOT NULL DEFAULT 1.0,
    CONSTRAINT one_row CHECK (id = 1)
);
INSERT INTO sim_config (id, factor) VALUES (1, 1.0) ON CONFLICT DO NOTHING;

-- Drop tick-based settled(); restore timestamptz version.
DROP FUNCTION IF EXISTS settled(double precision, double precision, int);
CREATE FUNCTION settled(p_amount double precision, p_rate double precision, p_calc_at timestamptz)
RETURNS double precision LANGUAGE sql STABLE AS $$
    SELECT p_amount + EXTRACT(EPOCH FROM (now() - p_calc_at))/60 * p_rate
             * (SELECT factor FROM sim_config LIMIT 1)
$$;

-- Restore kingdoms.silver_calc_tick → silver_calc_at TIMESTAMPTZ
ALTER TABLE kingdoms RENAME COLUMN silver_calc_tick TO silver_calc_at;
ALTER TABLE kingdoms
    ALTER COLUMN silver_calc_at TYPE timestamptz USING now(),
    ALTER COLUMN silver_calc_at SET DEFAULT now();

-- Restore player_world_records.kharis_calc_tick → kharis_calc_at TIMESTAMPTZ
ALTER TABLE player_world_records RENAME COLUMN kharis_calc_tick TO kharis_calc_at;
ALTER TABLE player_world_records
    ALTER COLUMN kharis_calc_at TYPE timestamptz USING now(),
    ALTER COLUMN kharis_calc_at SET DEFAULT now();

-- Restore settlement_goods.calc_tick → calc_at TIMESTAMPTZ
ALTER TABLE settlement_goods RENAME COLUMN calc_tick TO calc_at;
ALTER TABLE settlement_goods
    ALTER COLUMN calc_at TYPE timestamptz USING now(),
    ALTER COLUMN calc_at SET DEFAULT now();

-- Drop current_world_tick().
DROP FUNCTION IF EXISTS current_world_tick();
