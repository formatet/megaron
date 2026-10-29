# Sequential todo pass

Requested: twelve problems, then select twelve more. Work stays sequential; a pending human decision is not counted as completed.

First twelve, drawn from megaron_todo on 2026-10-29:
1. Remaining terrain production articles and their DB-backed checks.
2. Determine whether historical OfferExpired notices still need their Codex association.
3. Shared Storehouse under the approved order.
4. Shipyard test isolation left on codex/varvet-isolering.
5. Reproduce shipyard units/settlements lock ordering.
6. K6 parity records still marked unfinished.
7. Map edge beside an open drawer: native verification.
8. Telling places v1: native verification.
9. Remembered settlements: native verification.
10. Market boards: native verification.
11. Food and goods after wine removal: native verification.
12. Assemble outstanding merged visual evidence for Timothy.

## 1 — contract
Problem: eight terrain articles omit usable base production figures.
Player truth: every current terrain article states base workplaces and yield, including unavailable deep-water fishing without a harbour.
Invariant: values come from the same fully migrated DB and production owner as the city; no mechanics change.
Scope: economy terrain article test, Codex markdown/bundle, this report.
Non-scope: deposits, refining and building comparisons.
Acceptance: all fourteen current terrain articles match their base options; deliberately false values fail; generated bundle and Codex links pass.
Stop: production owner and approved terrain rules disagree.
Evidence: baseline passed on fresh schema 228; extend the existing DB-backed test, mutation, regenerated bundle and relevant guards.
Gate: proves geography → scarcity through the reachable in-game Codex.

Eight added articles: limestone/red mountains, maquis, semi-desert, coastal/deep sea, rivers and fords. Coastal water states the approved bay exception; the base fixture has no land neighbours. Deep water explicitly reports zero workplaces without Harbour. Existing city/API/Keryx production code is unchanged; Keryx Codex reads the same static articles as the web bundle.

Baseline: TestCodexTerrainProduction passed on fresh schema 228 at 061eeecc. Eight physical table mutations each failed their named comparison, then restored byte-identically. Codex links initially rejected a missing citizens link in rivers; fixed before final guards (29/29). No new layout; TEXT comprehension remains with playtest. Deposits/refining and harbour-level examples remain outside this base-terrain contract.

Final checks: full Go suite on fresh schema 228 passed (world 156.792s), 865/865 JS, economy vet, Codex facts/day guards, eight real Keryx article reads over isolated HTTP. Initial full Go run failed because /tmp's quota was exceeded; separate test-infrastructure commit 657c6a18 permits explicit GOTMPDIR, and the entire suite was rerun using /home/tk/.cache/megaron-gotmp. No shared temporary files or other agents' containers were removed.

Deployment: 4e492254 pulled on CT126, nine static assets byte-identical, schema 228 clean, same world and entity counts. As expected for static/test-only changes, running server remains 061eeec. `deploy-proof.json` contains checkout, process and ground evidence.

## 2 — historical OfferExpired association
Contract: determine whether removal breaks an existing player entry point; do not change frozen historical event semantics. Scope: read clients, index and expiry handler; prove existing mapping with its guard. Gate: proves correspondence remains explainable.

Result: keep the association. `economy/trade.go` no longer emits expiry notices, but `format.js`, `dispatch_window.js`, `dispatch_actions.js` and Keryx still read historical OfferExpired entries. The dispatch's “Read about this” relies on the Codex kind mapping. Physically removing OfferExpired from index.json failed the existing guard with “notification kind OfferExpired has no Codex article”; exact restoration passed. No production change or deployment needed. Closed as investigated/retained, not as removed.
