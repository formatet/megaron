# Speltest fixes

## A2 contract (complete; integrated by Claude)
Problem: stored stone 23.999999999999996 refuses a 24-stone building.
Player truth: a rounding residue must not prevent paying a displayed exact cost.
Invariant: real shortages refuse atomically; accepted deductions never leave negative stock.
Scope: shared goods affordability and atomic deduction, build/recruit/repair, rite offerings, gift/transfer/trade, capability checks.
No scope: kharis eligibility, balance, slakt, existing persistent event meanings.
Acceptance: literal DB regression via HTTP build; real-shortage refusal; nonnegative deduction; shared consumer checks.
Stop: conflicting game semantics require Claude's review.
Proof: fresh PG baseline and final Go, vet, JS; physical epsilon-removal mutation.
Gate: blocks the geography → scarcity → bronze → elite chain through erroneous build refusal.

## A2 implementation and evidence

Core commit: `9247f382`. An absolute `1e-6` goods tolerance lives beside the cost catalogues in `province`.
`GoodsAffordable` covers prechecks and capability projections; `DeductGood` checks and deducts atomically
with the same constant and clamps only accepted rounding residues to zero. Multi-good payments retain
the existing lock/check-all/debit-all transaction and cedar substitution.

| Finding | Result |
|---|---|
| A2 | Fixed: HTTP temple build accepts raw stored stone `23.999999999999996`, inserts one queue row, leaves exactly zero. Literal 16-cost residue also tested. Shortages of `1e-5` and `0.1` still refuse without debiting; surplus pays exact cost. |
| A1 | Claude owns the existing kharis change; untouched. |
| B1–B10 + C | See the separate text slice below. |

Evidence uses PostgreSQL 16, fresh empty DBs migrated to `181|f`, and real authenticated HTTP handlers.
`a2-original-baseline.log` runs the new HTTP regression against **original e62569f8 production sources**
in a detached worktree: named failure, HTTP 422, identical misleading shortfall. This baseline was
captured retrospectively to corroborate the original-code failure independently of the mutants.
There is no running-server `/healthz` provenance: these are Go test processes, not a claimed UI playtest.
The fixture writes the exact floating-point DB value intentionally; an ordinary UI cannot reliably manufacture it.

Physical mutation recipe: `python3 docs/reviews/speltest-fixar/proof_a2.py`.
Each arm uses its own fresh DB, asserts named regression failures, and restores original source bytes in `finally`.
- Epsilon set to zero: HTTP build and **both** multi-good/atomic payment paths refuse the residue.
- Clamps removed: HTTP build and both payment paths leave a negative DB value; tests reject it.
- Identical source bytes restored: the same named tests pass. SHA256 evidence: `a2-restoration.log`.

Inventory: build/recruit/repair through `deductGoods`; separate build silver, internal gifts,
transfer/foreign gifts, trade offer escrow and acceptance through `province.DeductGood`;
rite offerings through `GoodsAffordable` with locked goods and existing clamped exact deduction;
DTO building/recruit/prayer affordability and capability recruit/trade/road checks use the same rule.
Recurring deliveries reuse the atomic deduction; voyage provisioning uses the same comparison and existing clamp.
No new G1 edges, schema change, event type or event reinterpretation.

Remaining strict comparisons are intentional: kharis eligibility is not a goods payment;
upkeep is a separate recurring budget model; slaughter is explicitly awaiting START;
kingdom tribute is gated post-MVP. No changes to those mechanics.
The server rule serves both web and keryx through their existing requests; no client/wikicode change
is needed for this bug fix, which changes neither the displayed costs nor the documented rules.

Checks: `a2-final-go.log` is the final-source fresh full Go run; `a2-vet.log` vet;
`a2-js.log` 633 JS tests, zero skipped. `a2-full-go.log` is an intermediate full run before the final
consumer inventory and is not substituted for the final run. No visual layout changes; no BILD gate.
No merge, push or deployment by Codex. Claude owns review and integration.

Final results: full fresh Go green (world 139.395s), vet clean, 633/633 JS green with no skips.
Production sources and tests are byte-identical to core `9247f382`; later commits contain evidence only.

## B1–B10 + C — text slice

Problem: a stranger sees contradictory food forecasts, obsolete costs, hidden refusal reasons and blank/misnamed notices.
Contract: repair the authoritative inputs and their CLI/web/Codex wording; preserve mechanics, FOW and persistent event meanings.
Scope: existing placement forecast, catalogue, shared welfare predicate and player formatters.
Gate: blocks legible progress through geography → scarcity → bronze → elite.
No scope: A1, slaughter, exact hidden expedition-leg timing, new historical discovery tracking.

