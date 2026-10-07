# Caravan movement

## SAMMANFATTNING

Status: contracted, implementation and proofs pending. Baseline master `abd9615e`, migration159. This slice supports the geography → shortage → bronze → elite gate by making physical trade obey actual geography. Timothy explicitly authorizes Codex to integrate, push and deploy once applicable gates pass (2026-10-07), overriding the usual Claude handover role for this slice. The architecture programme's after-playtest sequencing is superseded for this expressly ordered slice only; no playtest agents, reseed or new encounter rules.

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

Inventory: `30+2·hex` in transfer, standing outbound/return, economy ship return and interception damaged return; `TradeTicksPerHex` in player trade, gift and gated tribute. Transport reads re-search routes or use straight interpolation. Naval ships are already bound transactionally; this rule is retained. Tests and runtime provenance pending.

## EXPERIMENT

Pending.

## GRINDAR

Pending: code, semantic, user and drift. No new BILD is planned; player wording is TEXT and recorded for playtest.

## METODISK LÄRDOM

Pending.

## KÄNDA AVGRÄNSNINGAR

Historical in-flight jobs retain previously committed timings. Route illustration and R3–R8 encounter migration remain separate work.

## Resume checkpoint

Branch `codex/caravan-movement`, worktree `/tmp/megaron-codex-caravan-20261007`. Contract written before production edits. Read this report, git status/log and main `.agents/board.md`/chat before resuming. Do not touch another worktree's uncommitted files. Integration/push/deploy are authorized for this slice after gates; canon/BILD still requires Timothy.
