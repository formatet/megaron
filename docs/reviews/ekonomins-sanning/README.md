# Ekonomins sanning — slice 2 (bokförd dagsräkning) och slice 3 (prognos) — bevispaket

Order `.agents/order-ekonomins-sanning-20261104.md`; svar på inventeringens frågor: Claude 2026-11-04 (se `inventering.md`).
Gren `sonnet/ekonomins-sanning` ovanpå master b9fcf18b. **Migration 238 ⇒ `systemctl restart poleia` (Claude).** `~/go/bin/poleia` är inte ombyggd (görs efter merge).

## Slice 2 — vad som byggdes
- **Mig 238:** `settlement_ledger` (stad, period, vara, kategori, belopp) + `settlement_ledger_close` (stängningslager) + SQL `ledger_post` + trigger
  `settlement_goods_ledger` (BEFORE UPDATE / AFTER INSERT) som i SAMMA transaktion förklarar varje ändring av `amount`:
  `production` = rate × förflutna ticks, `overflow` = det `storage_capped` klippte (tx-lokalt konto), resten under `megaron.ledger_reason`
  (tx-lokal `set_config`), saknas den ⇒ `unlabelled` ⇒ visas som "Other". `credit_settlement_good` stämplar sin egen reason.
- **Period:** en post hör till tick N om stadens stängning för N ännu inte tagits, annars N+1 — då är Δ(period) = stängning − föregående stängning exakt lika med posterna (även för HTTP-skrivningar efter dagsslutet).
- **`internal/ledger`** (G1: noll-beroenden, tillagd i `g1Allowed` + som tillåten kant för economy/province/kharis/combat/transport/messenger): `Reason`, `ExecAs`, `CloseDay` (claimad per event via `processed_tick_claims`, settlar alla producerande rader, sparar stängning, trimmar 14 speldagar), `LastDay` (`unlabelled` räknas som Other), `Label`.
- **`CloseDay`** anropas sist i Kharis-tick efter `observeStorage`.
- **Skrivare märkta** (kategori): måltid `meal`, kalvar `herd_births`/`herd_feed`, armé `army_ration` (upkeep.go) och `army_wages` (upkeep.go, nu egen tx via `ledger.ExecAs`), tionde `temple_tithe:<gud>` (per gud, per vara), reinforce (`ExecAs`), gudastraff `punishment`, skeppsproviant `ship_provisions`/`hospitality`, bygg-/rekryterings-/reparationskostnad, `dispatch_out` (frakt/gåva/handel), `trade_escrow`/`trade_pay`, `tribute`, `prayer_offering`, `road_stone`, `colonist_purse`, `standing_order_load`, `loot_out`, `refining_in/out` (brons), `founding_seed`, `carried_purse`. Krediter via `credit_settlement_good` behåller sina befintliga reasons.
- **Strukturvakt:** `internal/ledger/guard_test.go` — endast vitlistade filer får skriva `settlement_goods`; en ny skrivare utan vitlistning går rött (och utan stämpel hamnar den i Other).
- **Ytor:** `GET …/provinces/:id/ticklog` ger `ledger` (förra stängda speldagen, post + `label`); keryx `ticklog` och webbens City → Daily history visar "post · belopp, en rad per post, Other sist och bara när ≠ ~0" (inte längre dagens rate som såg ut som historik); Codex `city.md`.
- **Tester:** `kharis/ledger_day_test.go` (armé via TakeFood-ranson, tre tempel/tre gudar, hjord via riktiga FoodTick, gåva via `credit_settlement_good`: Other = 0 för varje vara; en okänd debit via direkt SQL syns som Other −7), `combat/upkeep_ledger_test.go` (riktiga UpkeepHandler bokför `army_ration`/`army_wages` exakt som lagret föll), guarden.

### Mutantlogg slice 2 (alla dödade, rad ur `_test.go`)
| Mutant | Utfall |
|---|---|
| släpp tiondets reason (`temple_offering.go`) | `ledger_day_test.go:104` grain Other = −0.75 och `:122` temple_tithe posts = 0 |
| släpp arméns reason (`combat/upkeep.go`) | `upkeep_ledger_test.go:43` army_ration booked 0.0000, store fell 1.3200 |
| räkna Δ ur rate (production) i stället för lager (`LastDay`) | `ledger_day_test.go:104` Other = 3.7750 (+ silver −25) och `:152` Other = 7.025 (väntat −7) |
(En första M1 gav ett byggfel och räknades inte; den kördes om med `Reason(ctx, tx, "")`.)

