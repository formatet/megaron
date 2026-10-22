# ORDER 9 — City destination shortcut

Contract before implementation: choosing a known city is an alternative to exact coordinates.
Reuse `GET /worlds/:id/cities`, which owns Known (live/remembered/contacted) versus rumour knowledge.
No new catalogue, FOW rule, sea-neighbour guess, event type or domain permission.
CLI: `--to` on post, march/alias and redirect; reject mixed destination forms,
unknown names and ambiguous names (show candidates). Coordinate forms remain supported.
Pickup resolves a unit, not a place; recall/repair/reinforce/load/unload/stance have no hex destination.
Web: city select in both War's Choose destination and map's march form, own cities first;
select fills the normal destination, leaving map clicks available. Confirmation names the city.
Tests: own/known allowed, unknown/rumour excluded, duplicate names ambiguous, coordinates preserved.
BILD: actual order panel/CSS/controller in Firefox at 1:1, 390 and 1000 widths, two clean runs.
Claude/Timothy judge BILD before merge; this branch never merges/pushes/deploys itself.
Proves geography portion of the chain gate.

Baseline: fresh198, unmodified master0b78b2b7, named city/resolver/preview tests and JS.
Inventory sent to Claude 2026-10-22 07:46 before production edits; approved with patrol excluded
because a city hex can never satisfy coastal-sea patrol. Rebased on ORDER8 LIVE e5daa694 before code.


## Result and evidence

- No production server change. Exact destinations consume the existing Cities directory.
- CLI: `cmd_city_target.go` filters exact Known rows, resolves names without guessing on ambiguity,
  and retains coordinate parsing. A best-effort directory lookup names coordinate receipts too;
  failure after acceptance does not report a sent order as failed. JSON stays the raw API response.
- Web: actual order form has a city select; own/known optgroups and owner/coordinates distinguish names.
  City choice uses normal `openMarchCtx`, so unit eligibility and mission rules remain in their owners.
  Closing/changing world or target invalidates outstanding city loads and stale receipt presentation.
- `TestCityTargetsCommandAndKnowledge`: 4 command factories ×10 cases, including post stance,
  coordinates, own, known, unknown, rumour, missing point, duplicate and mutually exclusive forms.
  Preview tests pin exact coordinates and no POST. Patrol deliberately has no `--to`.
- Shared real-PG `TestCitiesRumourKnownIsNotContactable`: own exact city, absent unseen foreign city,
  rumour without coordinates, then remembered exact city. No new knowledge rule.
- `mutation.py` physically lets rumour rows through in CLI and web independently: named RED, exact-byte
  restore GREEN (`mutation-{cli,web}-{red,restored}.log`).
- `browser_check.py`: actual map.html form and its inline styles, megaron.css, production controller and
  helpers in Firefox at1:1; scripted HTTP, not a live-world claim. Two resets at390/1000 each: War entry,
  own-first city list (no rumour), Pylos selected→POST(4,0), named receipt, subsequent ordinary map
  target→POST(7,0), long name readable, no horizontal overflow or page errors. Baseline uses master
  e5daa694's original form/controller. Screenshots are BILD for Timothy, not programmer approval.
- ORDER8's missed sibling article corrected separately in `4399b5be` before city-guide edits,
  explicitly approved for separate cherry-pick by Claude. No expedition mechanics change here.

## Reproduction and gates

`GOTEST_PORT=57900 tools/gotest.sh`; `python3 docs/reviews/stadsval/mutation.py`;
`python3 docs/reviews/stadsval/browser_check.py [--baseline]`;
`node --test $(rg --files web/static/js | rg '\.test\.mjs$')`; `go -C server vet ./...`.
Function/semantic: automated real-PG owner + CLI HTTP fixtures + actual Firefox controller.
User: automated scripted orders above; human comprehension not claimed. BILD blocks Claude's merge.
No new route, event, migration, catalogue, or movement/knowledge mechanics. No live account mutated.
