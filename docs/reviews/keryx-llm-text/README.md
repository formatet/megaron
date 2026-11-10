# Proof package: keryx-llm-text (2026-11-10)

Keryx-only. No server change, no migration, no game rule change.

## Changed
- `city`: default = summary + Buildings + one `Catchment: N hexes, M worked — ...` line; `--full` = old output (rows via `printCatchmentRows`). "Worked" = hex with any good placed > 0.
- `abandon --settlement`, `occupation order --settlement`: `resolveSettlement` (name or UUID). `cancel-training --unit`: `resolveUnitID`.
- `cmd_unit.go` confirmations: `unitLabel(id)` = display name from a per-run cache filled by `resolveUnitID` / `unit list`, else `shortIDEllipsis` (no `[:8]` slicing left; occupation too). `locationStr` embarked fallback uses it.
- `stormsPointer` const (cmd_storms.go, same wording as the old cmd_recruit line) printed by `passage`, `unit load/unload/pickup`.
- `apiError` -> `refusalHint` appends hints only for `no passable route`, `carries no passengers`, `Choose a ship docked in` (ships listed via `keryx unit list`).

## Tests (cmd_llm_text_test.go, cmd_city_test.go, cancel_training_test.go)
- city default short / --full rows: TestCityCmd_DefaultIsShort, TestCityCmd_FullHasHexRows
- name resolution: TestAbandonResolvesCityName, TestOccupationResolvesCityName, TestCancelTrainingResolvesUnitName
- post --to / both refused: TestUnitPostToCitySendsItsCoordinates
- display name + short id: TestDispatchConfirmationUsesNameAndShortIDNeverPanics
- storms pointer: TestSeaVerbsPointAtStorms; hints: TestApiErrorRefusalHints

## Mutants (all reverted)
- city rows by default -> TestCityCmd_DefaultIsShort red
- abandon resolver bypassed -> TestAbandonResolvesCityName red (posts .../pylos/abandon)
- hint on every error -> TestApiErrorRefusalHints + TestAPIError_Unauthorized_PointsAtKeryxLogin red
- storms line removed from passage -> TestSeaVerbsPointAtStorms (passage) red

## Flags
- `unit patrol --to` NOT added: existing `TestCityTargetsNoPatrolAndReceiptOutage` deliberately asserts patrol has no `--to` (patrol targets a coastal-sea hex, a city is land). `unit post --to` already existed.
- Existing city tests that read hex rows now run via `runCityCmd` with `--full`.
