# Proof package: builders, Workshop, serialised build queue (migration 245)

Branch `sonnet/byggare-verkstad` from master `ceee8906`. Order: `.agents/order-byggare-verkstad.md`.

## Inventory (step 0) -- matched the order, no stop needed
- A build was one `build_queue` row + one fixed-tick `BuildComplete` scheduled event (`province.go` Build, `combat/build.go` handler). Max 2 rows (`maxParallelBuilds`). `complete_at` was a stored estimate.
- The shipyard already used the `remaining / calc_tick / revision` model (`sync_shipyard_work`, mig 204) with a boolean `running`; the queue now uses the same pattern (`sync_build_queue`), which is the architecture the order names ("samma remaining/rate/calc_tick-monster som varvet").
- Readers of the queue cap: `province.go` (guard + `build_queue_max`), web `ui/drawers/city.js`, keryx `cmd_build.go`, `cmd_status.go`, `cmd_staff.go`, Codex `buildings.md`.
- Shipyard placement: generic `StaffingOnly` building option (`placement_yield.go`), `settlement_placement.go` build-out, `building_demolish.go` lift.

## Baseline (step -1)
`tools/gotest.sh` on fresh DB at 244 / `ceee8906`: **0 red** (Go 934 ok lines, no FAIL; web `# pass 904 # fail 0`). The two "known reds" in the order did not reproduce in a full-suite run on a fresh DB. After my first partial run (`./internal/... ./api/... ./cmd/...`) they and a kharis test did appear, but all three passed in isolation and in the final full run: leaked-fixture/order effects, not caused by this slice; no fixture clean-up was needed.

## What changed (by surface)
- **temenos:** migration 245 (role good `building`; `units.shipyard_work_rate`; `build_queue.work_*`; `sync_shipyard_work` rate model; `build_rate`, `sync_build_queue`, triggers on placements/buildings; backfill of in-flight builds; excess shipyard placements freed). `economy`: slots shipyard/workshop = 1/2/3, `GoodBuilding`, `ShipyardBerths` (3/6/10, see deviations), workshop `BuildingOption`. `province`: `workshop` catalogue (timber 10 / stone 12, 8 game days; sits between stable and barracks), levelled, name, purpose. `handlers/province.go`: queue cap 3 / active 1 (`build_queue_max`, `build_active_max`), insert + `sync_build_queue`, queue items carry `active`, `position`, `rate`, `work_left_game_days`, `complete_at`/`complete_tick` only when active; cancel starts the next; `combat/build.go`: revisioned event, stale event ignored, completion starts the next; demolish lifts workshop placements; goods seeding skips the role good.
- **keryx:** `build` receipt ("starts when the current build is done"), `build --queue`, `status` Construction block (`buildQueueState`), `staff` help/example, placement cell wording ("2x working pace", "construction 2/3 builders").
- **megaron (web):** build queue shows "next in line" for waiting rows and "Nx pace" for the active one; construct receipt for a waiting build; citygrid shipyard "Nx working pace" and Workshop "Construction ... builders ... Nx building pace / base building pace 1x"; `building_names.js`, `verb_telemetry.js`.
- **codex:** new `workshop.md` (+ index entry, facts block), `shipyard.md` (workers set pace, 1/2/3), `buildings.md` (queue rule + table row), `keryx.md`; `reference.json`/`articles.json` regenerated.

## Views to judge by eye (Timothy, after merge)
City drawer -> Buildings tab (build queue: active row with ready time + "Nx pace", waiting rows "next in line - N game days of work"; queue sentence), City placement grid (Shipyard row; new Workshop row "Construction"), Construct dropdown (Workshop entry), Codex Workshop article. No picture was taken (per instruction).

