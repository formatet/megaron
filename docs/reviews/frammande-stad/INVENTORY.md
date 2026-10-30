# Foreign city V1 — inventory before code (Claude 2026-10-30)

Contract: `~/Dokument/myltavault/megaron_kontrakt_frammande_stad.md`. Order: `.agents/order-frammande-stad-20261030.md`.

## Readers of `GET /worlds/{w}/provinces/{id}` for a FOREIGN city
- web `render/map.js` `openCityPanel` — fetches it only for `settlement.market_board` (your reading).
- keryx `cmd_status.go:608` (`status --province`), `cmd_sightings.go:348`, `cmd_map.go:111` — read the full body.
- keryx `cmd_build.go`, `cmd_build_list.go`, `cmd_gubbe.go`, `cmd_recruit.go`, `cmd_rite.go` — own city only.
- web `drawers/city.js`, `drawers/war.js` — own city only.

## Leaks found (all same root: no FOW gate on province reads)
- `province.Get`: full owner body to any id-holder; only kharis keys stripped (blacklist).
- `province.Get` 422 hint: a foreign **settlement** id passed as province id names the city → reveals existence.
- **Sibling GETs with no owner gate** (not named by the contract, same root, same slice):
  `/army` (exact composition + upkeep — used by the web panel's "Defended" line), `/goods`, `/ticklog`, `/buildings`.
  Owner-gated already: `/actions`, `/trade`, `/placements`, `/placement-options`.
  Decision here: `/goods`, `/ticklog`, `/buildings` → owner only, else the same 404. `/army` → owner, or the city hex
  in live sight (the map already reveals a live garrison total, Timothy 2026-08-03); else 404.

## `player_seen_settlements.population`
- Map (`world.go` markers) already sends only `size_tier` (2 tiers, render), never the number. ✓
- Story payload (`messengers.told_place`) carries it but is read only server-side into the hearer's S0 row. ✓
- Column stays internal. The size WORD comes from one server function (`province.SizeWord`).

## Contact write points (outcome, in the arrival transaction)
- messenger delivered — `internal/messenger/handler.go` (status → delivered).
- transfer/gift caravan delivered to a foreign city — `internal/transport/arrival.go`.
- trade delivery — `internal/economy/trade.go`.
- market board read — `internal/economy/market_board.go` `ReadBoardsAt`.
- Not covered in V1: a bare unit/ship standing in a foreign city hex without a delivery (in practice combat/siege).
  A sea trade arrives as a transport, so ships carrying goods are covered.

## Owner placement (deviation, reasoned)
Contract asks for the snapshot writer in `internal/province`. The food word needs `economy` (FoodNet/GrowthState),
and province may not import economy (G1). So: words that need no economy (`SizeWord`) live in `province`; the
snapshot writer `economy.RecordContact` lives in economy — still ONE owner, reachable from messenger/transport/economy.

## Memory without contact
S0 rows store no buildings. A remembered (not live, no contact) city therefore shows no building list in V1,
rather than inventing one. Live sight shows types; contact shows types + levels.
