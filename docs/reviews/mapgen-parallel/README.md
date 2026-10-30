# E4 — parallel map generator tests

## Contract
- Problem: sequential pure mapgen tests dominate the test suite.
- Player truth: unchanged world generation and assertions; faster developer feedback.
- Invariant: no shared mutable state or concurrent DB tests.
- Scope: pure top-level tests in internal/world, and this proof report.
- Non-scope: production mapgen, seeds, assertions, DB test parallelism, E5.
- Acceptance: baseline and parallel runs pass on separate fresh DBs; faster package time; race run and full suite pass.
- Stop: shared state requiring production changes, or generation/assertion changes.
- Proof: audit global writes and filesystem/DB access; baseline timing; parallel timing; race detector; scheduling mutant; full fresh DB suite.

Gate: infrastructure supporting chain verification. Visual/player-surface acceptance is inapplicable: no production code or player-visible content changes.

## Baseline and audit
Unchanged d499d83d, fresh migration 228, Go 1.27.3-X:nodwarf5, 16 logical CPUs: `tools/gotest.sh ./internal/world` passed in 158.665 s. Package globals (terrain/direction/color tables, fragDebugLog, coverageSizes and deposit predicates) have no writes after initialization. RNG and maps/slices are per invocation. Golden JSON is read-only; debug export uses t.TempDir. No Setenv/Chdir/global logger changes. Entire river_metadata_store_test.go stays sequential, including its pure metadata test; nested subtests and seed loops stay sequential.

## Change and timing
36 pure top-level tests now call `t.Parallel()`. No assertions, seed loops, nested subtests, fixtures or production functions changed. Separate fresh migration-228 database, same machine/toolchain and command: **34.500 s**, versus **158.665 s** baseline (**78.3% less time, 4.60× speedup**). These are package times from Go's uncached test runs, not Docker setup/build time; timings are hardware-dependent single-run measurements.

## Mutation and scheduling proof
Temporarily added `var parallelRaceProbe int` to mapgen_test.go and `parallelRaceProbe++` at entry to genTiles. With clean HOME/PATH environment, `go test -race -count=1 ./internal/world -run '^TestGenerateMap_(TileCountAndBounds|Deterministic)$'` failed with DATA RACE. Restored original file bytes before final checks. The mutant proves the detector observes shared unsynchronized writes between the newly parallel tests; it changes no production code.

`tools/gotest.sh ./internal/world -race -json` events confirm all 36 pure tests pause/resume, and TestRiverMetadataBackfillIsAtomicAndPreservesSavedValues passes before the first parallel test resumes. Both tests in river_metadata_store_test.go remain sequential. `go vet ./internal/world` and diff whitespace checks pass.

## Race and review result
Full fresh-DB race run passed in 120.451 s: all 38 top-level tests passed, no failures or skips. Diff review mechanically removed the added Parallel calls/comment and compared each file with baseline: byte-identical remaining tests. The core change is 41 added lines in 11 test files, with no production changes.

## Final suite
`tools/gotest.sh` passed on another fresh migration-228 database: all Go packages and 875/875 web tests green. World package time was 34.719 s, corroborating the isolated 34.500 s run. Baseline, timing, race and full-suite arms used separate fresh databases.

## Deploy
Merged/pushed/pulled `be249327c8d2d1e24d3d9e96d87f42eea72941b6` on CT126 (2026-10-30 10:10). Remote HEAD matched; clean checkout; 36 Parallel calls and zero in the DB file verified on server. Service active, health ok, migration 228. Running executable remains `5f51906`, as before: only test files changed, so no runtime restart was needed. Todo and vault now point to E5.
