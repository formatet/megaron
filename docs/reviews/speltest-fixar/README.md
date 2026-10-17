# Speltest fixes

## A2 contract (complete, awaiting Claude integration)
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
| B1–B10 + C | Next, separate branch per order. |

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
