# Area expedition

Player problem: exploration only travels to one point; the decided area expedition has no complete client flow. This slice completes Temenos, Keryx, Megaron and the Codex on `codex/expedition`, over live march-preview master `97328f75`. Supports the geography part of the main gate.

Contract: choose an area and duration; server-owned bounds feed both web controls. An expedition chooses unseen reachable ground deterministically, records sight along its path, reserves time for its actual route home, turns by half the duration, and reports when it returns. A new order/recall ends the expedition. Original home loss redirects to a reachable owned home or stops in the field. Outcome, notification and state commit together; repeated arrivals cannot duplicate a report. Legacy point-explore jobs keep their event semantics.

Scope: the already approved expedition, integration with the arrival forecast, its lifecycle, all four player surfaces and reproducible proof tools. No new balance, ownership, visibility or command rules. A preview never exposes the travel time to a hidden first expedition leg. The chosen duration is sent to both immediate orders and Runner-delivered orders; command remains physical.

## Baseline

Imported Claude commits `78b9c3c4`, `cbc6367f`, `610ea064`, `676e7539` cleanly onto march-preview master. Before additional implementation, combat, handlers and CLI packages passed on a newly created PostgreSQL16 database, migration159 (`/tmp/megaron-expedition-baseline.log`). Claude's original worktree was preserved.

## Proof

Implementation: `15396e642ec7927d2d1fdc155fd53b5df00a4719`. Full Go suite on fresh PostgreSQL16/migration159 and full `go vet ./...` passed; all 376 JS tests passed.

- [Real CLI expedition](live-cli-proof.json): register/join/found → compiled CLI preview (unavailable, no dispatch) → CLI order → mission in web/CLI → turns at half-time → home after 19 ticks, 55 hexes seen, tin/silver/cedar finds. Exactly one turn and report, no failed arrival jobs, no retained expedition/sight rows, clean migration state.
- [Real web expedition](live-web-proof.json): same player lifecycle, real menu duration input and March button → area explored → home after 14 ticks, 59 hexes seen, copper/silver finds. Exactly one turn/report; clean state and no browser errors. The two successful worlds are independent, with actual server process build `15396e64…` and migration159.
- Fresh DB regressions: path sight, normal half-time and actual mixed-terrain/shorthanded naval return reservation, captured/missing/unreachable home, new-course cancellation, legacy point-explore compatibility, atomic failed turn/report and idempotent retry.
- Four physical server mutations each fail the named invariant; restored tests pass. [half-time](server-mutation-half-time.log), [path sight](server-mutation-path-sight.log), [atomic report](server-mutation-atomic-report.log), [return budget](server-mutation-return-reservation.log). Allowing a forecast to a hidden first leg also fails the real HTTP test: [FOW mutation](preview-fow-mutation.log).
- [Client tests, two fixture browser runs and CLI mutation](client-proof.md). New controls receive server bounds; Runner and redirect semantics remain physical. Desktop/390px containment passes.

The first exploratory proof attempts exposed two assumptions in the new proof script (axial map coordinates and the CLI word “Expedition”); those were corrected without production changes. A small-map land expedition was legitimately refused because unseen ground in that chosen area was unreachable. Final web gameplay used the normal 56×40 map, CLI the earlier 30×20 map. The reproducible script now uses normal map size and asserts POST receipts directly. Screenshot cleanup closes the order menu before the Army capture.

A second clean web world repeated the complete flow after screenshot cleanup: home after 8 ticks, 46 hexes seen, one report and clean ground state; [repeat proof](live-web-repeat-proof.json), [clean mission mobile](web-repeat-mission-mobile.png).

Timothy approved BILD and shipping on 2026-10-07. He explicitly asked to record that **"game days"** is used in player text and that he is uncertain about the term; its wording is not settled. It remains unchanged for this release, with a language follow-up in `temenos_tid_kalender_plan.md`, todo and TEXT/playtest. Merge and deployment verification follow below.

Reproduce real gameplay: build `server/cmd/server` with `-ldflags '-X main.buildCommit=<commit>'` and `server/cmd/keryx` into a private OUT directory, then run `python3 tools/expedition_live.py OUT <commit> web` and separately `... cli`. Each uses its own fresh PostgreSQL16/Redis7/server and registers, joins and founds via player APIs, sends the order through the real web or CLI, observes legs/turn/return, verifies both client report text and reads ground state without SQL fixture writes.

Client fixture proof: `python3 tools/expedition_acceptance.py OUT`; real modules, explicitly supplied API fixtures, desktop and 390px screenshots. It is a separate proof of control semantics, not a claim of real gameplay.

## Limits

The home-time reservation assumes the original home remains owned and reachable and the event worker processes scheduled arrivals. Home loss can require a longer journey; after turning, the mission shows the scheduled actual arrival at the replacement home. Combat, attrition and server downtime can interrupt an expedition. Human comprehension of the report remains TEXT for playtest; Timothy approved the new controls before merge.

## Integration and deployment

LIVE 2026-10-07 on Timothy’s approval and explicit mandate for Codex to integrate and ship. Master fast-forwarded to `6b7f200f6109680e962766457dc374f10884b43c`; origin pushed and `/opt/poleia` fast-forwarded. Air rebuilt automatically; the new process logged migration completion and `world ready` for the existing world. Both origin and public readiness report runtime `6b7f200`, migration159. Direct schema read confirms `dirty=false`; original world remains active at tick150 with zero cities and one player. No production player actions, expedition rows, sight rows or failed arrivals.

[Deployment proof](deploy-proof.json): all ten changed production assets match local bytes at origin and public URLs, unauthenticated preview returns401, and the new process has zero error entries. The isolated-player-tested Keryx binary was installed at `~/go/bin/poleia`, with matching SHA256 and the duration flag verified. Fresh premerge combat/API/CLI tests passed on migration159; full Go/vet, JS376 and mutation proofs are above. Subsequent proof-only commits do not change server bytes or require a restart.