### Trigger-kostnad (grov, 3 rader/stad, 300 varv, inkl. en roundtrip för tickbyte)
Med trigger 1.05 ms per stad-tick, utan 0.73 ms ⇒ ca +0.1 ms per skriven rad. ~10 städer × ~12 varor där varje rad skrivs varje tick ≈ +13 ms per tick värsta fall; verklig skrivfrekvens är lägre (lazy). Mätt i testrigg, inte live.

### Att veta / flagga (designade inte om)
- **Första stängda dagen** efter deploy saknar föregående stängning: `ledger.known=false` (ytan säger "books not ready") tills staden har två stängningar. Befintliga städer får alltså ingen historik bakåt.
- **Tyst klipp som INTE är overflow:** `food_basket.go` skriver `LEAST(cap, settled) − qty`; lager över taket (kvarlevande överskott, mig 198-ratchet) som klipps vid en måltid hamnar i måltidens post, inte i `overflow`. Synlig men felmärkt; åtgärd = byt till `storage_capped` (ändrar notiser, så inte gjort).
- **Reason är tx-lokal och består tills nästa:** en skrivare som inte stämplar efter en stämplad skrivare i samma tx ärver den stämpeln. Vakten täcker filer, inte varje sats. Skriv en ny reason före varje ny sorts skrivning.
- `ExecAs` lade upkeep-lönerna och reinforce i egna korta transaktioner (de var pool-satser utan tx, ändrar inte semantiken).
- `cmd/keryx/cmd_ticklog.go` `fmtFlows` är nu oanvänd i produktion (behålls för sitt test).

## Slice 3 — vad som byggdes
- **En ägare:** `combat.StorageForecaster.CityForecast` (+ `economy.CityForecast/FoodForecast/Flow`, `economy.FoodRunway/FoodLow/FoodLowDays=24`). Per vara: produktion, armérans, medborgarnas måltid (armén först, som tickordningen), armésold, tionde per gud (via `kharis.ForecastTithe` = samma `planCityOffers` som tick laddar, kopplad i `handlers.NewCityForecaster`). Mat: Σ matvaror + hjordens överskott (`economy.HerdSurplusRate`). `StorageForecast` (lagertaksnotisen) läser nu samma ägare.
- **Ytor:** `GET …/provinces/:id` → `settlement.forecast` (endast ägaren); Warehouse: "net per game day" per vara + "Food for N game days · net x" (eller "not running down · net +x", aldrig "indefinitely") + matens poster; Codex `food.md` (§food-running-low), `index.json` (kind + sektion).
- **`FoodRunningLow`:** `kharis/storage_notices.go observeFood` (efter `observeStorage`), tillstånd i `kharis_notice_state` condition `food_low` (scope = staden). Skickas EN gång när netto < 0 OCH räckvidd < 24 speldagar (strikt `<`); ny först när staden varit bra (netto ≥ 0 eller räckvidd ≥ 24). Ingen debounce utöver det (kan fladdra om räckvidden ligger exakt vid 24). Text: "Food in <city> runs out in about N game days (<väggklocka>): citizens −5.0, army −2.0, temples −1.0, production +3.0 a game day." i webb (`format.js` icon/kategori/text) och keryx inbox (`notification_text.go`).
- **Tester:** `economy/forecast_test.go` (gräns 23.98/24/plenty/netto≥0), `kharis/food_low_test.go` (en gång; tyst vid exakt 24; inte varje tick; ny efter återhämtning; tyst vid positivt netto; prognosen nämner armé, tempel, medborgare och posterna summerar till nettot), `format.test.mjs`, `city_warehouse.test.mjs`.

### Mutantlogg slice 3 (alla dödade)
| Mutant | Utfall |
|---|---|
| `< 24` → `<= 24` | `forecast_test.go:31` FoodLow(stock 120, net -5) = true, want false |
| ta bort hysteresen (nytt scope varje tick) | `food_low_test.go:76` notice repeats every tick, got 3 |
| utelämna armén ur prognosen | `food_low_test.go:127` flows lack army_ration |

## Verifiering
Hela Go-sviten på färsk DB (mig 238, `gotest.sh ./...`): alla paket gröna; `tools/test-web.sh` 891 pass / 0 fail; `tools/day_guard_test.py` OK; `tools/test_verb_parity.py` OK.
Kvar för Claude: restart (mig 238), bygg om `~/go/bin/poleia`, BILD (Warehouse + dagstabellen) i Firefox efter merge. Inget är mergat, pushat eller deployat.
