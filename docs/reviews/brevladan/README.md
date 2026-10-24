# Brevlådan — proof package

Five overlapping diplomacy tabs become **Letters · Runners**. Letters groups the complete messenger history by Wanax, puts the nearest action first, and opens a chronological exchange with trade decisions, recorded gifts and a bottom composer. Private drafts/notes/citations and case surfaces are removed; frozen recipient records remain.

Runtime code: **09fb66dd** on `codex/brevladan`. Claude integrates; Codex has not merged, pushed or deployed. [Picture gallery](index.html), [final source verification](final-source-verification.json), [test logs](tests/).

Two independent native runs are **PASS**: [run1](run1/proof.json), [run2](run2/proof.json). Each uses its own newly created PG16 database, Redis, server and temporary Keryx binary, normal register/join/found, explicit stocks/contacts, real HTTP letter/offer departure, and actual-worker arrivals. Firefox1280/390 proves the three state words, first-action ordering, chronological thread/latest open, Write contacts, Runners, and no horizontal overflow. Native Accept leaves **Agreed: 2 copper for 5 silver** in the exchange; native reply changes the existing bearer without creating a fifth messenger. Real Keryx lists people, resolves name/UUID, reads JSON and sends a reply. Both runs have **0 page errors**.

Actual `/healthz` in both native processes: `commit=c5e08ef+brevladan`, `migration=210`, `status=ok`. This is a source-build label, not a claim those processes carried the later commit stamp. All **296 runtime source files** were SHA256-captured **before each build**, asserted unchanged after each complete scenario, and compared with committed09fb66dd: both manifests equal, **0 mismatches**. Includes all affected Go runtime packages, webJS, templates, Codex and CSS. Raw per-run manifests and final-source-verification.json establish the link.

An explicitly recorded `GiftDelivered` event fixture tests gift projection/display/privacy: **Myrine · Gift received: 1.5 copper**, with neither its deliberately secret origin name nor origin ID disclosed through the thread/history. This fixture proves reading an outcome; it does **not** claim a real gift voyage or stock credit was exercised. The native trade and messenger arrivals/reply above use production handlers.

## Slice contract

| Field | Contract |
|---|---|
| Problem | Five overlapping diplomacy tabs obscure the next action. |
| Player truth | One row per Wanax: who waits, response deadline, last line, then the exchange and next button. |
| Invariant | Frozen recipient, physical delivery/return knowledge, geography privacy and existing escrow rules remain authoritative. |
| Scope | Server messenger projection and removed private routes/tables; Keryx, web and Codex; proof rig. |
| Non-scope | New travel, interception, expiry or escrow mechanics. |
| Acceptance | Correct first-action sort/state, retained agreements/gifts, same-bearer reply, reachable Write list, two-tab mobile/desktop layout. |
| Stop | New information/mechanics decisions outside the written order go to Claude/Timothy. |
| Proof | Baseline, DB/HTTP tests, named source mutations, freshGo/JS/vet/Codex, two native independent runs. |

Gate: **proves** the chain through legible trade and correspondence. Images are BILD material; Timothy's 2026-10-22 instruction permits review after merge.

## Baseline and verification

[Baseline](baseline/proof.json) is an independently owned runtime reporting `c5e08eff`, migration209 from `/healthz`. It contains Firefox1280/390 images of the old five-tab UI, raw Queue JSON, real Keryx output and fixtures, with0pageerrors. Baseline makes no claim about a missing `/letters` endpoint: it records the existing surface/data.

| Gate | Evidence |
|---|---|
| Code | Fresh migration210 full Go command PASS (26 tested packages, world155.972s) in tests/fullfresh210.log; final full CLI package and vet logs cover the later small CLI goods-print delta. JS745 and Codex14 pass. No second full suite is claimed. |
| Mutation | Waiting-on-you sort and state source mutants fail by name, byteexact restore passes; client state-word mutant also fails. Logs in tests/. |
| Visual | Gallery A1–C2 at native1280/390; baseline D1–D2. No CSS overrides or page-routing mocks. |
| Semantic | Three state words agree with available actions; accepted terms remain; original-recipient/FOW tests are in server logs; secret gift origin absent and exact decimal goods retained. |
| User | Both independent fresh loops register/join/found, read actual-worker delivery, use real Keryx and native browser Accept/Reply, and record unchanged messenger count. |
| Drift | Not deployed by Codex. |

## Reproduction and fixtures

`python3 tools/brevladan_acceptance.py docs/reviews/brevladan/run1 [BUILD_COMMIT]`

Baseline mode reads the unchanged source tree selected by `BREVLADAN_SOURCE` with `BREVLADAN_BASELINE=1`. The harness creates/removes only uniquely named owned containers and temporary binaries on dynamic ports. It never uses shared acceptance8097 or production. The final proof loop holds the configured3600tick cadence; only the three explicit arrival timers are advanced to current100. Their outcomes are processed by the actual Arrival worker. Stocks, human Wanax names, mutual contacts and the separate gift outcome fixture are preserved in fixture.sql. Fourth messenger remains outbound until its normal arrival. No raw access tokens are stored in the report.

## Harness corrections and limits

- `attempt-pg-startup/`: Unix readiness saw PostgreSQL's temporary startup instance and the server connection reset before migration. Fixed by waiting for TCP readiness.
- `attempt-small-island/`: the first host stood on a small dry island; searching for partners exhausted spawn sites (`join`409). The rig now chooses four ordinarily spawned actors on the largest dry component.
- `attempt-back-navigation-race/`: the harness counted the old Write-back control while async Runners→Letters was replacing it, then clicked a removed control. Fixed by waiting for the new person row. Its eight screenshots passed before the harness timeout.
- `intermediate-run1/2`: complete earlier passing loops with a narrower initial manifest. Final run1/2 supersede them with all296runtimefiles and gift-fixture coverage.

All raw attempts are retained, and none is represented as a product regression or a passing final scenario. No online presence/read-receipt semantics, transport risk, offer duration or historical event interpretation was changed. Incoming gifts conservatively omit origin-city metadata even when that city is already known; server owner/FOW tests cover the broader data contract. Listing/opening a thread does not answer a letter or accept a trade.

## Integration notes

Measured base is c5e08eff. Master advanced independently to7ac27be0 (goods text/Construct sorting) during this slice; those unrelated changes and the main-tree city_production work were not touched or included in these proofs. Claude rebases/integrates and performs its own review/mutation. Removed feature-specific archive/private/citation tests are replaced by mailbox DB/HTTP/privacy tests; actual trade lifecycle tests now assert the person-thread projection. Migration210 only drops private/citation tables; recipient archives and triggers remain. The compatibility Queue endpoint remains, with Wanax author identity.
