-- Historical floor calibration: internal/kharis/tick.go kharisFloor = 1.0.
-- Only a living, ungrounded Host with no active owned city is repaired.
-- Reset its old geographic rate and anchor the lazy tuple in its own world.
UPDATE player_world_records pwr
SET kharis_amount = 1.0,
    kharis_rate = 0,
    kharis_calc_tick = w.current_tick
FROM worlds w
WHERE w.id = pwr.world_id
  AND pwr.status = 'active'
  AND pwr.kharis_amount < 1.0
  AND EXISTS (SELECT 1 FROM founder_phase fp
              WHERE fp.owner_id = pwr.player_id AND fp.world_id = pwr.world_id
                AND fp.active AND fp.founded_tick IS NULL)
  AND NOT EXISTS (SELECT 1 FROM settlements s
                  WHERE s.owner_id = pwr.player_id AND s.world_id = pwr.world_id AND s.state = 'active');
