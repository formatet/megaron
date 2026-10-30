# Foreign city V1 — proof package (Claude 2026-10-30)

Contract: `~/Dokument/myltavault/megaron_kontrakt_frammande_stad.md`. Inventory: `INVENTORY.md` (committed before code).

## What changed
- **temenos:** `province.Get` answers another ruler's city from a whitelist (`api/handlers/foreign_city.go`):
  unknown → the same 404 as a missing id · seen (live, or S0 memory with date) · contact (snapshot).
  New table `player_contacted_settlements` (mig 230), one owner `economy.RecordContact`, written in the arrival
  transaction by messenger delivery, caravan/gift/transfer arrival, shipment V2, trade delivery and board reading.
  Coarse words only: `province.SizeWord` (hamlet/town/city/great city), `economy.FoodWord`, `economy.GoodsWords`.
  Sibling reads closed: `/goods`, `/buildings` → `[]`, `/ticklog` → 404 for a foreign city; `/army` only in live sight.
  The 422 "that is a settlement ID" hint now only names your own settlements.
- **keryx:** `status --province <foreign>` prints level, date and source (`cmd/keryx/foreign_city.go`).
- **megaron:** the inspect panel shows the same lines above the market board (`ui/foreign_city.js`).
- **Codex:** new article `foreign-cities` (+ link from `contact`; `screen` now says what "Unknown" defence means).

## Mutants (one per contract row) — 21/21 killed
`mutants.py` in this folder; each arm on a fresh DB (`tools/gotest.sh`).

| # | Mutant | Killed by |
|---|---|---|
| 0 | 422 hint names a foreign city | foreign_city_test.go:174 |
| 1 | unknown city answers differently from a missing id | :170 |
| 2 | exact population leaks | :179 |
| 3 | building level seen from outside | :188 |
| 4 | siege shown out of memory | :230 |
| 5 | memory shows today's size | :224 |
| 6 | memory lists buildings | :227 |
| 7 | starving reads hungry | :269 |
| 8 | silver named among goods | :273 |
| 9 | "plenty of" threshold | :273 |
| 10 | older contact overwrites newer | :266 |
| 11 | raw stock stored in the snapshot | :259 |
| 12 | contact owner rewritten | :243 |
| 13 | **visitor sees the live market board** (the order's surviving mutant) | :321 |
| 14 | extra key passes the whitelist | :179 … |
| 15–17 | foreign `/buildings`, `/goods`, `/ticklog` readable | :205, :202, :208 |
| 18 | remembered city's `/army` readable | :214 |
| 19 | deposits on unseen catchment hexes | :309 |
| 20 | messenger contact not written at delivery | contact_delivery_test.go:52 (also proves no contact while outbound) |

## Known limits (said, not hidden)
- **"plenty of" ≥ half the store ceiling.** `settlement_goods.cap` is today's ceiling (same source as the storage
  forecast) and is ~1 000 000 for most goods, so live cities will read "some" until the Storehouse (todo NU 1) makes
  ceilings real. Nothing to change here then: the word reads the same column.
- **Memory without contact shows no buildings** — S0 rows store none; inventing them was not an option.
- **No contact from a bare unit/ship standing in the hex** without a delivery (in practice combat/siege). Sea trade
  arrives as a transport and is covered.
- **No backfill:** contacts before mig 230 have no snapshot from their own day.
- Trade return legs (goods back to the buyer's own city) record nothing — RecordContact skips own cities anyway.

## Gates
- Code: targeted tests + 21 mutants. Full suite on a fresh DB: first run red (owner-body tests read Get unauthenticated; visitor tests expected the old blacklist; Codex "day" wording) — fixed: owner tests read as the owner (`asPlayer`), visitor tests accept the 404. Then `./api/handlers` ok, web 879/0, other packages ok in the first run.
- BILD (after merge, NU 3 sweep): inspect panel of a foreign city — seen live, remembered, contacted.
- TEXT (playtest): "Your messenger was there … town, hungry, plenty of oil." — does a fresh reader understand it?
