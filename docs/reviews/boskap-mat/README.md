# Boskap som matkälla — proof package (2026-10-31)

Plan: `.agents/plan-boskap-mat-20261031.md`. Beslut Timothy 2026-10-31: ett djur = en matenhet.

## Vad som ändrades

- `livestockFoodValue` 3 → 1 (`internal/economy/recompute.go`, spegel i `cmd/keryx/cmd_status.go`).
- Hjordens överskott i matnettot. Ägare: `economy.HerdSurplusRate(herd, penLevel)` (Go) och
  `economy.HerdSurplusRateSQL()` (SQL-tvilling), båda i `internal/economy/herd_births.go`.
  Formel: hjord >= `HerdCap(pen)` ger `floor(herd / HerdPerCalf) / HerdGrazeBirthEvery * livestockFoodValue`
  per tick; under taket 0. Skäl: vid taket köper grain inga kalvar (`HerdBirths` ger `fed = 0`), bara
  betesfödseln var 20:e tick (`HerdGrazeBirthEvery`) kommer, och de djuren äts först eller går förlorade.
- Läsare: `PopulationGrowthSQL` (tillväxtgrinden, `food_net`), `api/handlers/province.go` (`food_net_per_tick`,
  nytt fält `food.herd.surplus_rate`), `internal/kharis/omen.go` (omenfaktan food_surplus/deficit).
- Variationsandelen räknas i matenheter (`djur * livestockFoodValue`) i `PopulationGrowthSQL`
  (`food_consumed.livestock` lagrar antal djur).
- Ytor: webb `ui/drawers/city_production.js` (rad "production X + herd Y − people eat Z"), keryx
  `food_format.go` (samma term). Codex: `food.md`, `growth.md`, `livestock.md`, `pen.md`
  (+ `codex_facts.py`, `codex_links.mjs`, `codex_facts.py --check` gröna).
- `province.go` `food.herd` är nu `map[string]float64` (surplus_rate är bråk); JSON för heltalen oförändrad.

## Kvar att veta

- Nyfödd/koloniserad stad börjar med hjord 50 = taket, alltså +0,25 i nettot direkt. Founding-prognoserna
  (`world.go`, `found_metropolis.go`, `unit_arrival.go`) räknar ingen hjordterm; de visar före-hjord-nettot.
- Under taket växer hjorden och ger ingen mat; gränsen är skarp (49 → 0, 50 → 0,25).

## Livesimulering (läst read-only, CT 126 poleia, 2026-10-31)

Sex ägda städer, alla med hjord 50 / tak 50 (pen 0). Alla växte redan före ändringen.

| Stad | pop | net före | herd-term | net efter |
|---|---|---|---|---|
| Lykonia | 1095 | 3,383 | +0,250 | 3,633 |
| Kleiopolis | 1095 | 1,740 | +0,250 | 1,990 |
| Hephaistia | 1095 | 1,740 | +0,250 | 1,990 |
| Thapsos | 1015 | 0,575 | +0,250 | 0,825 |
| Scheria | 1005 | 0,525 | +0,250 | 0,775 |
| Gournia | 1085 | 0,475 | +0,250 | 0,725 |

Städer som ändrar `food_net`: 6 av 6. Städer som byter tillväxttillstånd (holding → growing): 0.
Tillväxttakten (`pop * 0,005 * variety`) beror inte på nettots storlek (nettot är bara grinden). Vinsten är
därför att en stad med nettot <= 0 men hjord vid taket nu växer (inga sådana i livevärlden i dag), och att
boskapen nu räknas rätt i variationen (andel i matenheter). Livevärldens sex städer ändrar alltså bara
visat nettot, inte takten.

## Tester

- `internal/economy/herd_surplus_test.go`: `TestHerdSurplusRate`, `TestPopulationGrowthSQL_HerdAtCapRaisesFoodNet`
  (SQL = Go-balans, med och utan hjord, under/vid tak, pen lyfter taket), `TestPopulationGrowthSQL_VarietyShareIsInFoodUnits`
  (värde 1 och värde 3).
- `api/handlers/herd_food_net_test.go`: `TestProvinceGet_HerdAtCapRaisesFoodNet` (yta: 10,5 / 10,5 / 10,75).
- `cmd/keryx/herd_food_line_test.go`, `web/static/js/megaron/ui/drawers/city_production.test.mjs` (herd-termen).
- Värde-3-tester uppdaterade till 1: `equal_food_test.go`, `food_consumption_split_test.go`
  (`..._OneAnimalCoversExactlyOneFood`), `herd_births_test.go` (`TestHerdAnimalIsWorthOneFood`), `herd_pen_test.go`,
  `livestock_fallback_test.go`.

## Mutanter (committade före, alla dödade)

| Mutant | Dödad av |
|---|---|
| herdterm bort ur tillväxt-SQL | `herd_surplus_test.go:99,102` |
| herdterm bort ur province-ytan | `herd_food_net_test.go:67` |
| räkna även under taket (SQL) | `herd_surplus_test.go:99,102` |
| räkna även under taket (Go) | `herd_surplus_test.go:34,99` |
| värde tillbaka till 3 | `equal_food_test.go:33`, `food_consumption_split_test.go:125,177,183,188`, `herd_births_test.go:19`, `herd_pen_test.go:19` |
| variationsandel i djur, inte enheter | `herd_surplus_test.go:132` |
