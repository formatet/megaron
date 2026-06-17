-- One-row config bridging Go's TIME_SCALE env into SQL. main.go syncs it at boot.
CREATE TABLE IF NOT EXISTS sim_config (
    id     int PRIMARY KEY DEFAULT 1,
    factor double precision NOT NULL DEFAULT 1,
    CONSTRAINT sim_config_singleton CHECK (id = 1)
);
INSERT INTO sim_config (id, factor) VALUES (1, 1) ON CONFLICT (id) DO NOTHING;

-- Canonical lazy-eval. Single source of truth. Scales elapsed real-time by the
-- time-compression factor so production keeps pace with scaled durations.
CREATE OR REPLACE FUNCTION settled(p_amount double precision, p_rate double precision, p_calc_at timestamptz)
RETURNS double precision LANGUAGE sql STABLE AS $$
    SELECT p_amount + EXTRACT(EPOCH FROM (now() - p_calc_at))/60 * p_rate
           * (SELECT factor FROM sim_config WHERE id = 1)
$$;
