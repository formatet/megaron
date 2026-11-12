# Food: every screen says the same thing (2026-11-12)

Order `.agents/order-maten-en-sanning-20261112.md`. Measuring first: [inventory.md](inventory.md). Verdict: the server's figures
match the stock; the screens mixed two quantities under one name (B) and the server has two computations of "after army" and a
people-only day count (A). No economy bug, no balance change.

## The rule now
One question, one answer: *Food +N per game day* or *Food −N per game day · lasts D game days*, from `forecast.food` only (the
forecaster: citizens, army, temples, herd). Not running down: no day count. Over 1,000 days: "over 1,000 game days".
Population growth reads another figure and says so: *growth surplus … before army*. A good's own line shows its net.

Code: `web/static/js/megaron/ui/food_line.js` (web), `foodAnswerLine` in `server/cmd/keryx/food_format.go` (keryx). The overview
endpoint carries the same `forecast` as `/provinces/{id}`; the label `herd_surplus` became "Herd" in `ledger.labels`.

## Before / after, same city, same tick
Before (master, tick 6, 4 citizens on grain at the second reading) and after (this branch, tick 10):

| surface | before: idle | before: 4 on grain | after: 4 on grain | after: nobody on grain (falling) |
|---|---|---|---|---|
| City > Production | "food net +2.1" ×2 | "+13.5" ×2 | "Food +12.4"; "growth surplus +13.9 before army" | "Food −4.6 · lasts 272.9 game days"; "growth surplus −3.1" |
| City > Warehouse | "Food for not running down · net +1.1" | "+12.5" | "Food +12.4" | "Food −4.6 · lasts 272.9 game days" |
| Economy table | "+1.1" / "lasts 233.5" | "+12.5" / "237.1" | "Food +12.4" / "not running down" | "−4.6" / "272.9 game days" |
| Economy Grain row | "+7 per game day" | "+18.4" | "+12.2" (net) | "−4.8" (net) |
| keryx status | "food net +2.1" / "Food for 233.5 game days" / "after army +1.1" | +13.5 / 233.5 / +12 | "Food +12.4 (stocks …)" | "Food −4.6 · lasts 272.9 game days" |
| keryx dossier | "Food for 234 game days (net +2.1)" | same | "Food +12.4" | "Food −4.6 · lasts 272.9 game days" |

Every after-surface reads +12.4 / −4.6 and 272.9 at tick 10. Pictures (Firefox, real pages, 1280 and 390 wide):
`after-{production,warehouse,economy}-{1280,390}.png` (rising) and `fall-*` (falling).
Before-pictures are not kept: the before state is in the table, taken from the master build.

## Tests
Red first for each pair (the 15 existing web tests that asserted the old wording failed against the new code until rewritten;
new: `food_line.test.mjs`, economy goods-rate and forecast-column tests, `food_answer_test.go`, ledger label, overview forecast parity).
Web 1019/1019, `day_guard` web/keryx/codex PASS, `forces_guard` PASS, `codex_facts.py --check` current,
Go (fresh DB): ledger, cmd/keryx, api/handlers, economy, combat all ok.

## Left
* `keryx` growth lines use `formatNetPerTick` (whole numbers from 10 up) while the web says one decimal: "growth surplus +14" vs "+13.9".
* A zero net on a single good prints "+0 per game day" in the Warehouse (existing behaviour, unchanged).
* The temple-tithe divergence between the old handler field and the forecaster is code-read only; the old fields stay on the JSON for other readers.
