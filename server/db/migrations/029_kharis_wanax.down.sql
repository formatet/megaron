-- Reverse migration 029: move kharis back to settlements.
ALTER TABLE settlements
  ADD COLUMN kharis_amount  FLOAT NOT NULL DEFAULT 0,
  ADD COLUMN kharis_rate    FLOAT NOT NULL DEFAULT 0.0,
  ADD COLUMN kharis_cap     FLOAT NOT NULL DEFAULT 2000,
  ADD COLUMN kharis_calc_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN cult_level     TEXT NOT NULL DEFAULT 'enkel';

UPDATE settlements s SET
  kharis_amount  = pwr.kharis_amount,
  kharis_rate    = pwr.kharis_rate,
  kharis_cap     = pwr.kharis_cap,
  kharis_calc_at = pwr.kharis_calc_at,
  cult_level     = pwr.cult_level
FROM player_world_records pwr
WHERE s.owner_id = pwr.player_id AND s.world_id = pwr.world_id;

ALTER TABLE player_world_records
  DROP COLUMN kharis_amount,
  DROP COLUMN kharis_rate,
  DROP COLUMN kharis_cap,
  DROP COLUMN kharis_calc_at,
  DROP COLUMN cult_level;
