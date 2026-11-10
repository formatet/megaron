# keryx transfer duplicate guard (2026-11-10)

Inventory: `GET /api/v1/worlds/{w}/trades` (already used by `keryx cargo`) gives, per own caravan: `role`, `kind`,
`origin_name`, `dest_name`, `items[]` (good_key, quantity), `departure_tick`, `arrival_tick`/`arrives_at`. All four required fields exist; no server change.
Departure tick is set for route-v2 transports only; a caravan without it is never matched (guard lets the send through).

Guard: `refuseDuplicateTransfer` in `server/cmd/keryx/cmd_goods.go`. Refuses when a sender-role transfer/gift has the same origin name,
destination name, per-good totals (order-independent) and `departure_tick == current tick`. `--again` skips it. Same error in `--json` mode.
Any failed lookup (tick, names, trades) lets the transfer through. City names are matched case-insensitively; two own cities with the same name are indistinguishable.
Cost: up to four extra GETs per transfer (world tick, destinations, provinces, trades).

Tests: `cmd/keryx/transfer_dubblett_test.go`. Existing transfer test mocks (caravan_time, gift, haiku6, transfer_manifest) were taught to answer the new GETs.
Mutants (each reverted): tick comparison neutralised -> `TestTransferNotIdenticalSends` "earlier" red; qty ignored -> same test "qty" red;
`--again` ignored -> `TestTransferDuplicateAgainFlagSends` red.

## Opus review (2026-11-10)
- Own mutant: destination comparison removed → `transfer_dubblett_test.go:84` red (dest case). Killed.
- Own mutant: `m.Role != "sender"` removed → **survives**. Only observable when a foreign city shares your origin city's name and
  sends the same goods to the same destination this tick — the name-collision case flagged above. Accepted, not tested.
