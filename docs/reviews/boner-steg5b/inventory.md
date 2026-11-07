# Inventory (step 0) — böner steg 5b

No STOP condition hit. First round is deterministic on replay. No migration needed (prayer_blessings has no CHECK on effect; mover_id/mover_kind 'transport' exist since mig 247).

## 1. First round of a battle (the bull's horns)
- `combat/battle.go resolveTick(ctx, tx, battleID, worldID, tickIndex)`: `tickIndex = e.DueTick`, rounds are `for round := 1..battleRoundsPerTick`, dice seeded `battleRoundSeed(seed, tickIndex, round)`.
  `rollSide` has no round argument. Rows land in `battle_rounds(battle_id, tick_index, round_index)` with `ON CONFLICT DO NOTHING`.
- "First round" = `round == 1` of a tick for which `NOT EXISTS (battle_rounds WHERE battle_id=$1 AND tick_index < tickIndex)`. This reads stored rows only, so a replay of the
  same tick (stale re-enqueue) still sees itself as first (rows at `tickIndex` are not `< tickIndex`), and a tick after it never does. One query per battle tick, not per round.
- Velchanos (2a): `battleParticipant.blessedStrength` is loaded per land participant in the loop right after the participants query (`religion.LoadBlessed(... EffectBlessedStrength, defenseTick)`) and
  multiplies `diceCount` in `rollSide` (~:1162). 2a added NO field to the battle-round payload. Horns go next to it as `blessedHorns` (loaded the same way) plus a per-round flag `hornsNow`
  (set in `resolveTick` only for round 1 of the first tick), so `rollSide`'s signature (5 test call sites) is unchanged. The two factors multiply (1,25 x 1,5).
- A unit that joins later reads `blessedHorns` too, but only the first tick's round 1 sets `hornsNow`, so it gets nothing (the storm's horns).
- Outcome record: `BattleSideRoundResult` gets `blessed_horns` (omitempty bool, round 1 only) — additive, old rows read the same.

## 2. Interception scan (the Thread)
- `transport/intercept.go ScanAt` loads all in-transit interceptable transports once, then per transport runs ONE sentry query with `departureInterceptRadius(t)` (:221).
  `departureInterceptRadius` returns 1 for a journey with the old `j.Thread` flag (set at departure by the retired `next_land_departure` prayer), else `interceptRadius` (2).
- To read the blessing AT SCAN TIME without an N+1: one query before the loop, `religion.ThreadCovered(ctx, db, world, tick)` -> set of land transport ids covered by a live
  `blessed_thread` row (realm | city with `settlement_id = t.origin_id` | unit with `mover_id = t.id`, `from_tick <= tick < until_tick`, `owner_id = t.owner_id`), a JOIN against `transports`.
  The loop takes radius 1 if `j.Thread` OR the id is in the set. The old flag and its consumers are untouched.
- Tick source: `current_world_tick()` (same as `seize`). Other seizure paths: none (`'intercepted'` is set only in `seize`). Messengers are never in `transports` interception (excluded by target check).

## 3. Cast / surfaces
- `IsBlessing` effects flow `Rite -> checkBlessingTarget -> applyBlessing` (settlement.go). Horns reuse it unchanged (land units; `BlessingUnitKind`="land"; no permanent step).
  Thread needs its own target check (land transports only, stored as `mover_id`/`mover_kind='transport'`, like Swift Feet) and its own targets loader (`loadRoadTargets` lists Runners too).
- Catalogue targets: `cult_prayers.go loadPrayerTargets` (units) / `loadRoadTargets` (roads). Thread gets `loadCaravanTargets`.
- Owner-visible: unit `blessings[]` strings come from `unit_blessings.go` (permanent only today) -> keryx `army/unit` and web `force_card.js` render them already. Horns add a temporary line.
  Caravans: the map/trade marker list (`world.go`, `/trades`) has no blessing field; add `blessings: ["the Thread, N game days"]` for the caller's own markers (one query).
- Tests counting catalogue prayers: 12 -> 14 (religion, handlers, codex).
