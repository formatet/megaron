-- Rollback restores future host pay, without deleting or recreating money.
-- Cleared unpaid counters and old audit events are deliberately not replayed.
UPDATE founder_phase fp
SET silver_amount = GREATEST(0, fp.silver_amount + fp.silver_rate * GREATEST(0, w.current_tick - fp.calc_tick)),
    grain_amount = GREATEST(0, fp.grain_amount + fp.grain_rate * GREATEST(0, w.current_tick - fp.calc_tick)),
    silver_rate = CASE WHEN fp.is_refugee THEN -1 ELSE -2 END,
    calc_tick = w.current_tick
FROM worlds w
WHERE w.id = fp.world_id AND fp.active;
