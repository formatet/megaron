# Terrain `lake` — insjöar slice 2 (Claude 2026-10-30)

Order `.agents/order-insjoar-20261030.md` slice 2. Canon: vault `temenos_terrain.md` §Insjöar (Timothy 2026-10-30).
Slice 1 (`445cb3af`) made one owner per layer; this slice adds `lake` there.

## Contract

**What a lake is:** sea terrain (`coastal_sea`/`deep_sea`) not connected through sea terrain to the map edge.
Rivers do not connect — 8 of 9 live lakes reach the Thalassa only by river, and they are still lakes.

**Live measurement 2026-10-30 (read-only):** one world, 27 lake hexes (all `coastal_sea` today), 0 storm-track rows on
them, 1 unit (a disbanded galley). Lake neighbours already have `coastal = true` (coastal = beside any water since mig 101),
so coastal does not change.

| Rule | Lake | Where |
|---|---|---|
| Open water (horizon like the sea, land units/couriers cannot enter, not workable land) | yes | `province.IsSea` + lake; SQL `terrain_is_sea`/`terrain_is_navigable`; JS `isSea` |
| Ships sail it | yes, at the coastal-water cost | `TerrainMoveTicks` |
| Storms | never born there, never drift in | new `province.IsThalassa` (coastal_sea, deep_sea) for every storm check |
| Fishing | 2 places, 1 fish per fisher; no bay doubling, no harbour bonus | `hexRules`, production_rules copied from coastal water |
| Harbour | allowed (water neighbour, as today) | unchanged |
| Shipyard | refused when no navigable path (sea, lake, river, ford) leads to the Thalassa: "this lake has no waterway to the Thalassa" | Build handler |
| Ship exploration / patrol | lake counts as shallow coastal water (as today) | expedition, patrol |
| Rendering | same look as coastal water; label "Lake" | markraster/map.js; BILD after merge |
| Mapgen | new worlds label enclosed sea as `lake` as the last step | `world.MarkLakes` |
| Migration 232 | relabel existing worlds by flood fill; functions + production_rules | SQL |

Four surfaces: temenos (above), keryx (map glyph via `IsSea`, river-flow label, refusal text), megaron (render, label,
refusal text), Codex (`lake.md`, terrain list, fishing, storms, shipyard).

Out of scope: lake names and epitaph (slice 3).
