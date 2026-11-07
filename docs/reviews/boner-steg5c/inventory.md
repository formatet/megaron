# Inventory — boner steg 5c (murarna faller) — step 0, no code changed

## Stored wall level (two places, as the order expects)
1. `settlements.wall_level` (0..3) — the authority read by: battle start snapshot (`combat/battle.go:284`), field arrival / siege
   entry (`combat/unit_arrival.go:491`, `:2325`), population cap (`economy/population_growth.go:54`, `kharis/tick.go` growth),
   build guard (`api/handlers/province.go:1450`, `build_quote.go:70`), settlement view, `player_seen_settlements.wall_level`.
2. `buildings` row `building_type='wall'`, `level` — upgraded +1 by the generic insert in `combat/build.go:~108`
   (ON CONFLICT ... level+1); `settlements.wall_level` is raised in the same handler (`build.go:209`, LEAST(+1,3)).
   Mig 054 made them agree (`level = LEAST(GREATEST(wall_level,1),3)`, row only if wall_level >= 1).
   A fall must lower both in one TX; at level 0 the building row should be deleted (mig 054 has no row at 0). Not yet checked: any
   CHECK on buildings.level >= 1.

## Memory of a foreign city
`player_seen_settlements` (mig 227), owner `province.RecordSeenSettlements`; rows carry `wall_level`, `owner_id`, `name`, `q,r`.
Read by `api/handlers/seen_settlements.go`, `foreign_city.go`, `place_story.go`. A seen row is struck when the hex is seen empty.
The remembered wall is refreshed only when the hex enters sight again — so criterion "caster sees no new level" holds by default
as long as the fall does not touch `player_seen_settlements`.

## Scheduling an event with stored outcome
`events.Scheduler.EnqueueTickTx(ctx, tx, worldID, type, payload, dueTick)`; `events.Worker.Register` handlers must be idempotent.

## Omen-style notices
`kharis/omen.go` generates scheduled omens from world facts; the cast-time notice here is a new notification kind (Codex article needed).

## STOP condition check
- Wall level in more than two places: NO (exactly two stored, plus read-only snapshots).
- "A siege reads a cached level": YES, literally. `combat/battle.go:275-290` snapshots `wall_level` into `battles.wall_level` at
  battle start ("must not drift mid-battle") and the battle reads `battles.wall_level` (`battle.go:470`). A wall that falls DURING a
  running battle will not affect that battle; battles started after the fall use the new level. Live reads at arrival (`unit_arrival.go`) are not cached.
