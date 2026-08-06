-- Migration 109: the tick IS the day — scale production to the new unit.
--
-- Canon 2026-08-06 (Timothy): a tick is the world's indivisible unit of time and
-- represents one day. events.TicksPerDay goes 24 → 1, so every macro handler
-- (upkeep, loyalty decay, welfare, colony penalty, kharis) now fires once per
-- tick instead of once per 24, and consumption is charged per tick.
--
-- production_rules.rate_per_tick held per-game-HOUR rates. Against per-tick
-- consumption they are now 24× too low — the exact shape of the bug mig 071
-- fixed one unit down (rates were per-MINUTE against an hourly tick: 60× too
-- low, universal starvation from tick 1). Same fix, same reason: scale the rates
-- to the new meaning of a tick.
--
--   mig 071:  rate_per_min  × 60  → per game-hour
--   mig 109:  rate_per_tick × 24  → per day (= per tick)
--
-- The column name needs no change this time: it was already "per tick", and it
-- stays true. What changed underneath is what a tick means.

UPDATE production_rules SET rate_per_tick = rate_per_tick * 24;

-- ── settlement_goods.rate — SETTLE BEFORE RESCALING ─────────────────────────────
--
-- settlement_goods carries the materialised lazy tuple (amount, rate, calc_tick)
-- and is read through settled(amount, rate, calc_tick) =
--   amount + rate × GREATEST(0, current_world_tick() − calc_tick)
--
-- Writing a new rate WITHOUT settling first would re-evaluate the whole elapsed
-- span (current_tick − calc_tick) at the NEW rate — silently rewriting history
-- and handing every settlement 24× the goods it actually accrued. So: fold the
-- accrual into amount at the OLD rate, stamp calc_tick to now, and only then
-- scale the rate. All three in ONE statement so no tick can land between them.
--
-- Grain's rate is a NET rate (population consumption is folded in, so it can be
-- negative). Both sides of that sum scale by 24, so scaling the net is correct —
-- this is not an approximation.
--
-- cap is an absolute stock ceiling, not a rate, and is deliberately untouched:
-- LEAST(cap, …) still clamps exactly as before. GREATEST(0, …) mirrors the
-- floor the read path already applies.
--
-- ⚠️ settled() is NOT used here, deliberately. It resolves the current tick via
-- current_world_tick(), which is
--     SELECT current_tick FROM worlds WHERE status = 'active' LIMIT 1
-- and therefore returns NULL whenever no world is active — writing NULL into a
-- NOT NULL calc_tick and failing the whole migration. That is not hypothetical:
-- it failed exactly this way on the first run against a database holding only
-- non-active worlds. A migration must not depend on a world being in a
-- particular state at the moment it runs.
--
-- Joining each settlement to its OWN world is both NULL-safe and strictly more
-- correct: it settles every row against the tick that row actually accrued
-- against. Under single-world enforcement the two are the same number, so
-- production behaviour is identical to the settled() form.

UPDATE settlement_goods sg
SET amount    = LEAST(sg.cap,
                      GREATEST(0, sg.amount + sg.rate * GREATEST(0, w.current_tick - sg.calc_tick))),
    rate      = sg.rate * 24,
    calc_tick = w.current_tick
FROM settlements s
JOIN worlds w ON w.id = s.world_id
WHERE s.id = sg.settlement_id;

-- Sanity: no NULLs introduced, no rate left unscaled. A row with rate = 0 is
-- legitimate (a good with no production and no consumption) and needs no note.
