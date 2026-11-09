# Proof: the yard's sharing on the unit surfaces (2026-11-09)

Order: `.agents/order-varvet-delning-enhetsytan-20261109.md`. No rule change, no migration.

## What changed
- `server/api/handlers/unit.go`: `unitSummary.ShipyardWorkers/ShipyardSharing` (`omitempty`), filled by `attachShipyardSharing` (one query per list, keyed by city, only for units with `ShipyardWorkRemaining != nil`).
- `web/static/js/megaron/ui/force_card.js`: `building —` and `being repaired —` lines append ` · N workers shared by M ships` (via `shipyardShareNote`).
- `server/cmd/keryx/cmd_unit.go`: `unitRow` fields; naval `forming` and `repairing` lines append `shipyardShareNote`.
- Codex: `shipyard.md` does not name where the player sees the sharing, so unchanged (no articles.json regen).

## Tests per criterion
- Handler: `TestAttachShipyardSharing` (2 ships, 3 workers -> 3/2; lone ship -> 3/1; garrison -> omitted).
- Force card: `force_card.test.mjs` last test (note when sharing 2; none at 1; both lines).
- Keryx: `TestFormatSizeShowsYardSharing`.

## Mutants (each reverted)
- Fields never filled -> `unit_shipyard_share_test.go:70` red.
- Note dropped in force_card -> `force_card.test.mjs:35` red.
- Note dropped in keryx -> `unit_shipyard_share_test.go` (keryx) :13 red.

## Flags
- `shipyard_waiting` lines stay unchanged (workers are 0 there anyway).
