# ORDER 8 — First to the selected hex

## Contract

Problem: Explore silently substituted a nearby unseen hex for the selected destination.
Player truth: first to the selected hex, then explores around it, including a selected hex already seen.
Invariant: no substitute first destination; saved legacy legs and event semantics unchanged.
Scope: initial combat planning; HTTP preview/receipt; web/Keryx text; Codex sight article.
Excluded: later-leg selection/coastal priority, physical provisions, return budget, risk and report mechanics.
Acceptance: selected target persisted; unreachable selected target rejected despite reachable alternate;
second leg uses existing deterministic exploration; legacy saved destination retained; four surfaces agree.
Stop condition: new canon/event interpretation required. Claude approved the contract in chat (07:35).
Proof: separate fresh migration-198 baseline/final DBs, functional tests, physical nearest-unseen mutation,
full Go suite once, JS, vet; TEXT comprehension left to playtest. Proves geography part of chain gate.

## Baseline

`baseline-go.log`: unmodified 4054bd63, fresh schema 198, expedition and field Explore tests green.
Original `StartMarch` changed selected target to `nextExpeditionLeg.Target` before persisting it.

## Implementation

Validate the exact chosen destination before area planning, and retain it in the saved first march.
`nextExpeditionLeg`, arrival handlers, saved event payloads and migrations are unchanged.
Existing known-area/usable-unseen checks and return/provision budgets still apply.
API preview adds `planned_first_target_q/r`; new dispatch receipts add `first_to_area:true`.
These are HTTP presentation fields only, absent from persisted missions and historical events.
Older servers/missions retain generic area text. The preview's hidden route ETA remains unavailable.
Lifecycle fixtures now request their former actual first destinations (4/5 instead of 9/10), retaining
original time/provision assertions. Roaming-near-home requests (1,0), preserving own-hex refusal.

## Evidence

Pending final verification. Tests include Timothy's exact galley target (24,20), alternative (23,16),
both deep_sea; actual persisted first destination must be (24,20). Coast preference starts at leg two.
No claim of having changed the live world; this measurement is an isolated real-PG fixture.

## Gates and limits

Function blocks merge and is covered by real-PG tests. TEXT comprehension belongs in playtest,
not a BILD gate; no visual layout change. Claude integrates and deploys, then records LIVE in the vault.
The live Timothy fleet is not modified. No migration, risk, cargo or matlager code changes.