| Finding | Result / reason |
|---|---|
| B1 | Forecast exposes full population consumption, grain/fish production and total food net from the SAME starting-placement calculation. Both clients read explicit fields; deficits say the stock lasts before hunger. Centre-first list order is repaired: Disk was coordinate-sorted, so view[0] could mistake a coastal founding for an inland farm gift. Centre + Ring preserves all19 hexes/FOW and restores the promised wire ordering. |
| B2 | Warn truthfully about existing grain-based welfare, even if fish covers meals. Prediction and the actual welfare handler use the SAME unchanged predicate owned by loyalty: empty grain store OR grain production below demand. Claude explicitly approved this choice. No new welfare model. |
| B3 | Timber warning reads BuildingSpecs; test changes the live catalogue and checks the warning follows. Native status shows16/8/8/8. |
| B4 | Already correct in current source. A real PG/HTTP test with capital and colony reads the SAME realm14.7 from player_world_records. Both CLI and web consume that province field. No imaginary per-city fix. |
| B5 | rite --list floors the shown precision to one decimal, so4.99 says4.9, not5.0. Server still compares exact pool values. |
| B6 | net in garrison; stock runway clauses identify grain vs silver. Source scan also fixed city [stad] → city [city] and both march-preview surfaces say Pay. Remaining accented words are comments, actual names or imported language/context data. |
| B7 | The prayers introduction now names the five actual Minoan prayers and points to the authoritative list. |
| B8 | Wording narrowed to the expedition's search area and recorded resources/cities. Truly NEW discoveries remain UNBUILT per order: unit_expedition_seen records seen coordinates, not pre-expedition knowledge. Selecting newly learned terrain requires historical knowledge tracking/new payload semantics; this is reported to Claude, not fabricated from today's FOW. |
| B9 | UUID poseidon_gift caused JSON decoding into bool to discard the entire CLI summary. Reader now accepts archived bool or UUID payloads. Real native founding notice is readable; web's existing truthy reader is regression-tested on UUID too. |
| B10 | TradeDelivery is displayed neutrally as Delivery arrived on CLI/web; a transfer is no longer called a trade. Persistent event kind unchanged; bilateral deliveries remain correctly described as deliveries. |
| C | Help names POLEIA_CONFIG; actual CLI errors print just error + command help pointer; redirect message gives the CLI command; server supplies no-temple/kharis/offering reason; Ready includes it; Pay replaces Sold on both preview clients. |

Source ownership remains unchanged: economy owns food math, loyalty owns its welfare discriminator,
province owns costs and kharis reads the realm pool. No new G1 edges, migration or mechanic.
API additions are additive; old archived notification payloads remain readable.
The public catchment list is now centre-first as its existing clients/comments already promise.

Verification recipes: proof_b.py (ten physical mutants, separate fresh DB for each DB arm,
explicit available GOTEST_PORT, exact byte restoration); native_b.py (own random-port PG/Redis/server,
normal registration/join/founding and actual CLI/browser requests, no SQL writes or payload overrides).
The recorded port collision was an infrastructure failure and was never counted as mutant RED.
The initial native run exposed the centre-order bug despite its other checks passing;
its artifacts are retained as a diagnosed baseline, not substituted for the corrected native final run.

Final full Go, vet, JS and native re-run are recorded alongside this report; final results below.
TEXT is for subsequent stranger playtest, not a new visual layout/BILD approval gate.
Claude owns review/merge/deploy. The later slice3 ETA/timber and olive visual work are separate.

Final B/C results: fresh full Go181 GREEN (world138.878s), vet clean, 638/638 JS with zero skips.
Ten physical mutants are named RED followed by byte-identical restoration and GREEN.
Final native process: health255620db/181, actual founder forecast5.0 consumption + correct Coastal/Poseidon
gift, zero-stock welfare warning, status catalogue16/8/8/8, no-temple readiness, UUID founding notice,
help/error output, integrated river UI POST202 followed by physical marching, zero browser errors.
`native-final/` is the corrected run; `native-initial/` documents the diagnosed centre-order baseline.
Final sources/tests are identical to255620db; this closing change contains evidence only.
A2 was integrated by Claude asb2e13145; river core asf5261fb4, docs1139c835.
