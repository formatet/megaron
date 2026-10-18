# Slice14 — frånvaron syns

## Kontrakt

Problem: actual garrison grain losses are audit-only; kharis band changes are silent; archive rows cannot be aligned to world ticks.
Player truth: a notice marks army hunger starting/ending and each new kharis band, once per transition, with the day it occurred.
Invariant: persistent per-owner state, audit outcome and personal archive row commit atomically; retries/restarts never duplicate notices; no new punishment/grace rule.
Scope: kharis transition detection/storage, notification tick capture/API/WS, CLI/web/archive/Codex and proofs.
Non-scope: gameplay/balance changes, grace notifications, old history guessing.
Acceptance: hungry army noticed despite positive civilian food; continued hunger silent; recovery once; kharis down/up once; every row has exact day or explicit unknown legacy day; urgent active hunger stays first.
Stop: canon or unreliable history inference goes to Claude. Historical wall times cannot be translated through an assumed fixed tick cadence.
Proof: master baseline/new namedRED, fresh PG transition/API/WS tests, physical mutations, fullGo/vet/JS, native CLI/Firefox.

Chain gate: blocks legibility of the geografi→brist→brons→elit path after absence. First player encounter: returning to Notifications/Keryx notifications and seeing actual army losses or divine mood changes with action.

Baseline master3edcfb43 / migration185; 144-day status tip already removed by19 (confirmed by inventory and existing CLI/web regression tests). Sitos release generation already removed by13b; historical rows remain available as grouped reports.

## Resultat

Production `93c5a34e` + reviewfix `764dd2be` on `codex/franvaron-syns`. No merge/push/deploy or live changes for slice validation.

| Gate | Evidence |
|---|---|
| Baseline | Separate fresh185: full old kharis+combat GREEN (`baseline-domain-combat.log`); new transition/day/elite-routine tests namedRED (`backend-baseline-red.log`, `day-baseline-red.log`, `slice14-combat-baseline.log`). |
| Persistence | Migration186 stores owner-scoped condition+private last-known context; snapshot, new audit and personal archive row commit in one TX. Failure→rollback→retry, same-tick/restart dedup, stale-tick no-op tested. Live callback sees already committed row. |
| Actual game chain | `TestStarvationSignal_ActualLossEvenLastGarrison`: real upkeep→kharis at size100/1, fish surplus for people but no grain for army, one start on right city/day; next paid upkeep or no remaining garrison→one end; retry silent. Fed other city remains untouched. `actual-upkeep-transition.log`. |
| Lifecycle | Continue/recover/restart hunger; fixed adversarial same-world city ids; owner transfer and collapse close old scopes using only prior private context. Clearing archive before transfer cannot break handler or expose new owner's renamed city. Full kharis package and named archive-delete RED→GREEN proof. |
| Kharis | Canonical boundaries60/30/5 in both directions, external between-tick changes, initial maintenance crossing and rollback/retry tested. A band-change notice names both old and new band. Rule/start40/cap40/no-grace/punishment unchanged. |
| Existing loss channel | Daily UnitAttrition audit retained; non-destroyed land garrison routine notices replaced by transition owner. All land types incl elite carry additive pre-loss location witness. Destruction, field/naval notices unchanged and tested. |
| Day | Migration187 captures exact world-specific tick for new archive rows; explicit transition tick retained. API/live WS/delayed WS day agree; day0 remains known. Only explicit original body.world_tick backfills history, unknown remainsNULL, payload/id/walltime unchanged. |
| Mutation | Original eight physical namedRED→byte-exactrestore→fresh187GREEN: transition suppression, wrong-city witness, band boundary; wrong-world day, delayedWS current-day substitution, invented legacy days; elite witness exclusion, daily routine restoration. Repro: `domain-mutations.py`, `day-mutations.py`, `combat-mutations.py`; logs beside scripts. |
| Four surfaces | Three new kinds mapped to upkeep/kharis Codex, actionable CLI/web text, all archive/chip/dispatch/watch rows show Day# or Dayunknown. Latest active city hunger floats above chronology; recovery unpins old start, restarting repins. Historical Sitos releases grouped with drill-down; archives remain readable. |
| Verification | Full `tools/gotest.sh` fresh187 GREEN(world145.890s) before reviewfix; after reviewfix whole fresh kharis10.328s+combat11.452s and vet GREEN, fixed-id failure fixture also passes twice;646JS/0fail (UI unchanged by reviewfix); four Day guards GREEN. `full-go.log`, `vet.log`, `js.log`, `day-guard.log`. |
| Native | `native/evidence.json`: process health commit93c5a34e/migration187/statusok, productiondirty=false; natural register→CLIlogin/join/found/watch/feed gives Day#0. Actual Firefox1280/390, zero pageerrors, reviewed1:1. New-kind rendering uses **separately labeled synthetic replay**, not a claim of natural hunger/band transitions; those use real PG handlers above. |

