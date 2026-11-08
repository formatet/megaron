# Proof package: the yard shares its pace; berths = workplaces (migration 255)

Branch `varvet-delad-takt` from master `c789cecd`. Order: `.agents/order-varvet-delad-takt-20261108.md`. Decision: Timothy 2026-11-07 (`megaron_styrande_beslut.md`).

## What changed
- **temenos, mig 255:** `units.shipyard_work_*` is stored in 60ths of a worker-tick (`economy.ShipyardWorkParts`). `shipyard_work_rate = floor(workers*60/ships)` (0..180, CHECK widened). New SQL: `shipyard_workers(city,owner)`, `shipyard_sharing(city,owner)` (ships = naval, forming/repairing, job open, same owner, min 1), `sync_shipyard_city(city)`. `sync_shipyard_work` uses them (settling under the old rate is unchanged; the blessing cut is scale-free). `shipyard_unit_changed` now syncs the whole city (new and old city on a move). The sync functions never write `owner_id/status/settlement_id`, so the trigger cannot recurse. `StartShipyardWork(work)` still takes worker-ticks, multiplies by 60 and syncs the whole city. `LoadShipyardWork` returns `Remaining` in worker-ticks rounded up, plus `Workers` (0..3) and `Sharing` (replaces `Rate`). The conversion block of the migration (`-- BEGIN CONVERT`) settles every job under its old rate, stores x60 and re-shares each city.
- **berths:** `economy.ShipyardBerths(level)` is now `WorkplaceSlots("shipyard", level)` = 1/2/3; the 3/6/10 table is gone. The refusal text is unchanged. Repairs already running above the cap go on (the gate only counts new repairs).
- **Four surfaces:** web city drawer Training row ("building . 3 workers shared by 3 ships", `shipyardShareNote` in citygrid.js); the overview `training_units` JSON carries `shipyard_workers` / `shipyard_sharing`; keryx `status` Training line (`shipyardShareNote`); Codex `shipyard.md` (shared pace, example, berths) and `ship-types.md` (the "3, 6 or 10" sentence), `articles.json` regenerated.
- `unit.go` unit summary read `shipyard_work_remaining` as days (an old rate-1 assumption, already wrong at 2-3 workers). It now converts parts (adds `ShipyardWorkRate` to the unit model). Not named in the order, see Flags.

## Tests per success criterion (`api/handlers/varvet_delad_takt_test.go` unless noted)
| Criterion | Test |
|---|---|
| 1 ship, 1 worker, 12 work -> 12 days; stored 720 at rate 60 | `TestVarvet_SingleShipKeepsItsFinishTick` |
| 3 ships/3 workers 12 days; 2/3 8 days; 1/3 4; 3/1 36; 2/1 24; Workers/Sharing/Remaining read | `TestVarvet_TheYardSharesItsPaceEqually` |
| Repairs share with builds | `TestVarvet_RepairsShareWithBuilds` |
| A finished ship returns its share the same tick, one timer per ship, none left on the finished one | `TestVarvet_FinishedShipReturnsItsShare` |
| A new ship slows the others, done work kept | `TestVarvet_NewShipSlowsTheOthersButKeepsDoneWork` |
| Unit trigger (status change) syncs the whole city | `TestVarvet_UnitTriggerSyncsTheWholeCity` |
| Staffing change reaches every ship | `TestVarvet_FewerWorkersSlowsEveryShip` |
| Migration: same finish tick, remaining x60, done work settled | `TestVarvet_MigrationKeepsFinishTickAndWork` |
| Blessing on a shared ship, once | `TestVarvet_BlessingOnASharedShip` |
| Berths 1/2/3, 422 "shipyard full" on the next | `TestVarvet_RepairBerthsAreWorkplaces`, `TestShipyardBerthsAreTheWorkplaces` (economy) |
| Web note | `citygrid.test.mjs` `shipyardShareNote ...`; keryx `TestShipyardShareNote` |

Existing tests updated for the new storage unit: `shipyard_work_test.go` (ten jobs now 180 parts each, a tenth of a worker), `recruit_ship_test.go` (saved work 3 worker-ticks = 180 parts), `labor_capacity_test.go` (berths 0/1/2/3/0).

## Mutants (values, each reverted, tree clean afterwards)
| Mutant | Red |
|---|---|
| ships counted as 1 (`shipyard_sharing`) | `varvet_delad_takt_test.go:107, 123, 138, 159, 175, 191, 244` |
| `*60/` -> `*1/` in the sync | `:89, 107, 123, 138, 153, 175, 190, 224, 244` |
| `ShipyardWorkParts` 60 -> 1 (Go) | `:89, 107` |
| unit trigger syncs only its own ship | `:138, 179` |
| berths back to 3/6/10 | `labor_capacity_test.go:138`, `varvet_delad_takt_test.go:277` |

## Suite
`tools/gotest.sh` (whole, fresh DB at mig 255): every Go package `ok` (handlers 84 s, combat, economy, kharis, world ...), no `FAIL`, no `_test.go:` line in the log. The web half of that run showed `# pass 949 # fail 1`: `Day guard codex` flagged my first Codex example ("a 12-day job") in `shipyard.md:22`; reworded to "game days", then `tools/test-web.sh` re-run whole: `# pass 950 # fail 0`. `codex.test.mjs`, `day_guard.test.mjs`, `python3 tools/codex_facts.py --check`, `tools/day_guard_test.py`, `tools/test_verb_parity.py` green.

## Flags
- Counting ships: the order's list did not say owner; I count only ships of the same owner as the city's staffing (a foreign ship in the city gets rate 0 and must not steal a share).
- Unit summary (`unit.go`), the force card and keryx `unit` list show "building/repairing" without the share note (only the city Training row and keryx `status` do). The `training_units` JSON plumbing in `province.go` has no handler test of its own (the values come from `LoadShipyardWork`, which is tested).
- No trigger on `DELETE FROM units`; nothing in the code deletes ships mid-job today. A deleted job would leave the others at their old share until their next sync.
- The migration's convert block is not idempotent on rows already in parts (production holds only old rows; the test clears other rows first). The `down` re-derives rates through mig 251's sync and rounds work up.
- Not run: anything against a real database, deploy, push, merge. `~/go/bin/poleia` not rebuilt.
