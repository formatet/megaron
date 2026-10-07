# March arrival preview

- Problem: players cannot see a march's estimated arrival before sending it.
- Player truth: web and keryx show authoritative tick timing before dispatch, or an explicit unavailable reason when an order needs a courier or explores unknown ground.
- Invariant: preview cannot mutate game state, reveal hidden route geometry, or imply an immediate departure for courier orders; march and preview use the same preparation.
- Scope: combat march preparation, GET unit march-preview, web march panel, keryx march preview, Codex march documentation and focused tests.
- Non-scope: redirect, courier interception prediction, new movement or world rules.
- Acceptance: matching preview/dispatch timing; unchanged rows/jobs/events after preview; ownership and fog gates; clients label conditional estimate; four surfaces agree.
- Stop: a new canon decision is required.
- Proof: targeted baseline and regression tests, intentional timing mutation, client tests and review of user wording. Supports (proves) the main gate by making movement decisions legible.

## Server proof

Fresh database, migration 158, `tools/gotest.sh`:
- Existing `TestStartMarch_|TestMarch_` combat/handler tests passed before new preview assertions.
- `TestMarchPreview_ReadOnlyAndTiming` passed: authenticated GET has no unit/goods/job/event/messenger mutation, same timing as POST, ownership/FOW checks, unknown exploration unavailable, field orders require Runner, no-store.
- Intentional preview arrival_tick +1 mutation failed the real preview-vs-POST comparison (7 versus 6); restored.
- Existing full/shorthanded ship crew speed test now also compares preview and actual departure timing.

API: GET `/worlds/{worldID}/units/{unitID}/march-preview`, query fields mirror march POST. Success `available:true,arrival_tick,duration_ticks,arrives_at_utc`; unavailable `available:false,reason:unknown_terrain|courier_required`. No route/hidden geometry returned. Known destination route semantics stay those of dispatch. Distance-zero nomadic commands use the normal origin resolver. A field command with positive Runner distance has no predicted arrival, since passage may wait for a real ship. This is a timing estimate, not a resource reservation; provisions and concurrent state still validate at dispatch.

## Completed client and integration proof

Implementation commits: `f135b768` (CLI), `31b9d90c` (server), `e3b0da77` (web/Codex).
Branch: `codex/march-preview`; review/merge/deploy belong to Claude per AGENTS.md.

- Full Go suite passed on a fresh PostgreSQL 16 database, migration 158 (`tools/gotest.sh`); full `go vet ./...` passed.
- All 371 JS tests passed. Tests cover per-unit differences, escaped labels/errors, exact order fields, courier/redirect/unknown forecasts, stale responses and cancellation during debounce.
- Chromium with actual map and Army modules: both entry points update estimates; changing quantities and destination/intent works; blank coordinates clear the forecast; no mutation requests; desktop and 390px containment pass.
- Physical web mutation removing the input/change forecast bindings makes the browser proof fail. Restoring those bindings passes the same scenario. CLI GET→POST mutation fails `TestMarchPreviewNeverDispatches`; restored CLI passes.
- Two separately created real worlds with PostgreSQL16/Redis7/server build `e3b0da776eeba945779b09a80b0e0835394bd197`, migration158: register → join → found city → actual API preview → compiled Keryx preview → web menu preview → click March. Forecast and actual arrival matched (game day 4 in run1, game day5 in run2), with no browser errors or SQL fixture edits. Both disposable worlds and servers removed.
- Actual process provenance and result: [first world](proof.json), [second world](proof-run2.json). The browser menu was opened programmatically for a discovered known destination, then its real quantity input and March button were used. Army-panel integration is additionally proven by the fixture browser scenario.
- 1:1 screenshots: [desktop](live-desktop.png), [390px mobile](live-mobile.png). Existing ETA row styling reused: FUNCTION + TEXT; wording comprehension remains for playtest.

Reproduce UI fixture proof: `python3 tools/march_preview_acceptance.py`.
Reproduce real proof: build `temenos` with `-ldflags '-X main.buildCommit=<commit>'` and `keryx` into a private output directory, then `python3 tools/march_preview_live.py OUT <commit>` (requires Docker, Go, Python Playwright/Chromium). It allocates its own containers/ports, uses a clean server environment and removes its containers, process and private CLI config afterwards.

Logs: `/tmp/megaron-march-preview-{full-go,vet,all-js,browser,web-mutation}.log`; live logs/proofs in `/tmp/megaron-march-preview-live/` and `run2/`.

Limits: snapshot estimate, no resource reservation; dispatch revalidates. Positive-distance Runner orders, redirects and unexplored destinations explicitly have no arrival forecast. This slice does not introduce expedition-duration rules. Claude's subsequent expedition branch requires integration review of the shared preparation and client fields.
