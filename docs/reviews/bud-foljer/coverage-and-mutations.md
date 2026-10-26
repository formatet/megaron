# Coverage and mutation review

The new behavior is opt-in (`following_unit`) on freshly dispatched land march, recall, redirect, stance and stance_pursuit envelopes. Existing envelopes keep their old delivery branch. No migration or retroactive order repair.

| Proof | Covered | Boundary |
|---|---|---|
| `TestOrderPursuit_MovedUnitNeedsPhysicalLeg` | All four order families; two changes of location; same messenger/order, reached endpoint as next origin; generation replay; no remote effect; actual final meeting | Stationary units between legs |
| `TestOrderPursuit_UnflaggedMovingEnvelopeKeepsOldMeaning` | Old unflagged envelope retains its previous moving-meeting outcome; runtime meeting opt-in is never decoded from JSON | Does not reinterpret old saved jobs |
| `TestOrderPursuit_MovingMeeting` | Four families meet a still-marching v2 unit; course changes and stance apply | Does not force a concurrent course change during the domain transaction |
| `TestOrderPursuit_MovingRouteLeg` | Saved v1 and v2 routes; moving interception, subsequent physical legs, delivery; advances world anchor for v1 | Does not cover every terrain/path combination or route-less interpolation separately |
| `TestInterceptCourierRoute_FrozenPhysicalClock` | Frozen v2 cadence/phase overrides a drifting world anchor | Legacy route timing remains the anchor-based path |
| `TestOrderPursuit_GuardsAndWaiting` | Early deadline, old generation, passage awaiting, pending carrier witness, world mismatch cannot schedule or execute; disband/owner change fail visibly; embarked/unlocated/no-port wait at reached hex and resume | Real boarding, rescue and disembark scheduling retain existing passage tests; no new sea-order behavior |
| `TestOrderPursuitAPI_*` | Five actual HTTP dispatch arms; receipt/DB/scheduled flag identity and generation; unit pending metadata, waiting/passage; visible foreign map projection excludes private metadata | Worker progression is proved separately |
| `client-report.md` tests | Actual Keryx commands/JSON and web renderers, qualified meeting time, changed route, wait without ETA, old unflagged text | Formatter tests do not prove layout |
| `acceptance-legacy-scoped/proof.json` | Fresh isolated PG217/Redis/server; real CLI march, later real CLI stance; same Runner misses old endpoint and replans; stance unchanged before meeting then applied by worker; real API, CLI roster and native Firefox1280/390 | Explicit initial world/terrain/workforce/unit fixture; no post-dispatch DB writes. No live-world test orders |

## Review findings

The extended tests exposed two real implementation errors, recorded assertion-red in `extended-baseline.log`: moving meetings were rejected by the stationary-only final ExpectedPosition check, and v2 interception used `anchor.WallAt` instead of the route's frozen clock. A subsequent frozen-envelope regression exposed that the initial moving-guard fix also relaxed an unflagged older envelope. A runtime-only `FollowingMeeting` flag, excluded from JSON and set only after a new following envelope physically meets the unit, now gates that relaxation. `TestOrderPursuit_UnflaggedMovingEnvelopeKeepsOldMeaning` preserves the old failure and a dedicated mutant kills the regression. The final course transaction retains its unchanged-route and status locks for moving delivery; ExpectedPosition is checked against the resolved physical origin. The new pursuit decision locks the messenger and shares the unit row while determining claim/new-leg/wait. The existing claim-before-domain-execution boundary is retained; this slice does not convert all order execution into a single transaction.

The first extended test run also had fixture defects (NULL owner violates schema; inaccessible sea fixture needed the map's `coastal_sea` representation). They are not counted as production regressions. Legacy moving-route proofs must advance the world anchor, since that version deliberately derives position from it. Initial full-suite failures in seven older API tests exposed their immediate/manual delivery before the actual deadline; their fixtures now progress the real clock and, when needed, follow later generations rather than weakening assertions or bypassing the new physical check. `full-go-initial-red.log` preserves the initial run.

## Physical mutations

Eleven server mutants and eight client/API mutants were run on separate clean checkouts. Every primary mutant failed a named assertion; original production bytes/SHA were restored and the relevant suite passed again on fresh databases. `server-mutations/verified.json` and `client-mutations/verified.json` give exact original/replacement text, source commit, expected test and SHA. Reproduce with:

```sh
python3 tools/order_pursuit_server_mutants.py CLEAN_CHECKOUT OUTPUT
python3 tools/order_pursuit_client_mutants.py CLEAN_CHECKOUT OUTPUT
```

[Exact byte-verified sed recipes](exact-sed.md) are generated from the executed manifests. Each manifest row specifies: replace its `old` literal with its `new` literal once in `file`, then run the named test. Core-file recommendations:

| Core file | Mutant | Expected assertion |
|---|---|---|
| messenger/order_pursuit.go | `if pos == at {` → `if true {` | TestOrderPursuit_MovedUnitNeedsPhysicalLeg |
| messenger/order_delivery.go | following stance branch → ordinary stationary branch | TestOrderPursuit_MovingMeeting/stance |
| messenger/recall.go | disable frozen-clock branch | TestInterceptCourierRoute_FrozenPhysicalClock |
| combat/march_start.go | restore the unconditional stationary ExpectedPosition guard | TestOrderPursuit_MovingMeeting/march |
| api/handlers/unit.go | dispatch FollowUnit=false | TestOrderPursuitAPI_DispatchOptInPersists |
| api/handlers/world.go | suppress owner metadata | TestOrderPursuitAPI_MapMetadataOnlyForOwner/owner |

Client core-file literals and named assertions are also listed in `client-report.md`; the execution manifests supersede its original unexecuted proposals. One exploratory mutant survived: removing only the inline `m.Own` map-metadata guard. The endpoint's later `foreignCivilianProjection` discards all private fields independently. That redundancy is documented, not counted as a killed mutant or concealed as a test gap. The initial early-deadline server mutant also survived a weak assertion that checked only `outbound`; the test was strengthened to require zero new jobs/legs/wait state, then the mutant became assertion-red.
