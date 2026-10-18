# Slice 20 — ta bort lojalitet

## SLICE-KONTRAKT

- Problem: per-city loyalty has no working revolt outcome and produces recurring noise, own-colony gifts and hidden combat variation.
- Player truth: combat commits 90% per round and retreats when 20% of starting strength remains unless ordered to hold. Own logistics uses transfers; foreign gifts remain physical.
- Invariant: no new loyalty work or state; old scheduled identifiers finish without retry/dead-letter; foreign gifts and historical event payloads survive.
- Scope: server loyalty/model/schema, consumers/fixtures, fixed combat and retreat, own-gift retirement, CLI/web/Codex.
- Non-scope: kharis rules (slice19), gated kingdoms restoration, future revolt mechanics.
- Acceptance: no loyalty seeding; .90 participation for every city/kharis; .20 remaining-strength retreat; factor1 upkeep; own gift rejected without debit and foreign gift unchanged.
- Stop: unresolved canon or event history migration conflicts go to Claude/Timothy.
- Proof: pristine master baseline and new red tests, migration lifecycle, fresh PG suite/vet/JS, physical mutations, natural CLI/Firefox.

Gate: removes a hidden rule and false affordances on the geografi→brist→brons→elit chain (blocks).

Baseline: `df98d5ed`, migration184. Existing targeted baseline green (`baseline.log`). New no-loyalty-seeding contract RED (`baseline-seed-red.log`: four retired ticks still seeded). Baseline/fix use separate fresh PG databases.

## Resultat och bevis

Production commits: `dccd5f3e` (implementation), `19da729d` (fortify wording/comments), `1630ba85` (retired route inventory) on `codex/ta-bort-lojalitet` (base `df98d5ed`). No merge, push, deploy or live writes.

| Gate | Evidence |
|---|---|
| Baseline | Existing subset GREEN; new seeding, own-gift and actual battle-participation tests RED on master184 (`baseline*.log`, `combat-baseline-red.log`). Own gift previously returns200, sends100silver and grants loyalty; now410 with unchanged city/goods/jobs. |
| Data | `TestRemoveLoyaltyMigrationPreservesOtherStateAndAudit`: actual185 up removes schema, retires4failed+4future jobs, leaves FoodTick pending and city/goods/full historical LoyaltyChanged audit JSON unchanged. |
| Queue | `TestSeedDailyTicks_NoLoyalty`, `TestRetiredLoyaltyTicksCompleteWithoutEffects`: no seeding; old types finish once without events, replacement jobs, retry or dead-letter. |
| Combat | `TestBattleParticipationFixedAcrossCityAndKharis`: real handler and round90active/90dice at kharis1/100, with/without capital, fortified defender. Fixed .20 **remaining strength**, factor1 unpaid desertion; hold remains. |
| Compatibility | Both historical `ByLoyalty` and `by_loyalty` courier payloads decode; new output emits `when_losing`. Existing percentage/hold:false couriers still handled. Historic queue identifiers and notification renderers retained. |
| Foreign gifts | Existing real API/FOW/dispatch/delivery/carrier/history tests GREEN in full API suite; foreign gift/tribute implementation unchanged. |
| Mutation | Six physical named RED→byte-identical restore→fresh185 GREEN: participation,rout,desertion,seed,migration,no-op. Reproduce `combat-mutations.py` / `retirement-mutations.py`; logs beside them. |
| User | `native.py`: isolated fresh185 PG/Redis, CLI login/join/found/status/city/history/retreat hold/reset, actual API without loyalty, Firefox1280+390 City More and War; zero pageerrors. See final `native-final/evidence.json` for process commit185. |
| Visual/semantic | Existing loyalty/log/own-gift affordances removed; desktop/mobile same city/retreat truth as API. Screens inspected1:1. No new visual family; Claude/Timothy owns final BILD decision and TEXT playtest. |

Validation: full `tools/gotest.sh` fresh185 **GREEN** (`internal/world`148.633s), `go vet ./...` **GREEN**, **642 JS tests/0fail**, Codex13tests after final wording edit, four Day guards **GREEN**. Static verb-parity inventory and tool tests saved. Full Go ran on implementation code; afterwards only fortify prose/comments and inventory metadata changed.

Final natural process provenance: `native-final/evidence.json` reads `/healthz` **commit19da729d / migration185 / statusok**, tracked production dirty=false; zero pageerrors. `1630ba85` only changes tool inventory metadata. Native proof repeated from ordinary registration/join/founding after closing prior drawers correctly. Images in `native-final/` inspected at1:1.

## Avgränsningar och återstående referenser

- The old own-colony POST remains a **410 tombstone**, with transfer guidance and no DB access; the loyalty-log route is removed. CLI/web/capabilities no longer offer the action.
- Frozen history: original SQL migrations and prior review artifacts remain unchanged. `remaining-references.txt` classifies current remaining code references: retired identifiers/priority/no-op, historical order decoder/old-key clearing, historical founding-notice renderers, negative regressions, historical snapshot event filter. No live loyalty state reader/writer remains.
- Removed `revolting` Go state has no reachable writer/reader; DB state is free TEXT, so no rewrite of historical state values. Kingdom functionality remains gated and otherwise unchanged; succession uses active→population DESC→id ASC, approved by Claude17:20.
- Current CLAUDE.md intentionally unchanged per order. Stale CLAUDE.md lines77–78,121,123,171: hybrid-event-sourcing bullet ("Only loyalty is replay-derived"), G1 loyalty row and combat dependency, Loyalty design guardrail. Claude should update these at integration.
- Vault integration belongs to Claude: remove own-colony gift parity row, rename retreat flags `--when-losing`; mark temenos_settlement loyalty/revolt sections historical, update relevant combat/settlement descriptions, and retire this verification todo at merge. Telemetry explicitly excludes the retired gift row until the vault/snapshot is updated.
- Reseed now checks FoodTick, snapshot removes live loyalty column (use a new CSV directory), client proof/mutation scaffolds use when_losing.

Method note: first native pass correctly exercised the DOM but left War open over the mobile City screenshot. Fixed the **rig** to close the prior drawer and reran whole natural flow (`native-corrected/`), then repeated on production commits (`native/`, final `native-final/`). Initial mutation runner port collision is separately logged, followed by real named failures and greens; no collision is counted as product evidence.

