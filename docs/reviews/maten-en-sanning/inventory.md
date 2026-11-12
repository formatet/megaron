# Food: every screen says the same thing — Step 1 inventory (2026-11-12)

Measured on the isolated acceptance world (`tools/acceptance.sh`, localhost:8097, master 6a80f279), one city
(Itanos, 10 citizens, 2 spearman units, tick length 120 s), Firefox on the real pages, keryx, the API JSON and the DB.
All readings of a table row are inside one game day (tick noted); `tools/maten_capture.py` reads the three web screens.

## Ground truth: what actually happens to the stock
Grain stock read from `settlement_goods` right after each tick (calc_tick = tick), with the city unchanged except for
its own growth (+5 people a game day):

| tick | 0 | 3 | 4 | 5 | 6 | 7 |
|---|---|---|---|---|---|---|
| grain | 1200 | 1200.85 | 1201.37 | 1202.00 | 1202.75 | 1214.95 (after 4 citizens placed on grain at tick 6) |

Per-tick change at tick 4 = +0.52 against the forecast's grain net at that moment, 0.767 (all-food net) − 0.25 (herd) = 0.52.
Falling state: tick 11 1245.92 → tick 14 1231.42 = −4.83 a game day, forecast grain net −4.83. **The server's own figures agree with the
stock. No economy bug.** The one term that cannot show in a 5-day window is the herd overflow (+0.25 a game day): it
is five calves arriving every 20 game days (`herd.next_birth_tick`), eaten at the pen cap when born, so a daily average
of +0.25 is correct over the cycle but invisible between two readings 5 days apart.

## The table (tick 6, before placing; then tick 6/7 after placing 4 citizens on grain)
| surface | exact text | JSON field | computed by | before | after |
|---|---|---|---|---|---|
| City > Production | "Growing: food net +2.1 per game day" | `food.food_net_per_tick` | `economy.FoodNet` via `settlementFoodSummary` (growth gate) | +2.1 | +13.5 |
| City > Production | "Food net  +2.1 per game day" | same | same | +2.1 | +13.5 |
| keryx `status` | "growing: food net +2.1 per game day" | same | same | +2.1 | +13.5 |
| keryx `status` | "Food production 7.0 + herd 0.25 − people eat 5.2 = net +2.1 (before army)" | `food.*` | same | +2.1 | +13 |
| keryx `status` | "Army eats 1 food → food net +1.1 after army" | `food_net_per_tick_after_upkeep` | `foodNetAfterArmy(growth net, army grain)` in the handler | +1.1 | +12.5 |
| City > Warehouse | "Food for not running down · net +1.1 a game day" | `forecast.food.net` / `.days` | `combat.CityForecaster` (the same owner as the FoodRunning notice) | +1.1 | +12.5 |
| City > Warehouse | rows "Army rations −1, herd_surplus +0.3, Citizens' meals −5.2, Production +7" | `forecast.food.flows` | forecaster; label from `ledger.labels` (no entry for `herd_surplus`) | | |
| City > Warehouse | "Grain 1,202 / 2,000 · net +0.9 a game day" | `forecast.goods[grain].net` | forecaster (grain only, herd not included) | +0.9 | +12.2 |
| Economy | "Food after army +1.1 per game day" | `/settlements/overview` `food_net_per_tick_after_upkeep` | handler subtraction (no temple tithe) | +1.1 | +12.5 |
| Economy | "Food lasts 233.5 game days" | overview `food.coverage_ticks` | `settlementFoodSummary`: stock ÷ people's meal only | 233.5 | 237.1 |
| Economy | Grain row "+7 per game day" at stock 1,202 | `/goods` `rate_per_tick` | `settlement_goods.rate`, gross production | +7.0 | +18.4 |
| keryx `status` | "Food for 233.5 game days at current consumption" | `food.coverage_ticks` | same as the Economy figure | 233.5 | 233.5 |
| keryx `dossier` | "Food for 234 game days at current consumption (net +2.1 per game day)" | both | people-only days beside the growth net | | |

## Verdict per pair
* **Growth +2.1 vs Warehouse/Economy +1.1**: (B). Different quantities: the growth gate (herd overflow added, army NOT
  subtracted) against the net after everything. Both labelled "food net". A newcomer cannot tell them apart.
* **Economy "after army +1.1" vs Warehouse net +1.1**: (A) in structure, equal in every state I measured. Two server
  computations of "the net after the army": the handler's `growth net − army grain`, and the forecaster. They diverge when a
  temple's tithe takes a food good (the forecaster subtracts tithe, the handler does not; read in `ForecastTithe`, not
  produced live). One owner must win: the forecaster, the one the tick's own FoodRunning notice uses.
* **"Food lasts 233.5" vs the forecast's days**: (A). `coverage_ticks` is stock ÷ the people's meal only; it ignores army and
  production. It said "240 game days" for a city whose net is positive. The forecaster's `days` (stock ÷ loss, absent when not falling) is the answer.
* **"8,000 game days" in the Warehouse**: not a bug in the arithmetic. The forecaster's days is stock ÷ loss, and a net of −0.15
  on a stock of 1,200 is exactly 8,000. It was a true number nobody can use: now capped ("over 1,000 game days").
* **Grain row "+6.4 per game day" at a constant 1,200**: (B). It is gross production (`settlement_goods.rate`; D1: meals and the army are
  debited from the stock once a day, not folded into the rate). Not a store cap (cap 2,000). The stock moves by the forecast's grain net (+0.2 there).
  The row now shows that net.
* **Warehouse grain net +0.9 vs food +1.1**: (B), explained by the herd overflow (+0.25) which the food line has and a grain row cannot.
  The flow row was printed as the raw key `herd_surplus`.
* **Stale client values (C)**: none found. Drawers re-fetch on open. The tester's differing numbers are explained by (A)/(B)
  plus moving workers between readings. (The Economy page in my "post" run was loaded one tick later: stock 1,221 vs 1,202.)
