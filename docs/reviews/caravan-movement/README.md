# Caravan movement

## SAMMANFATTNING

Status: **WIP handed back to Claude for project leadership, review and integration (Timothy, 2026-10-07)**. Baseline master `abd9615e`, migration159. Implementation is checkpointed; acceptance and shipping gates remain open. No merge, push or deployment has occurred. The slice supports the geography → shortage → bronze → elite gate by making physical trade obey actual geography. No playtest agents, reseed or new encounter rules.

## SLICE-KONTRAKT

| Field | Contract |
|---|---|
| Problem | Goods use conflicting distance/time formulas and can cross impassable land without an actual route. |
| Player truth | Transfers and accepted trades follow a saved A* path, with terrain-based caravan time at 1.5× unrounded march cost; sea transport still needs a real eligible ship. |
| Invariant | A refused route changes no goods, ships, offers, transport or jobs. Accepted route coordinates and entering-hex costs are saved at dispatch and never searched again for current-route reads. Total cost is rounded once to nearest whole tick (minimum one for a dispatched leg); reverse legs are priced for their entering terrain. |
| Scope | One additive migration for transports; canonical province journey planning; transport persistence/position; economy trade arrival/returns; combat standing orders and goods dispatch; trade/accept/gift consumers (including compile-only gated consumers); removal of old trade-time helpers; API/CLI/web/Codex ETA semantics; focused architecture guard and proof tools. |
| Non-scope | New encounter/combat/pursuit rules, ship rules or capacity balance, route-line renderer (movement2b/BILD), courier movement, kingdom enablement, reseed and autonomous playtest agents. Historical scheduled jobs retain event meaning and their committed ETA. |
| Acceptance | (1) Disconnected land/shipless sea reject atomically. (2) Bent mixed-terrain route persists correct costs and one rounded ETA. (3) Transfers, standing outbound/return, buy/sell and ship returns share planner; naval needs eligible ship. (4) Existing in-flight legacy rows retain ETA and explicit compatibility. (5) Real isolated register/join/found/trade lifecycle through web and compiled CLI completes, all relevant tests and physical mutations pass. |
| Stop | A new canon choice, frozen event reinterpretation, FOW expansion, BILD change or concrete playtest prerequisite is needed: bring the concrete conflict to Timothy. |
| Proof | Fresh PostgreSQL16 baseline/final package and full suites; failure injection/concurrency where touched; position tests without tile reads; real isolated player lifecycle twice; physical mutations of route rejection, persistence, rounding and ship requirement; migration up/down and live hash/health/migration/assets/log evidence. |

## BASLINJE

Inventory: `30+2·hex` in transfer, standing outbound/return, economy ship return and interception damaged return; `TradeTicksPerHex` in player trade, gift and gated tribute. Transport reads re-search routes or use straight interpolation. Naval ships are already bound transactionally; this rule is retained. Fresh baseline seven-package suite passed (`/tmp/megaron-caravan-baseline.log`). Runtime provenance for the final implementation remains pending.

## EXPERIMENT

Implemented: canonical saved raw-cost A* journey, 1.5× total march cost rounded once, additive migration160, transactional dispatch/return schedules, naval carrier enforcement, saved position reads, trade/transfer/standing/gift/plunder consumers and authoritative schedule ticks across server/CLI/web/Codex. Legacy NULL journeys keep their committed ETA. Review atomic event/notification changes in economy and saved-water-endpoint ship release in transport.

Evidence already obtained:
- Fresh focused core, consumer and API/CLI suites passed. Logs: `/tmp/megaron-caravan-core-tests.log`, `/tmp/megaron-caravan-core-economy-atomic.log`, `/tmp/megaron-caravan-consumers-focused3.log`, `/tmp/megaron-caravan-root-tests.log`.
- All 379 JavaScript tests passed: `/tmp/megaron-caravan-js.log`.
- Fresh migration159→160→159→160 preserves a legacy naval row and rejects half-populated journey columns: `migration-proof.json`, runner `tools/caravan_migration_proof.py`.
- Broad eight-package handover run initially passed seven packages; two handlers courier tests failed because the new economy fixture queued an empty messenger ID, which their UUID-cast query sees across worlds. Fixture now uses a valid UUID. Recheck passed all eight packages on fresh PostgreSQL16/migration160, exit0: `handover-tests.log` (original `/tmp/megaron-caravan-handover-tests-final.log`).

