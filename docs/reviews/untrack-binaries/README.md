# E3 — untrack generated binaries

## Contract
- Problem: three compiled executables are tracked and copied into every checkout.
- Player truth: server and Keryx continue to run from builds of current source.
- Invariant: deployment must not consume the removed prebuilt files.
- Scope: .gitignore and server/{cli,poleia,cmd/keryx/keryx}; this report.
- Non-scope: history rewriting, game rules, E4/E5.
- Acceptance: no tracked binaries at these paths; all three ignored; source builds and fresh full suite pass; deployed service remains healthy.
- Stop: any runtime or deployment dependency on these artifacts.
- Proof: inspect build recipes, baseline tracking/ignore checks, remove an ignore rule to falsify check, full fresh DB suite, build source, read-only deployment verification.

Gate: infrastructure support; no new player surface. Visual and gameplay acceptance are not applicable because executable source and assets do not change.

## Baseline
Master 2e874ce9 tracks all three executables and ignores none. Dockerfile builds ./cmd/server into /build/temenos and copies that output; development Air builds its own runtime executable. No source/deployment-script reference to the three legacy paths was found.

## Verification
- Removed 34,597,692 bytes of generated binaries from future checkouts (history remains intact).
- Tracking and ignore checks: all three paths untracked and ignored. Removing `/server/cli` from `.gitignore` made the check fail; restoring it passed.
- Source builds of `./cmd/server` and `./cmd/keryx` passed; Keryx `--help` runs. `go vet ./...` passed.
- Live recipe inspected before removal: Air builds `./cmd/server` to `/tmp/air-poleia/poleia-server`; it does not read the legacy files.
- Pre-deploy health: commit `5f51906`, migration 228, status ok. This infrastructure-only change should not restart or rebuild the unchanged running executable.

## Baseline repair
The full suite exposed an existing stale text assertion in `TestStatusBuildSinkUsesAllPlacementHexes/stock_127`: it searched for “produces with no receiver” although the current warning says “makes more than known sinks absorb”. Reproduced on untouched master with its own fresh migration-228 database. Separate commit aa1d032d updates only the expected warning fragment, retaining the under/at/over-capacity assertions. The entire Keryx package then passed on another fresh database (0.490s). One attempted run could not allocate a randomly selected Docker port; retry used a new container and passed.

## Final checks
Fresh migration-228 full suite: every Go package except the separately reproduced/repaired Keryx assertion passed; corrected Keryx package passed on a fresh database. All 875 web tests passed. Build, vet, diff checks and ignore mutation passed. No gameplay, assets, or schema changes.
