# ORDER19 — spawn consumes physical Host positions

Contract before production edits: a new join/Rise must clear the Host's physical route position and balance landmass load using that same position. Consume province.UnitPosition.Physical and injected clock/tick anchor; never mutate saved movement, change spawn spacing, ore bias or landmass viability. Scope handlers join/rise + position batch reader and DB/API tests. Existing owner from ORDER10 remains authoritative. No player text/layout change or new verb: all four surfaces retain their existing joining behavior. This fixes the main gate (a stranger can join).

Baseline d248b09e. Named tests against unchanged production: TestJoinSpawnUsesPhysicalHostClearance (free departure vicinity and occupied physical ground), TestJoinSpawnBalancesPhysicalHostLandmass, TestRiseSpawnClearsPhysicalHost. Tests use authenticated HTTP and real fresh migrated PostgreSQL. Weighted persisted route at tick3 places Host at11,0 while q/r remain0,0; candidate fixtures eliminate random selection. Baseline result pending. Production implementation, own mutation and full fresh suite pending.

## PAUS checkpoint, Claude order20:55

Production commit **f57f063f**. Named old-code baseline RED for free departure, occupied physical ground, landmass choice and Rise. First fixture-offset attempt expected10 while the existing owner correctly returned11; preserved separately in baseline-fixture-offset.log, corrected before the actual baseline-red.log. No production changed for that correction.

Batch reader uses province.UnitPosition.Physical, one injected clock instant/anchor; both SQL clearance and landmass CTE consume its physical arrays. Join and Rise including same-landmass fallback covered. Added4hex blocked/5hex free boundary and inactive-phase cases. Saved route/q/r invariant stays unchanged. Targeted fresh200 GREEN. Own physical mutation Host clearance<=4→<=3 gave named four_hex_boundary_is_occupied RED; exact SHA restore then fresh GREEN.716JS GREEN, vet GREEN.

**Full fresh Go suite deliberately TERMINATED at Claude PAUS order; full-fresh.log is NOT a green proof.** No final delivery or integration approval. Resume must run full fresh suite after higher-priority13→17→15 and rebase if instructed. No live write, merge, push or deploy. No UI text/markup change, no BILD. No ORDER20/21 work started.