## Claude review17:51 — notification errors must not stop the world

`764dd2be` restores log+continue per Wanax and logs a failed garrison observation, so observation errors cannot prevent recurring KharisTick scheduling. Atomic per-Wanax transition state remains available for the next scheduled observation.

`TestTickNoticeFailureKeepsWorldRunning`: pinned A-low/B-high ids, B inserted first; A's notification CHECK fails, B still loses exactly0.1 kharis and gets its notice, next tick is queued. Remove failure: next tick maintains both again, A's original band crossing arrives at day11, B has no duplicate. Garrison-notice failure separately still queues next tick. NamedRED on previoushandoff, **two more physical return mutations** namedRED→byte-exactrestore→fresh187GREEN (`world-tick-*` logs/repro). **Total10 physical mutation cases.**

Garrison observation keeps its existing world transaction: on a notification fault that observation batch rolls back, is logged, and is retried on a later scheduled tick; it cannot stop gameplay or future world ticks. Splitting garrison observation into city transactions is not claimed.

Validation after reviewfix: `reviewfix-domain-combat.log`, `reviewfix-repeat.log`, `reviewfix-vet.log`. Final ordinary CLI/watch/Firefox process proof: `native-reviewfix/evidence.json`, commit764dd2be/migration187/dirtyfalse. Earlier full suite and646JS remain applicable except for the small error-isolation control flow covered by these fresh targeted checks. No gameplay rule changed.

## Kända avgränsningar / integration

- Historical rows without explicit recorded tick display **Day unknown**; no changing-cadence wall-clock estimate is invented. Existing wall timestamps still support stable history ordering.
- A garrison ending means no further army hunger losses at that city; it does not promise that people are fed or any soldiers remain. Ownership/inactive closures carry `reason=no_longer_owned_or_active` and only previously known private context.
- Conditions are observed on the kharis tick, including external kharis changes since the prior observation. A stable low band at migration gets no invented historical crossing.
- Archive view retains its existing100-row API window; active hunger priority is resolved from transitions in the loaded window. This slice does not add archive pagination or redesign existing civilian warnings.
- BILD: current archive/dispatch layout with new date/text, shown in `native/render/`. Claude/Timothy decides integration approval; TEXT clarity belongs in playtest. No claim that a synthetic render proves the server transition.
- Claude keeps vault/todo: update notification mechanics/date presentation and link this proof at integration, then remove the Codex verification row. No canon or punishment decision introduced here.

Method: final review caught archive deletion coupling (fixed context ownership with namedRED before fix). The first extra real-pipeline recovery test accidentally reseeded unique goods rows; fixture corrected to UPDATE (`actual-upkeep-fixture-first.log`), fresh complete pipeline thenGREEN. Old renderer's impossible Wrathful9 fixture corrected to Wrathful3 before final native proof. None counted as product regression.

CLI text snapshots normalize trailing whitespace only.
