# Hex för hex — inventory before code, 2026-10-25

Baseline `42a8d4ff` on `codex/skepp-omedelbart`, production baseline `7e478f88`.
This slice proves the movement surface in the geography → shortage → bronze → elite chain.
No production edits. No new prod access in this inventory. Varvet untouched.

| Concern | Current owner and timing |
| --- | --- |
| Dispatch and ETA | `combat/march_start.go:839` rounds total to whole ticks, minimum1; line866 adds that duration to Clock.Now. Saved BuildRoute at882 starts at integer currentTick, before a late dispatch. |
| Physical occupancy | `movement.PositionAt`, `combat/route.go:75` RouteEnterMilli implement old valA: first entered hex at dispatch, target occupied before final completion. New order explicitly replaces that position/arrival rule. |
| Live sight | `province/eyes.go:27` LoadLiveEyes → UnitPosition.Physical reads saved route at tick.Anchor.MilliAt(now); world.go Map derives visible tiles and remembers them during the request (line461ff). Thus sight during travel depends on reads for this persistence path. Existing vision radius still applies; future route occupancy and visibility of a nearby tile are different concepts. |
| Completion sight | `combat/unit_arrival.go:274–294` re-finds the path and inserts every route tile into player_scouted_tiles in the arrival transaction. It does not persist entered steps as their own timed progression. |
| Storms | `combat/sea_storm.go:126` recurring scan advances weather and loads voyages; sail at228 evaluates entered hexes and storm occupancy since previous progress tick, in a batch transaction. This is not a scheduled event at each sub-tick crossing. |
| March encounters | `combat/march_encounter.go:133` groups current interpolated hex snapshots. loadCandidates181 excludes arrivals due this tick (`arrive_tick > currentTick`). This is not the full interval-based, time-ordered contact chain described in R3–R8. |
| Stationary interception | `combat/unit_intercept_scan.go:106` samples current positions; line157 calls old province.InterpolatePosition rather than reading the saved route for the moving target. Live-eye gate uses the newer anchor path. |
| Messenger passage | `messenger/passage.go:785` departure scan selects depart_tick=currentTick; BoardDispatchedShipRunner888 also boards directly. Carried position must follow carrier; RouteEnterMilli supplies entry boundaries to route-contact consumers. Boarding, passage, and arrival have separate handlers rather than one global sub-tick event sequence. |
| Consumers | province physical-position owner and eyes; API unit/path/timestamp projections; web map.js unitPixelNow → routes.js routeProgress/routePosition; Keryx reads API arrival timestamps. |
| Queue | events/scheduler.go claim query446–452 gates on integer due_tick and orders due_tick,priority,id. It is not a shared per-hex millitick ordering mechanism. |

Not everything currently waits until arrival: live sight can change during reads,
storms process entered route hexes in batches, and interception scans during travel.
Conversely those separate mechanisms do not establish one authoritative ordered
per-hex progression with prefix-only observations and consistent initial position.

## Proposed single time owner

The saved timed route in `movement` owns dispatch offset, ordered hex boundaries
and final arrival. `tick.Anchor` only converts Clock.Now to/from that coordinate.
Scheduler, encounters, storms, sight persistence, passage, API/CLI ETA and browser
all consume those saved boundaries. Existing event interpretation remains frozen;
a changed journey/event contract needs versioning and a transition for old journeys.
No implementation or transition chosen before the decision below.

## Internal contract conflict — STOP before fix

Order §1 says arrival at each hex = actual dispatch + cumulative step costs.
§3 says completion coincides with reaching the final hex and starts at origin.
§4 forbids changing the currently rounded total duration without a canon decision.

For the reported reconstructed route, deep-sea costs are0.2+0.2+0.2=0.6 ticks;
current total is max(1,round(0.6))=1 tick. Literal §1 produces0.6 while §4 requires1.
This contradiction is in the new order itself, independently of the superseded valA.

- **A, recommended:** keep current rounded total1, reach hexes at1/3,2/3,1 tick
  after actual dispatch. Preserve relative terrain costs through normalization.
- **B:** literal raw cumulative costs0.2,0.4,0.6; explicitly authorize the changed
  total and resulting arrival/interaction timing as an exception to §4.

Full inventory and both alternatives were sent to Claude in main .agents/chat.log
before production code. Claude coordinates Timothy's decision. Existing two red
reproductions remain red, no claim of a fix or broad suite. No mutations or Firefox
images yet because implementation is stopped at the explicitly required canon gate.
