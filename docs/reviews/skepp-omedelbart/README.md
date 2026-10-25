# Skepp omedelbart — investigation, STOP before fix

## SAMMANFATTNING

Report 43640c42: a galley seemed to reach (58,26) immediately. Baseline master
7e478f88, health 7e478f8 / migration 212. No production source changes, no deploy.
Classification: **(c), short valid travel plus a reproducible presentation/time-origin problem**.
One tick is not intrinsically a movement bug: entered deep sea costs 0.2 ticks,
three steps cost 0.6 and round once to 1. This investigation **proves** the chain's
movement surface; it does not change movement balance or canon.

## SLICE-KONTRAKT

Read-only production investigation; reproduce before fix; hand evidence to Claude.
No production writes, merge, push, deployment or changes to Varvet.

## BASLINJE

- Report: 2026-10-24 20:10:31 local, tick 103, city Tylissos (61,27).
- Ship bee01813-f017-4838-9e70-7424fbafa002, galley, full crew 20.
- UnitFormed tick 100 at 20:07:16; this was not a ship spawned at the target.
- UnitMarchOrdered tick 103 at 20:10:14.118, origin (60,27), target (58,26).
  Advertised departure 20:10:14 and arrival 20:11:14.
- Scheduled UnitArrival 1111541 due tick 104, processed 20:10:45.408, one attempt,
  no failure. The following leg's timer was created then, due 108 and processed
  20:14:45.405. Actual first-leg processing was about 31.290 seconds after order,
  14.208 seconds AFTER the player filed the report. No immediate server completion.
- Owner-filtered events around 100–104 show the ship order and land orders, not
  a transfer/load or shipyard placement at the target. No ship UnitArrived notice
  appears in the queried notification window; an exploring ship can continue
  without an ordinary positioned/garrison arrival notice.
- Saved historical route/intent has been overwritten by later exploration.
  Current snapshot is an explore_return voyage, **not the historical first leg**.
  We cannot recover the exact historical path or original intent from these rows.
  Orders query was empty, not evidence that no transient order ever existed.

Raw evidence: live-discovery.jsonl, live-evidence.jsonl, live-followup.jsonl.
Existing 07:04 evidence was recovered from the prior session, not recollected here.
Followup was executed through psql -XqAt -v ON_ERROR_STOP=1 inside BEGIN READ ONLY.
health.json records the inspected build. Acceptance provenance N/A (no server rig).

## EXPERIMENT

A shortest legal reconstructed route is (60,27) → (59,27) → (59,26) → (58,26).
All entered tiles are deep_sea; 3 × 0.2 = 0.6, galley full-crew factor 1,
max(1, round(0.6)) = 1 tick. This route is explicitly reconstruction, not a saved route.

The client's map.js unitPixelNow feeds routeProgress an interpolated movement tick.
routeProgress subtracts integer departure_tick, not the actual order instant.
Using the world's 60-second anchor (created_at 16:26:30.229916Z, also consistent
with current tick817/last_tick_at06:03:30.229916Z), nominal tick103 starts
18:09:30.229916Z. At order creation18:10:14.118Z, fractional tick is103.731468.
A newly issued 103→104 voyage is therefore already73% complete in the renderer.
The entered-hex ownership rule puts a three-equal-step voyage on its final hex
at two-thirds progress. Thus it is drawn at destination with **zero elapsed time**.
Wall-time fallback correctly reports0 progress at that same instant.

Replay directly imports unchanged production routeProgress/routePosition:

```
node docs/reviews/skepp-omedelbart/replay.mjs
node docs/reviews/skepp-omedelbart/replay.mjs --assert-departure
```

First command GREEN: verifies observed final-hex rendering and wall fallback0.
Second command deliberately RED: expects first entered hex at departure (R1),
gets final hex. Logs committed. No mutation: no production fix exists to mutate.
No claim of a browser reproduction or recovery of the historical route.

A second time discrepancy is independently saved: advertised ETA20:11:14 differs
from processing20:10:45. Integer due_tick is relative to the world's tick boundary,
while advertised timestamps add a full duration to the actual order time.
Do not repair only the sprite before deciding which clock owns dispatch/ETA.

## GRINDAR

| Gate | Evidence |
| --- | --- |
| Code | Direct unchanged production JS replay GREEN; explicit regression assertion RED |
| Visual | No screenshot/browser run; numeric renderer position only |
| Semantic | Distinguishes physical target occupancy, handler completion and displayed ETA |
| User | Historical report + owner-filtered production read-only rows |
| Drift | No writes/deploy, original health recorded |

## METODISK LÄRDOM

A whole-tick travel duration does not imply a full tick of wall time after dispatch;
compare all three: saved due_tick, display timestamps, interpolated current tick.

## KÄNDA AVGRÄNSNINGAR

Historical route overwritten; exact original sprite state is not proven. This is a
reproduction of the mechanism using observed times and a legal shortest route.
Historical process tick configuration was inferred from anchored60-second world
time and adjacent arrival processing, not an archived environment dump.
No balance change is proposed. Claude decides the followup clock contract and fix.
