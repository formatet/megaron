-- Reverse of 109. Same ordering hazard, mirrored: settle at the CURRENT (scaled)
-- rate before dividing it back, or the elapsed span is re-evaluated at the old
-- rate and every settlement silently loses 23/24 of what it accrued.
--
-- ⚠️ Not a clean round trip on a running world: settle() truncates against cap
-- and floors at 0, so a settlement that was clamped at cap during the up-migration
-- cannot recover the overflow. Reverting is for a dev wipe, not for live data.

UPDATE settlement_goods sg
SET amount    = LEAST(sg.cap,
                      GREATEST(0, sg.amount + sg.rate * GREATEST(0, w.current_tick - sg.calc_tick))),
    rate      = sg.rate / 24,
    calc_tick = w.current_tick
FROM settlements s
JOIN worlds w ON w.id = s.world_id
WHERE s.id = sg.settlement_id;

UPDATE production_rules SET rate_per_tick = rate_per_tick / 24;
