-- Spearmen stop drawing silver. Preserve every currently owned coin before
-- freezing active hosts' lazy rates; use each world's tick, not the global
-- active-world accessor. Both goods share calc_tick, so settle both tuples.
UPDATE founder_phase fp
SET silver_amount = GREATEST(0, fp.silver_amount + fp.silver_rate * GREATEST(0, w.current_tick - fp.calc_tick)),
    grain_amount = GREATEST(0, fp.grain_amount + fp.grain_rate * GREATEST(0, w.current_tick - fp.calc_tick)),
    silver_rate = 0,
    calc_tick = w.current_tick
FROM worlds w
WHERE w.id = fp.world_id AND fp.active AND fp.silver_rate <> 0;

-- A live salary warning cannot remain on a unit that no longer earns salary.
-- Do not alter manpower, status, carried reserves or historic events.
UPDATE units SET unpaid_periods = 0
WHERE type = 'spearman' AND unpaid_periods <> 0
  AND status IN ('garrison','marching','positioned','embarked','repairing','freighting');
