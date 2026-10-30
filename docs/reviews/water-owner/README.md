# Water classification owner — insjöar slice 1 (Claude 2026-10-30)

Order: `.agents/order-insjoar-20261030.md` slice 1. Inventory: vault `megaron_inventering_insjoar_20261030.md`.

## Contract

One owner per layer for "which terrains are water", so slice 2 adds `lake` in one place per layer
instead of ~30. **No behaviour change.**

Three classes (all existing, only named):

| Class | Terrains today | Meaning |
|---|---|---|
| sea | coastal_sea, deep_sea | open water: horizon, storms |
| navigable | sea + river + river_ford | a ship floats here; never worked as land (fish only) |
| water barrier | sea + river | a land unit cannot step in (the ford is the gap) |

Owners:
- Go: `server/internal/province/water.go` (`IsNavigable`, `IsWaterBarrier`; `IsSea` stays in `hex.go`).
- SQL: migration 231, `terrain_is_sea(text)` and `terrain_is_navigable(text)` (inlinable `LANGUAGE sql IMMUTABLE`,
  same NULL semantics as the `IN` lists they replace).
- JS: `web/static/js/megaron/terrain.js`.

Out of scope: `internal/world` (mapgen) is a zero-dependency leaf and keeps its own typed `isSea`; slice 2 touches
it anyway. Per-terrain tables (move costs, fishing rules, palettes, labels) are tables, not classifications.

Proof: full suite green before and after (fresh DB each), a guard test that fails on a new hardcoded list,
a test that the SQL functions agree with Go for every terrain, mutants (drop a terrain from each owner → named red tests).

## Result (LIVE `445cb3af`, mig 231, 2026-10-30)

- Suite: baseline `01d2dc5f` green; slice green (Go all packages + 880 web tests), fresh DB each.
- Mutants (fresh detached worktree each): Go `IsNavigable` without ford → `pathfind_ford_test.go:77`, `river_current_test.go:70`,
  `water_test.go:43`; Go `IsWaterBarrier` without river → `chariot_test.go:40`, `courier_pathfind_test.go:41`,
  `pathfind_test.go:161`, `place_reach_test.go:563` …; SQL navigable without river → `water_test.go:62` (only the
  agreement test — production SQL coverage of river catchment is thin, the agreement test carries it); JS barrier
  without river → `terrain.test.mjs` + "river march: real map destination…".
- Live ground state: `terrain_is_sea`/`terrain_is_navigable` over `map_tiles` give sea=coastal/deep, navigable=+river/ford; no errors in journal.
