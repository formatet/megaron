# ORDER 10 — The Host stands where its marker is

## Contract before code

Problem: marker/vision follow the saved march, but founding status, camera and settlement read departure q/r.
Player truth: Host marker, hover/panel, founding/status CLI, vision and actual settlement use the same physical hex.
Invariant: no change to persisted route/time, event interpretation, sight radius, founding geography or moving-Host permission.
Scope: one province position reader (saved movement owner + preserved legacy fallback), its eyes/API/founding consumers,
Host web projection/status text and Keryx/Codex. Spawn's raw-host occupancy is reported separately to Claude and excluded.
Acceptance: marching and positioned Host agree across surfaces; settlement appears at physical hex; old arrival after
settlement is harmless; marching/waiting/can found here statuses are words; dissolved Host has no marker/eye/active phase.
Stop: new canon or movement/event interpretation needed. Preserve ability to settle during a march, using the physical hex.
Proof: fresh198 baseline, real-PG API/settlement/eye agreement and scheduled retry, named mutation, JS, fullfresh once;
Firefox1:1 three marker states, BILD to Timothy via Claude before merge. Proves geography portion of the chain gate.

## Inventory sent before code (2026-10-22 08:05)

- `movement.LoadActiveRoute/PositionAt`, caller's `tick.Anchor.MilliAt`: canonical weighted route occupancy.
- `province/eyes.go`: already saved-route physical, legacy wall/path fallback.
- `render/map.js unitPixelNow/render/hover`: weighted route physical; agrees with eyes for valid saved routes.
- `homePosition` and `FoundingStatus`: founderPhase/raw unit q/r, departure during a march.
- `openHexPanel`: matches Host's raw q/r, opens Host panel/forecast at departure rather than drawn marker.
- `foundMetropolisFromNomadicHost`: raw q/r, permits settlement during march, hence wrong founding hex.
- Keryx founding/status: inherits raw founder API and lacks explicit physical state. `/units current_q/r` already physical.
- `hostCurrentPos`: already physical for Host messenger/order origin but duplicates the position algorithm.
- Extra reported, excluded: `pickSpawnTile` counts other Hosts at stored departure q/r.
- Existing arrival handler has a disbanded/null-coordinate idempotency guard; test after actual physical founding.

Baseline `baseline-go.log`: untouched mastera44cd3a1, fresh198 named Host/founding/live-eye cases green;
`baseline-js.log`: current JS suite before changes. These tests did not assert cross-surface physical founding.