**Claude review + proofs (2026-10-07, Claude took over from Codex):**
- Time unit: 1 tick = 1 game day (TicksPerDay deleted 2026-08-06). Timothy's rule (2026-10-07): fighting unit 1×, courier 0.5×, caravan 1.5× of terrain cost, identical for internal transfer and player trade. This is what the planner implements; the old `30+2·hex` minute formula was an hour-era leftover (transfers were ~1 tick at any distance). Examples: 15 plains hexes ≈ 17 ticks for a caravan vs ≈ 11 for an army; coastal sea 0.6/hex. No ETA question remains for Timothy.
- Full fresh-DB suite (`tools/gotest.sh`, mig160): `go vet` clean, 24 packages ok, race clean on province/transport/economy/combat/messenger/api/handlers.
- Claude core mutations, each assertion-red then byte-restored green: floor instead of half-up rounding (TestTradeJourneyTerrainAndRounding), no-path accepted (TestTradeJourneyRejectsDisconnectedAndUsesWater), naval without ship (TestDispatchNavalRejectsNoCarrier), journey not persisted (TestDispatchPersistsJourneyAndRejectsNoPathAtomically). Codex consumer mutations commit-before-schedule / eta-plus-one / trade-commit-before-schedule: red→green.
- Legacy in-flight (`trade_legacy_test.go`): journey-less ThenReturn delivers with NULL-journey return on the old timing; a legacy naval delivery with no sea route home, and a ThenReturn without destination_id, roll back fully (worker would dead-letter after 3 tries). Accepted: the return route is planned on the same map as the outbound leg and the server writes ThenReturn itself, so neither occurs in normal operation.
- Real player lifecycle (`tools/caravan_live.py`, own PG16/Redis, build b52a694, mig160, web accept + compiled CLI): buy outbound 15 ticks + return 15 ticks delivered; sell outbound 8 ticks delivered, return lost (probably the 5 % `tradeRiskPct` roll, not confirmed in the log). browser_errors [], failed_jobs 0, none in transit. Script fix: `keryx sent` → `keryx outbox`.

Previously unproven (now covered above): physical mutations, full `./...`/vet/race, final real gameplay. The first client mutation attempt is INVALID evidence: concurrent unfinished test compilation failed both mutated and restored runs. Source was restored; do not count those logs as a red→green proof. Mutation runners are checkpointed but need reviewed execution. Gameplay runner reached a baseline naval BUY accept only; no completed final-code trade lifecycle or proof.json exists. Scenario selection/sea reachability still needs work; if using a read-only graph oracle, document it as diagnostic fixture selection rather than blind player exploration.

Claude additionally requested before/after ETA examples for short land, long land and sea, core review, and independent rejection/rounding mutations (chat 16:14). Those comparisons remain pending; the 1.5× terrain rule was already explicitly decided by Timothy.

## GRINDAR

Open: final code review, semantic/user acceptance and drift. This checkpoint is not ready to deploy. No new BILD is planned; player wording is TEXT and recorded for playtest.

## METODISK LÄRDOM

Pending.

## KÄNDA AVGRÄNSNINGAR

Historical in-flight jobs retain previously committed timings. Route illustration and R3–R8 encounter migration remain separate work.

## Resume checkpoint

Branch `codex/caravan-movement`, worktree `/tmp/megaron-codex-caravan-20261007`. Contract written before production edits. Read this report, git status/log and main `.agents/board.md`/chat before resuming. Do not touch another worktree's uncommitted files. Claude now owns project leadership, integration/push/deploy and the remaining verification. Canon/BILD still requires Timothy. Do not treat the earlier temporary Codex shipping mandate as the current ownership instruction.


## Handover next actions

1. Review the complete branch against `abd9615e`; run the full fresh-DB suite, vet and relevant race checks.
2. Confirm route/time examples and complete valid physical red→green mutations for rejection without side effects, persisted path, rounding, carrier requirement and surface schedules.
3. Finish isolated real register/join/found/contact/buy/sell/naval-return gameplay and required transfer/standing coverage using actual clients. Keep no-reseed/no-playtest-agent constraints.
4. Update vault status in coordination with Claude, then satisfy repository integration and drift gates. No live migration160 has been applied.

Known surface boundary: server map position follows the saved route; the existing JavaScript caravan sprite interpolation/route-line renderer remains the explicitly excluded movement2b/BILD work. No new BILD or canon decision has been made here.
