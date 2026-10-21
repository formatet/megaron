# Lay up / Fit out — review handoff

Branch: `codex/lay-up`. Owner: Codex. **STOPP before merge, push or deploy; Claude reviews and Timothy's BILD gate applies.**

## Contract and implementation

- `POST /worlds/{worldID}/units/{unitID}/lay-up`: own naval unit in `garrison` at own active Harbour, no cargo or boarded messenger. Atomically moves `crew` into port population and sets `laid_up`. A second call refuses. No ship ration or silver upkeep; the crew now eats as civilians.
- `POST .../fit-out`: same port, `laid_up`, population at least `crew`. Atomically drafts the crew, sets `fitting_out`, and queues `ShipFitOutComplete` for current tick + `combat.FitOutTicks` (3). Completion idempotently restores `garrison`. Normal ship upkeep resumes from the draft; no cancellation. `build_complete_at` shows the ETA.
- Both verbs lock city before ship, recheck ownership/location/status and recompute city production. Disband uses the same city→unit lock order. The scheduled event is written in the same transaction as the status and population change.
- Annex transfers stored/preparing hulls with their port, including a fit-out that finished during occupation. Burning disbands them. Disband of a laid-up hull returns **0** crew because the crew is already in population. Occupied cities cannot disband units.
- Keryx `unit lay-up` / `unit fit-out`; Army cards and status/ETA with server-derived `can_lay_up` and `can_fit_out`; Codex `units`, `sea`, `upkeep`; Umami catalog and both verb lists. Fit out remains visible but disabled when its crew or owned active port is unavailable.

## Evidence

- Named DB tests: `TestLayUpReturnsCrewAndZeroesShipUpkeep_FitOutDraftsAndCompletesAfterThreeTicks`, `TestFitOutRequiresFullCrewInOwnedPort`, `TestLayUpRejectsCargoPassengersAndUnharbouredShip`, `TestDisbandLaidUpHullDoesNotReturnCrewTwice`, `TestAnnexTransfersLaidUpAndFittingOutShipsWithPort`.
- Mutation: removed the `laid_up` zero-upkeep return in `economy.UnitUpkeep`, ran the named round-trip test on fresh migration 195 DB. **RED**: observed `Grain:0.04 Silver:1.5`, expected zero. Restored production code.
- `node --test` all 78 web suites: **695/695 pass**. `go vet ./...`: pass. Firefox fixture with actual War renderer and CSS: [800 px](browser/firefox-800.png), [390 px](browser/firefox-390.png), both pass; fixture source [lay_up_browser.py](../../../tools/lay_up_browser.py). These images show UI states only, not a live-game acceptance claim.
- Full fresh `GOTEST_PORT=57145 tools/gotest.sh` on migration 195, after the final code change: **pass**, including `internal/world` (153 s). The passenger refusal and Disband lock-order cases also passed their targeted fresh-DB runs.

## Review focus

- At occupation the city's `owner_id` stays with the defender until annex. Stored hulls are immobilized by the active-port gate during occupation; annex transfers them with the city. A fit-out already in progress may finish while occupied; annex captures that completed hull too.
- Fitting-out crew is deducted immediately and therefore pays normal upkeep during the three-day preparation. Population's civilian ration correspondingly falls.
