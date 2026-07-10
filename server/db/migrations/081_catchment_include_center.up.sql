-- Migration 081: catchment now includes the settlement's own hex.
--
-- Catchment changes from the 6 adjacent tiles to the 7 tiles the city works:
-- its own hex + the 6 adjacent (W3 revised, Timothy 2026-07-10). RecomputeProduction
-- and every producible/deposit query now read the centre map_tile as well.
--
-- Existing settlement_goods rates are settled into `amount` then nulled so they
-- recompute (including the centre hex) next tick; settlement_labor is cleared so
-- auto-seeding re-picks the producible set from the full 7-tile catchment. Mirrors
-- migration 051, updated for the tick substrate (mig 067: calc_at → calc_tick INT,
-- settled() takes a tick, now() → current_world_tick()).

UPDATE settlement_goods
SET amount    = LEAST(cap, settled(amount, rate, calc_tick)),
    rate      = 0,
    calc_tick = current_world_tick();

DELETE FROM settlement_labor;
