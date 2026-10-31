# Platstyper P1 — proof (2026-10-31)

Six new nameable place kinds: `island`, `strait`, `cape`, `plain`, `hills`, `delta`. Mig 236 widens the
`place_names.kind` CHECK. Open depths are NOT included. Plan: `.agents/plan-platstyper-p1-20261031.md`.

## Definitions as built (`server/internal/world/places.go`)
- **island**: a land component (anything not coastal/deep sea/lake; rivers and fords are land) that is not the
  largest on the map and has no hex with a neighbour outside the map. Min 1.
- **strait**: coastal/deep sea hex with land on two opposite sides (neighbour pairs 1,0/-1,0, 0,1/0,-1, 1,-1/-1,1).
  Connected such hexes = one strait. Min 1.
- **cape**: land hex with >=4 coastal/deep-sea neighbours in a land component of >=3 hexes. Connected = one cape. Min 1.
- **plain / hills / delta**: components of `plains` / `hills` / `river_delta`. Mins (measured below): plain 7, hills 3, delta 1.
- Map tile shows the smallest place it belongs to (`smallestPlaceByHex`, `api/handlers/world.go`); ties keep
  `Places` order. A delta hex still carries its river's name in `river_name`.

## Measurements (places per map; `noOwnHex` = places with no own hex under smallest-wins)
Seeds are 64x64 `GenerateMap`; live = world e1ed7d0c (4096 tiles, read-only from CT 126).

| kind | 1009 | 2018 | 3027 | 4036 | live |
|---|---|---|---|---|---|
| island | 10 | 9 (1 hidden) | 6 | 4 | 6 |
| strait | 21 | 19 | 16 | 16 | 23 |
| cape | 41 (1 hidden) | 33 (1 hidden) | 35 (1 hidden) | 28 (1 hidden) | 27 (1 hidden) |
| plain (min 7) | 11 | 10 | 11 | 11 | 11 |
| hills (min 3) | 8 | 7 | 5 | 7 | 10 |
| delta (min 1) | 9 | 11 | 14 | 8 | 14 |

Raw plain sizes before choosing min (seed 1009): 26 one-hex, 6 two-hex, ... 55 components at min 1; at min 7 it is 11.
Raw hills (1009): 13 components, 3 one-hex... at min 3 it is 8. Delta sizes are 1-3, so min 1.

Hidden (no own hex) places: every hidden cape is a 3-hex island whose hexes tie with the island and the
earlier island wins; one hidden 6-hex island in seed 2018 (anchor 48,49) is covered by smaller places.
Single cases per map, not a pattern. Strait and cape counts are above 15 (alfa: more rather than fewer).

## Tests
- `server/internal/world/places_shapes_test.go`: `TestPlacesIslandStraitCape` (:64), `TestPlacesLargestLandmassIsNotAnIsland` (:87),
  `TestPlacesPlainHillsDeltaMinimums` (:102).
- `server/api/handlers/place_overlap_test.go`: `TestSmallestPlaceByHex` (:37), `TestPlaceDeltaCanBeNamed` (:13, mig 236 CHECK + API).
- `server/internal/combat/place_reach_overlap_test.go`: a hex of two places (island + hills) is a reach target for both.
- Changed: `places_test.go` (the fixture's two deltas are places now), `place_reach_test.go`
  `TestPlaceReachRiverFordDeltaShareOneRight` (river chain keeps one right; the delta is now a second place with its own right: 2 rows, 2 notices).

## Mutants (all killed by an assertion, none `[build failed]`)
| mutant | killed by |
|---|---|
| island: drop "not the largest" | `places_shapes_test.go:98` |
| strait: any two land sides instead of opposite | `places_shapes_test.go:73` |
| byHex: last place wins again | `place_overlap_test.go:43` |

## Suites
- Full `tools/gotest.sh`: all packages ok, web 878/878. Four-package invocation notes below.
  In ONE multi-package invocation `TestShipyardDeployTransitionPreservesDeadlineWithoutFreeStaff` and
  `TestWorldMe_OwnWorldIsolation` fail — identically on master 99ffed65 (verified), so pre-existing cross-package state.
- `tools/test-web.sh` 878/878, `tools/day_guard_test.py`, `tools/test_verb_parity.py` OK.
