# Inventory (step 0) — böner steg 5a

No STOP condition hit. Findings and the two places the order is adapted:

## 1. Population growth (fertility)
- One SQL owner: `economy.PopulationGrowthSQL()` (`economy/population_growth.go`), a correlated `settlements s` projection with
  `growth_per_tick = LEAST(pop_cap-pop, GREATEST(1, ROUND(pop*0.005*variety)))`. It is consumed by BOTH `kharis/tick.go applyDecay`
  (the mutation, `growth_calc` CTE) and `LoadNextCitizen` (status). So one factor in that SQL reaches growth AND the surfaces; starvation
  (`unmet>0`) and `food_net<=0` are separate branches and stay untouched ("only growth").
- Tick used in SQL is `current_world_tick()` (variety already uses it). The factor is a subquery over `prayer_blessings`
  (effect `blessed_fertility`, scope city/realm, `owner_id = s.owner_id`, `from_tick <= t < until_tick`).
- G1: `economy` may NOT import `religion`. The SQL therefore names the effect as an economy constant (`economy.FertilityEffect`); a handlers
  test pins it equal to `religion.EffectBlessedFertility`. The factor 1.5 is `economy.FertilityGrowthFactor`.

## 2. Herd births
- `economy.HerdBirths(herd, grain, herdCap, tick)` is pure; applied by `economy/food_tick.go herdBirths(ctx, tx, sid, tick)` in the food
  tick's per-settlement TX. Fed ticks every 5, graze ticks every 20 (`HerdGrazeBirthEvery`), so a 20-tick window always holds one graze tick.
- Factor applied to `born` only (floor, never fewer); `fed` (the grain actually paid) is unchanged — the extra calves are the blessing's.
  `NextHerdBirth`/`HerdSurplusRate` (forecasts) are not scaled (a forecast of an unannounced blessing); noted, not a surface the order asks for.

## 3. Build queue (mig 245 model)
- The queue is `build_queue` with `work_remaining/work_rate/work_calc_tick/work_revision`; SQL `sync_build_queue(city)` is the ONE owner of the clock:
  it settles the active row under the OLD rate, re-issues `BuildComplete` stamped with a new `work_revision`, and starts the oldest waiting row.
  `combat.BuildCompleteHandler` ignores any event whose revision is not the row's -> "the old completion does nothing" already exists.
  Re-queueing is therefore safe (no STOP).
- Ships: `units.shipyard_work_*` with SQL `sync_shipyard_work(ship)` and the same revision rule in `combat.ShipyardWorkCompleteHandler`.
  The same function also serves 'repairing'; the blessing is limited to `status='forming'` (building a ship; a repair is not construction).
- Due tick today = `calc_tick + ceil(work/rate)`; remaining time T = ceil(work/rate). New T' = max(1, ceil(0.75*T)); stored as
  work' = (T'-1)*rate+1 so `ceil(work'/rate) = T'` at the current rate.
- Design: ONE code path. `sync_*` itself scales a row once (new marker columns `build_queue.sped`, `units.shipyard_sped`) when a live
  `blessed_building` row covers the city at `current_tick`. The cast inserts the blessing and calls `sync_build_queue`/`sync_shipyard_work`
  for each covered city's running work; a build that starts later (enqueue, completion of the previous one) reaches the same branch.
  Waiting rows are not touched until they start (order). Mig 251.

## 4. Divine punishment
- `kharis/tick.go applyDivinePunishment(settlementID, world, owner)`: dice rolled by the caller; inside the function the shield row
  (`kharis_notice_state` condition `divine_punishment`, `FOR UPDATE`) is read, a city and a feasible punishment are chosen, the SQL takes the loss,
  then the shield `until = anchor.Tick + WrathProtectionTicks` is written with the `DivinePunishment` event + notification, one TX.
- The scapegoat is checked right after the punishment is chosen (so a feasible loss exists and the dice are unchanged) and before the take SQL:
  one `UPDATE prayer_blessings SET until_tick=<now> WHERE id = (the live row ... FOR UPDATE)` spends it (row kept for audit; same TX and the same
  `kharis_notice_state` lock as the punishment). Casts are serialised per player by the Rite handler's `player_world_records ... FOR UPDATE`.
- The row is `prayer_blessings` scope 'realm' effect `scapegoat`; `prayer_blessing_wards_used` needs a unit and is not used.

## 5. Surfaces found
- Cast: `api/handlers/settlement.go Rite -> checkBlessingTarget/applyBlessing`; catalogue `cult_prayers.go` (`targets`, `scope_cost_factors`).
- Order deviations: `kind` in `sped_up` is `"build"` for the queue and `"ship"` for a shipyard job (the order names only "build").
