-- Migration 029: kharis pool moves from settlements to player_world_records.
-- Each Wanax has ONE kharis relationship with their gods, not one per settlement.
-- cult_level also moves here — it is a per-Wanax choice, not per-settlement.

ALTER TABLE player_world_records
  ADD COLUMN kharis_amount  FLOAT NOT NULL DEFAULT 0,
  ADD COLUMN kharis_rate    FLOAT NOT NULL DEFAULT 0.0,
  ADD COLUMN kharis_cap     FLOAT NOT NULL DEFAULT 2000,
  ADD COLUMN kharis_calc_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN cult_level     TEXT NOT NULL DEFAULT 'enkel';

-- Copy from capital settlement (is_capital = true) so we don't lose the live state.
UPDATE player_world_records pwr SET
  kharis_amount  = GREATEST(0,
      s.kharis_amount + (EXTRACT(EPOCH FROM (now() - s.kharis_calc_at))/60 * s.kharis_rate)),
  kharis_rate    = s.kharis_rate,
  kharis_cap     = s.kharis_cap,
  kharis_calc_at = now(),
  cult_level     = s.cult_level
FROM settlements s
WHERE s.owner_id = pwr.player_id
  AND s.world_id = pwr.world_id
  AND s.is_capital = true;

ALTER TABLE settlements
  DROP COLUMN kharis_amount,
  DROP COLUMN kharis_rate,
  DROP COLUMN kharis_cap,
  DROP COLUMN kharis_calc_at,
  DROP COLUMN cult_level;
