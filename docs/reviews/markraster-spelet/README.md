# Slice C: real-game acceptance

Measured application: **master `ad2dc277`**, `/healthz` **ad2dc27**, migration
**171/171**, Firefox **146.0.1**, DPR 1. New acceptance world
`3b5fb9cd-8c06-4685-88df-b162c9acff99`, 80×60, TICK_SECONDS=6.
[Start provenance](provenance.txt), [end provenance](provenance-final.txt),
[source/container/seed metadata](metadata.json), [raw results](browser.json).
This branch adds a report and observer tool only; no production/design changes.

Before using the rig, searching current `megaron_todo.md` for the exact phrase
`Röra ingenting i containern` returned no matches. The existing process was
84504e4 / migration 170 and was never used as evidence. `acceptance.sh up` upgraded
it; `acceptance.sh reset` created this fresh world. Four human accounts
Eirene, Nestor, Melia and Damon joined normally and founded Mycenae, Tiryns,
Kyme and Phaistos through the normal founding API. No SQL placement, fog reveal,
resource grant, unit teleport, fabricated HTTP or client map data was used.
The SQL reset is the acceptance tool's normal reset; other SQL was read-only.

## Result

**Passed the requested renderer checks in the real game**:

- Firefox 1280×900 and 390×844, authenticated `/play`, normal bootstrap and live
  RAF/polls. No console/page errors and no HTTP response errors in all four views.
- Mycenae (Eirene), inland, and Kyme (Melia), coastal: city silhouettes/flags and
  shore/ground rendering are visible at both widths. [Inland](Eirene-1280-city-before.png),
  [coastal](Melia-1280-city-before.png), [coastal mobile](Melia-390-city-before.png).
- Eirene issued an actual **28-day expedition from the web UI**. [Order](Eirene-1280-web-explore-order.png).
  The accepted march physically moved the Spearman. Twelve real refresh polls
  recorded **fog→live** and **live→remembered**, while thousands of tiles remained
  unknown. [Before](Eirene-1280-city-before.png), [after movement](Eirene-1280-city-after-movement.png).
  The integration observer used read-only map coordinates to choose a real nearby
  river six hexes away; the player still saw fog and used the regular explore order.
  This is targeted integration acceptance, not a blind geography playtest.
- Sangarios became known through the moving unit's sight. No fog tile carried a
  river_name on any checked response. [River desktop](Eirene-1280-river-after-movement.png),
  [mobile: all three tiers](Eirene-390-river-after-movement.png),
  [mobile panel](Eirene-390-river-panel.png). The name appears in the map and title;
  the panel's TERRAIN is **River Delta**, respecting the integrated casing fix.
  [Real Keryx map output](keryx-map.txt) also lists River/Delta of Sangarios with
  remembered knowledge. [After-movement map data](movement-tiles.json).
- Completed equal-content refreshes added **0 bakes**. During movement, each
  accepted bake had a **different signature** from that cached chunk; unchanged
  signatures were never re-baked. Four changing polls rebuilt 2, 6, 4 and 4 chunks
  respectively; eight polls with no tile changes rebuilt 0. Worker mode and
  pending=0 were verified before each settled screenshot.

The desktop view captures the expedition starting and the first new sight. The
mobile view was opened afterwards, while the unit was farther away: it includes
58 live / 49 remembered / 4693 unknown tiles. It is not labelled a pre-move view.
A first observer attempt omitted opening the existing **More** disclosure before
filling expedition duration; the harness was corrected. A subsequent mobile
refresh crossed a real movement tick, so the equal-content check was made
conditional on actual equality and changed signatures checked otherwise.
Neither issue required a product change.

## Timing

Stopwatch around the real synchronous render call; normal rendering arithmetic
unchanged. Worker callbacks log accepted chunk keys/signatures. A separate RAF
probe records frame intervals. These observers add small overhead and do not
measure GPU presentation. Active-render statistics exclude early no-op calls.

| Player / viewport | Initial max render (ms) | Active median / p95 / max (ms) | RAF p95 (ms) | Aggregate worker bake (ms / chunks) |
| --- | ---: | ---: | ---: | ---: |
| Eirene 1280 | 20 | 9 / 12 / 26 | 17.22 | 918 / 41 |
| Eirene 390 | 22 | 8 / 9 / 22 | 17.24 | 441 / 20 |
| Melia 1280 | 14 | 4 / 9 / 14 | 17.18 | 511 / 30 |
| Melia 390 | 14 | 2 / 3 / 14 | 17.18 | 282 / 16 |

All these views use the worker. Aggregate bake CPU time is not time until the
view completes. This world is 4800 tiles, mostly unknown to each player; it
cannot replace B's full-world/minzoom Firefox+Chromium comparison.

## Findings and rig limitation

No new markraster, FOW, river-name or city-icon defect was established in this
run. Images are supplied for Claude/Timothy's visual review; no redesign done.

[Server diagnostic](server-transport.log): at closure of the long-lived Firefox
context the server logged `response.WriteHeader on hijacked connection`, and the
WebSocket request as 504 after ~46 s. The page reported no errors. This is an
observed transport/logging issue for separate triage, not proof that the renderer
or an in-flight player action failed. No fix made.

The normal Docker build could not resolve Alpine's package CDN, also with
`--network=host`. To complete acceptance without changing the machine's DNS or
production source, the current server was built locally with `CGO_ENABLED=0`
and `-X main.buildCommit=ad2dc27`. A temporary Compose build override layered that
binary and current migrations on the rig's existing cached runtime. The same
`acceptance.sh up` then passed its **actual /healthz** commit/migration check.
Frontend files are the real bind mount from this worktree; the served renderer
bytes equal the worktree source (hash in browser.json). Local Go version and
container image ID are recorded. This verifies application integration, not a
clean network-dependent Docker image build.

The cached build inputs are under `/tmp/markraster-acc-build`; private account
configs under `/tmp/markraster-spelet-accounts` (not committed). The observer is
`tools/markraster_spelet.py`, using those real accounts and the locked world's
chosen expedition area. It resumes already completed report rows; remove the
report JSON only when deliberately starting a new run, and choose a new target
if using another world. `acceptance.sh provenance` reads the running process.
The rig remains running for review. No merge, push or deploy by Codex.
