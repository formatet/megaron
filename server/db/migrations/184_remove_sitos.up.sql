-- Timothy 2026-10-18: one food inventory per city. Return every stored good
-- without truncation, even for an over-cap row or a missing city-good row.
-- Settle at the settlement's OWN world tick (also correct for archived worlds);
-- this is settled(amount,rate,calc_tick) on the active world, without relying on
-- the global active-world function for historical worlds.
-- The ceiling is raised to at least the returned stock, so API/recompute/next
-- credit cannot clip the returned food immediately. Rate and silver unchanged.
INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
SELECT g.settlement_id, g.good_key,
       GREATEST(0, COALESCE(sg.amount + sg.rate * GREATEST(0, w.current_tick - sg.calc_tick), 0)) + g.amount,
       COALESCE(sg.rate, 0),
       GREATEST(COALESCE(sg.cap, 1000000),
                GREATEST(0, COALESCE(sg.amount + sg.rate * GREATEST(0, w.current_tick - sg.calc_tick), 0)) + g.amount),
       w.current_tick
FROM settlement_granary g
JOIN settlements s ON s.id = g.settlement_id
JOIN worlds w ON w.id = s.world_id
LEFT JOIN settlement_goods sg ON sg.settlement_id = g.settlement_id AND sg.good_key = g.good_key
ON CONFLICT (settlement_id, good_key) DO UPDATE
SET amount = EXCLUDED.amount, rate = EXCLUDED.rate, cap = EXCLUDED.cap, calc_tick = EXCLUDED.calc_tick;

-- Retired work is complete, not failed. The runtime also accepts any old
-- SitosTick still delivered after deployment as a no-op, without rescheduling.
UPDATE scheduled_events SET processed_at = now(), failed_at = NULL
WHERE event_type = 'SitosTick' AND processed_at IS NULL;
DROP TABLE settlement_granary;
DROP TABLE processed_sitos_ticks;
