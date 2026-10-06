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
