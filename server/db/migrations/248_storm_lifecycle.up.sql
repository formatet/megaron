-- Stormar föds och dör (megaron_plan_stormar.md §Stormar föds och dör, Timothy 2026-11-07).
-- dies_tick: the tick the storm ends, rolled ONCE at birth (200-300 ticks of life) and stored.
-- died_tick: set by the weather step when it has passed (3b: also by a prayer). The row stays:
-- name, claimed_by, map memories and the track written before death are all kept.
ALTER TABLE sea_storms ADD COLUMN dies_tick INT;
ALTER TABLE sea_storms ADD COLUMN died_tick INT;
-- The living world: spread the ages so the storms do not all die together. The outcome is
-- stored here (random() is fine in a migration; it is rolled once, not re-derived).
UPDATE sea_storms s SET dies_tick = w.current_tick + 1 + floor(random() * 300)::int
  FROM worlds w WHERE w.id = s.world_id;
ALTER TABLE sea_storms ALTER COLUMN dies_tick SET NOT NULL;

-- The last tick the weather (storm steps + births) was rolled for. A birth roll that leaves
-- no trace (no birth) must not be rolled again by a duplicate scan, and a world whose storms
-- have all died has no track to anchor on.
ALTER TABLE worlds ADD COLUMN storm_weather_tick INT;
UPDATE worlds w SET storm_weather_tick = (
  SELECT max(t.tick) FROM sea_storm_track t JOIN sea_storms s ON s.id = t.storm_id WHERE s.world_id = w.id);
