# ORDER19 — spawn consumes physical Host positions

## Summary / contract

Base `0825a0ac`, migration202. Rebasing original checkpoint313aeda4 produced
runtime `b6a34c1a` and checkpoint `f1ba9954` without conflicts. A new join/Rise
must clear the Host's physical route position and balance landmass load using
that same position. Consume province.UnitPosition.Physical and injected
clock/tick anchor; never mutate saved movement, change spawn spacing, ore bias
or landmass viability. Proves the main chain: a stranger can join.

Scope: join/rise picker + batch reader and actual DB/authenticated HTTP tests.
No new verb, notification, protocol or client text/layout: temenos/keryx/web/
Codex retain their existing joining behavior. No migration, BILD or deployment.
ORDER20/21 remain outside scope, awaiting Claude's order.

## Baseline and experiments

Original baseline d248b09e, named actual old-code RED in baseline-red.log:
free departure vicinity, occupied physical ground, landmass load and Rise.
Persisted weighted route places Host at11,0 at tick3 while saved q/r remain0,0.
Single-candidate fixtures eliminate random selection. Initial fixture-offset
attempt expected10 while existing owner correctly returned11; preserved in
baseline-fixture-offset.log and corrected before the actual baseline.

One batch reads active founder projections; canonical owner resolves every
physical hex from one injected clock instant and world tick anchor. Both SQL
clearance and landmass CTEs consume those arrays. Join and Rise, including
same-landmass fallback, covered. Fourhex blocked/fivehex free and inactive
phase cases are explicit. Saved route/q/r remain unchanged.

## Verification

- targeted-rebased-green.log: actual fresh202 authenticated Join/Rise DB/API.
- mutate.py: physical <=4→<=3 Host clearance mutation, settlement rule unchanged,
  named four_hex_boundary_is_occupied RED; byte-exact restore, fresh GREEN.
- mutate-physical.py: overwrite canonical positions with saved departureq/r;
  named Join clearance, landmass and Rise RED; exact restore, fresh GREEN.
- mutation-result.json and mutation-physical.json record restored source hashes.
- full-fresh.log: final full fresh202 run after rebase; js.log and vet.log final
  checks. No additional implementation changes after rebase.
- checkpoint-20261022/: historical interrupted full suite and earlier migration200
  JS/vet/targeted/mutation evidence. Interrupted log is NOT a green proof.

Current tests preserve existing outcomes on genuine physical positions. No
client screenshot or live-world claim: authenticated HTTP requests run against
synthetic persisted movement/candidate terrain. Human experience of joining
is not claimed as a new browser/CLI end-to-end test. No new positioning engine.

## Gates / limits

Code and semantics: named OLD RED→fixed GREEN, canonical single owner, exact
movement projection unchanged. User surface: existing authenticated Join and
Rise entrypoints exercised. No graphics/layout change; visual gate N/A.
Claude owns review/merge/deploy. No live writes, merge, push or deploy by Codex.

Final result: one full fresh202 Go run GREEN26 packages (world157.471s),733 JS GREEN, vet GREEN. Runtime remains byte-identical to rebased b6a34c1a. Both physical mutations RED→exact restore/fresh GREEN.
