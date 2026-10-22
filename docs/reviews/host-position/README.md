# ORDER 10 — Host physical position

## SAMMANFATTNING

Marker and eyes followed the saved march while Host status/founding read departure q/r.
They now consume `province.UnitPosition.Physical` and a shared read-only founding-site preflight.
Built/tested on `codex/host-position`; Claude integrates, Timothy judges BILD. No merge/push/deploy here.
Actual native process `/healthz`: `a78e3c40`, schema198, status ok (`native/verification.json`).
That measurement precedes only the final private error wording change (“Host clock anchor”).

## SLICE-KONTRAKT

Player truth: marker, camera, hover/panel, sight, Keryx founding/status and settlement agree on physical hex.
Invariant: persisted route/time, events, vision radius, founding geography and moving-Host permission unchanged.
Scope: province reader; eyes/Host read/founding/order-origin consumers; web/Keryx/Codex.
Acceptance: moving/stationary agree; actual metropolis at physical hex; stale confirmation409;
dissolved Host has no active phase/marker; old queued arrival and retry harmless; three status words.
Stop: new canon or event/movement interpretation needed. Proves geography portion of chain gate.

## BASLINJE OCH INVENTERING

Sent to Claude before code at08:05. `baseline-go.log`: untouched mastera44cd3a1, fresh198 green;
`baseline-js.log`:707 green. Existing tests did not assert physical founding across surfaces.
- Saved route: `movement.LoadActiveRoute/PositionAt`, caller's `tick.Anchor.MilliAt`.
- Eyes and weighted renderer already physical; camera/founding API/raw-q Host-panel match were departure.
- Actual founding also read departure and already permitted founding while marching.
- `hostCurrentPos` was physical but duplicated the algorithm for messenger/order origin; now shared.
- Extra reported/excluded: spawn occupancy `pickSpawnTile` counts other Hosts at raw departure.

## IMPLEMENTATION

`UnitPosition.Physical` centralizes saved-route reading and preserves explicit legacy path fallback.
Founding locks phase+unit, resolves physical position, then uses the same `loadFoundingSite` as status.
Both `/founding/status` and the active Host in `/units` carry `host_state(q,r,at_milli,status,can_found_here,reason)`.
The web consumes that projection, never a Host's raw departure q/r. Foreign-unit projection is unchanged.
Statuses are words: Marching / Waiting (reason) / Can found here. Moving on valid ground remains foundable.
Web and CLI send optional expected_q/r from the confirmed position; a moved Host returns409 without founding.
Older API callers without expected coordinates still found where the Host physically stands.
No event type/payload or schema change. Existing null-coordinate/disbanded arrival guard remains the owner.

## EXPERIMENT OCH GRINDAR

| Check | Result / evidence |
|---|---|
| Real PG, authenticated API | `TestHostPhysicalPositionStatusEyesAndFounding`: weighted route departure(0,0), marker API/eyes(2,0), actual metropolis(2,0). Moving, stationary invalid/valid, stale409, old arrival+retry. `targeted-go.log` |
| Mutation | Restore departure position in server and web independently → named RED; exact-byte restore → GREEN. `mutation-{server,web}-{red,restored}.log`; `mutation.py` |
| Natural player flow | Built server + Keryx, natural register/join; no SQL writes or API overrides. Departure(6,48), scrub_maquis target(6,49), moving Host API/CLI(6,49), stale409, metropolis(6,49)201, phaseinactive. `native/verification.json`, `native.py` |
| Firefox BILD | Actual map DOM/styles/renderer/panel/hover, isolated HTTP fixture. Physical(2,0), raw departure(0,0). Marching/waiting/can_found_here at390/1000, two pixel-identical frozen runs each, no page errors, warmed terrain. `firefox.json` and `firefox-*.png`; `browser_check.py` |
| Full fresh Go once | All packages except language guard green, world154.342s. Guard rejected private “tick anchor” error wording; changed to “Host clock anchor”. Original exit1 retained in `full-go.log` |
| Final fresh affected packages | Schema198 on57900: handlers42.747s, province0.370s, tick/language guard0.166s, Keryx0.352s GREEN. `final-affected-go.log` supersedes that one guard failure |
| JS / vet |708/708 JS and vet GREEN. `js.log`, `vet.log` |
| Integration | BILD sent to Claude; Timothy approval before merge. Vault Host plan, owner map and BILD entry updated. No live world modified |

## REPRODUCTION OCH AVGRÄNSNING

`GOTEST_PORT=57900 tools/gotest.sh`; final affected packages with same tool; `python3 docs/reviews/host-position/mutation.py`;
`python3 docs/reviews/host-position/browser_check.py`; `python3 docs/reviews/host-position/native.py` (commit production first);
`node --test $(rg --files web/static/js | rg '\.test\.mjs$')`; `go -C server vet ./...`.
Snapshots use server projections; the Host marker updates with its API poll. Spawn placement is a separate reported issue.
Native proof is a real process; frozen BILD is explicitly a scripted HTTP scene. Neither claims human comprehension approval.