## Tests per success criterion
| Criterion | Test |
|---|---|
| Yard 1/2/3 workers, W=12 -> 12/6/4 days; 4 on L3 -> still 3x; 0 -> idle | `TestShipyardRateIsPlacedCountCappedAtThree` |
| Crew change mid-job keeps remaining work (no loss/double count) | `TestShipyardCrewChangeMidJobKeepsRemainingWork` |
| Workshop 1/2/3 builders -> 1x/2x/3x on 12 days; none/no workshop -> base 1x; ceil on 10 days at 3x -> 4 | `TestWorkshopBuildersSetBuildPace` |
| Builders arriving/leaving mid-build re-time without loss; stale event cannot finish | `TestWorkshopCrewChangeMidBuildKeepsWork` |
| 4th build refused; 2nd/3rd don't start before 1st done; cancel active starts next; cancel waiting touches nobody; refund as before | `TestBuildQueueRunsOneActiveAndTwoWaiting` |
| In-flight builds keep their deadline across the migration (oldest row same tick; parallel 2nd waits with remaining work) | `TestBuildQueueMigrationKeepsInFlightDeadlines` (runs the migration's own backfill SQL on legacy-shaped rows) |
| Slots 1/2/3, repair berths unchanged 3/6/10 | `TestWorkplaceSlots_ShipyardMatchesTaxonomy`, `TestShipyardBerthsStayAtThreeSixTen`, `TestWorkshopHasOnePlacePerLevel` |
| Four surfaces | keryx `TestWorkshopBuildersAndQueueStateAreTold`, `TestShipyardWaitingSuppressesETA`, `TestAIRound4_QueueExplainsFixedSlots`; web `citygrid.test.mjs` (shipyard pace, workshop); Codex `TestCodexEntityCoverageFromRuntimeCatalogues`, `TestCodexReferenceMatchesAuthoritativeCatalogues` |

## Mutant log (committed first, restored with `git checkout`)
| Mutant | Killed by |
|---|---|
| queue rate hardcoded `rate:=1` (SQL) | `builders_test.go:277` (TestWorkshopBuildersSetBuildPace), `:291` (TestWorkshopCrewChangeMidBuildKeepsWork) |
| shipyard cap 3 -> 1 (`least(1,count(*))`) | `builders_test.go:64` (TestShipyardRateIsPlacedCountCappedAtThree), `:102` (CrewChange) |
| two builds active at once (every queued row started) | `builders_test.go:211` (TestBuildQueueRunsOneActiveAndTwoWaiting) |
| queue cap 3 -> 4 | `builders_test.go:207` (4th build must be refused) |
| ceil -> floor in both sync functions | `builders_test.go:102` (8 left at 3 workers), `:277` (10 days at 3x) |
(An earlier first attempt at "rate=1" for the shipyard that replaced the whole SELECT broke repair saving instead of the rate; it was discarded and redone as the cap mutant above.)

## Known edge cases / deviations from the order
1. **Repair berths kept at 3/6/10** (`economy.ShipyardBerths`): `unit_repair.go` used `WorkplaceSlots("shipyard")` as concurrent-repair limit; changing the worker slots would have silently cut it. Decoupled so repair capacity is unchanged. Say if you want berths to follow 1/2/3 instead.
2. **Parallel legacy builds:** a city with two builds in flight at deploy: the older finishes on its old tick; the younger waits (progress kept, not lost) -- "same tick" holds only for the active one.
3. **In-flight ship jobs** keep saved rate 1 until the next staffing change (trigger disabled while freeing excess placements, so no finish tick moves at deploy). Excess shipyard placements (above the new 1/2/3) are deleted (free citizens), highest ordinal first. This DELETE and the ship half of the migration have no automated test (the backfill for builds has one).
4. A started build finishes `ceil(remaining/rate)` ticks after the tick the rate last changed; the next queued build starts at the tick the completion is processed (up to worker lag later than the due tick).
5. `down` migration restores the schema but not the old `sync_shipyard_work` body.
6. Web queue rendering (`city.js refreshCityBuildings`) has no unit test; checked by reading only.
7. Rows inserted without `work_remaining` (old test fixtures) are shown in a legacy mode (stored deadline, first row active).

## Final run
`tools/gotest.sh` (whole suite incl. web) on a fresh DB at migration 245: **0 red** (Go 935 ok packages, no FAIL; web `# pass 905 # fail 0`; Codex catalogue/reference tests green).

## Diffstat
 server/cmd/keryx/cmd_build.go                      |  11 +-
 server/cmd/keryx/cmd_gubbe.go                      |  11 +-
 server/cmd/keryx/cmd_staff.go                      |   6 +-
 server/cmd/keryx/cmd_status.go                     |  18 +-
 server/cmd/keryx/shipyard_staffing_test.go         |  18 +-
 .../db/migrations/245_builders_workshop.down.sql   |  13 +
 server/db/migrations/245_builders_workshop.up.sql  | 141 ++++++++
 server/internal/combat/build.go                    |  16 +-
 server/internal/combat/shipyard_work.go            |   2 +-
 server/internal/combat/unit_arrival.go             |   2 +-
 server/internal/economy/labor_capacity_test.go     |  18 +-
 server/internal/economy/placement_yield.go         |  17 +-
 server/internal/economy/recompute.go               |  16 +-
 server/internal/economy/shipyard_work.go           |  13 +-
 server/internal/province/building.go               |   7 +-
 server/internal/province/building_names.go         |   2 +-
 server/internal/province/model.go                  |   1 +
 web/static/codex/articles.json                     |   7 +-
 web/static/codex/buildings.md                      |   3 +-
 web/static/codex/index.json                        |  33 +-
 web/static/codex/keryx.md                          |   2 +
 web/static/codex/reference.json                    |  31 +-
 web/static/codex/shipyard.md                       |   8 +-
 web/static/codex/workshop.md                       |  25 ++
 web/static/js/megaron/ui/building_names.js         |   2 +-
 web/static/js/megaron/ui/citygrid.js               |   9 +-
 web/static/js/megaron/ui/citygrid.test.mjs         |  17 +-
 web/static/js/megaron/ui/drawers/city.js           |   9 +-
 web/static/js/megaron/verb_telemetry.js            |   2 +-
 39 files changed, 865 insertions(+), 110 deletions(-)
