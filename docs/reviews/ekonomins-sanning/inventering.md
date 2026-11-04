# Slice 2 — inventering av lagerskrivare + lagringsförslag (ingen kod)

Underlag: grep av `settlement_goods`-skrivare, `TakeFood`, `DeductGood`, `credit_settlement_good`, `settled(` i `server/` på `sonnet/ekonomins-sanning`
(master b8a5c8b5 + rötan borttagen). Radnummer gäller den grenen. "pool" = utan transaktion (ensam sats), "tx" = i anroparens transaktion.

## 1. Mekanismen (viktigt för designen)
- Lagret är lazy: `(amount, rate, calc_tick)`, `settled() = amount + rate × (nu − calc_tick)`. **Produktion bokförs aldrig som en skrivning**; den
  "faller ut" först när en annan skrivare nollställer `calc_tick`. En stad kan alltså gå flera speldagar utan att produktionen syns i någon rad.
- **Krediter har redan en skorsten:** SQL `credit_settlement_good(sid, good, qty, reason, default_cap)` (mig 191) tar en reason-sträng och kör `storage_capped`
  (som redan skriver `StorageOverflow`-event för förlorat överskott, med reason). Reasons idag: `trade_delivery`, `trade_return`, `gift_delivery`,
  `shipment_delivery_v2`, `transfer_delivery`, `legacy_delivery`, `escrow_refund`, `build_refund`, `recruit_refund`, `returned_purse`, `returned_provisions`,
  `founder_stores`, `seized_cargo`, `retired_wine_provisions`.
- **Debiteringar har ingen skorsten:** ~25 handskrivna `UPDATE settlement_goods SET amount = GREATEST(0, settled(...) − $x), calc_tick = current_world_tick()`
  plus `province.DeductGood` (7 anrop) och `economy.TakeFood` (6 anrop). De är kopior av varandra.

## 2. Skrivare (fil:rad · vara · kategori · var körs det · tx)
### Produktion / lagerutrymme
| Skrivare | Vara | Kategori | Körs i | tx |
|---|---|---|---|---|
| `economy/recompute.go:704` upsert per vara (settlar gamla rate, sätter ny) | alla producerbara | production (accrual) + overflow | `RecomputeProduction`: vid varje placering/bygge/tick-recompute (HTTP, tick, handler) | tx |
| `economy/recompute.go:792` bronsens ingredienser (stock drain) | copper, tin | refining_in | RecomputeProduction (ej tickbundet!) | tx |
| `economy/recompute.go:802` brons krediteras (`storage_capped` 'refining') | bronze | refining_out | samma | tx |
| `economy/recompute.go:827` nollar gamla rate | alla | production (accrual) + overflow | samma | tx |
| `combat/silver_audit_v2.go:76` settlar silver (rate>0) | silver | production (accrual) | engångs-audit-event | tx |
### Måltid, hjord, armé, tempel (tickbundet)
| Skrivare | Vara | Kategori | Körs i | tx |
|---|---|---|---|---|
| `economy/food_tick.go:222/226` kalvar födda/matade (`herdBirths`) | livestock +, grain − | herd_births / herd_feed | FoodTick (prio 55, per stad, claimad `processed_tick_claims`) | tx |
| `economy/food_basket.go:176` via `TakeCivilianFood` (food_tick.go:158) | grain/fish/oil.../livestock | meal (befolkning) | FoodTick | tx |
| `economy/food_basket.go:176` via `TakeFood` ← `combat/upkeep.go:361` | grain/fish/… | army_ration (inkl. `support_settlement_id`, fält ×2) | Upkeep-handler (prio 50) | tx |
| `combat/upkeep.go:407` löner | silver | army_wages | Upkeep-handler | **pool (ingen tx)** |
| `kharis/temple_offering.go:290` tionde (summerat över alla tempel i staden) | grain, silver, incense (`templeOfferingGoods`) | temple_tithe (per tempel finns i `planCityOffers`, men skrivs som EN summa per vara) | Kharis-tick | tx |
| `kharis/tick.go:977` reinforcement-påfyllning | per-man-kostnadsvaror | reinforce | Kharis-tick | **pool** |
| `kharis/tick.go:1289` gudastraff: halverar grain | grain | punishment | Kharis-tick | tx |
| `economy/food_basket.go` ← `combat/march_start.go:1085`, `standing_orders.go:483`, `economy/ship_load.go:31`, `ship_hospitality.go:39` | mat | ship_provisions / hospitality | HTTP (marsch) resp. handler (stående order) | tx |
### Spelarhandlingar (HTTP, i anropets tx)
| Skrivare | Vara | Kategori |
|---|---|---|
| `api/handlers/helpers.go:304` `deductGoodsPaid` ← `province.go:1436` (bygge), `province.go:2416` (rekrytering), `unit_repair.go:154` | byggvaror | build_cost / recruit_cost / repair |
| `api/handlers/province.go:1448` `DeductGood` | silver | build_cost_silver |
| `api/handlers/province.go:3534` `DeductGood` (frakt/gåva/handel skickas) | valfri | transfer_out / gift_out / trade_out |
| `api/handlers/messenger.go:533,547` (erbjudande reserverar escrow), `:1424,:1541` (accept, motparten betalar) | valfri + silver | trade_escrow / trade_pay |
| `api/handlers/settlement.go:1014` bön-offer | wine/oil/silver/… | prayer_offering |
| `api/handlers/kingdom.go:1109` tribut (gated, post-MVP) | silver | tribute |
| `combat/road.go:233` vägsten | stone | road |
| `combat/march_start.go:1030` kolonistpung | silver | colonist_purse |
| `combat/standing_orders.go:797` `deductGood` | valfri | standing_order_load |
### Krediter och övrigt
| Skrivare | Kategori |
|---|---|
| `credit_settlement_good` (se §1; 18 anropsställen) | reason finns redan; blir `category` oförändrad |
| `combat/occupation.go:708` byte av plundring (loot) | loot_out (offrets stad) |
| `transport/intercept.go:414` → credit `seized_cargo` | via credit |
| `api/handlers/create_metropolis.go:117,148` / `combat/unit_arrival.go:2045,2086` | founding_seed / carried_purse |
| migrationer 197/198/106 triggers | rör bara `cap` (lagertak/silo-ratchet), inte `amount` |

Inte gjort av en skrivare men måste med: `storage_capped` skriver redan `StorageOverflow`-event (reason `production`/`refining`/ledgerreason) — **overflow
finns alltså redan som mätbar post, men inte som belopp per tick och vara.**

## 3. Lagringsförslag
### Princip: tre skrivningar, en tabell, sanningen ur lagret
`settlement_ledger(world_id, settlement_id, tick, good_key, category, amount NUMERIC, PRIMARY KEY (settlement_id, tick, good_key, category))`, `amount` signerad (+in, −ut).
Skrivs som `INSERT … ON CONFLICT (pk) DO UPDATE SET amount = ledger.amount + EXCLUDED.amount` — flera poster av samma slag samma tick summeras.
Retention: 14 speldagar (`DELETE … tick < now − 14`, en gång per tick i housekeeping-prio 80, **i små delete-batchar** — `megaron_drift.md` mäter 20+ min på en enda stor `DELETE FROM events`; tabellen är liten (≈ städer × ~12 varor × ~10 kategorier × 14 dagar) men batcha ändå).

### Hur posterna skrivs så att ingen kan glömma en (min rekommendation)
**Gör ledgern till en DB-egenskap, inte en kodkonvention.** En `AFTER UPDATE` / `AFTER INSERT`-trigger på `settlement_goods` räknar
`delta = settled(OLD) − NEW.amount_after` respektive accrual `= OLD.rate × (nu − OLD.calc_tick)` och skriver två rader i samma tx som skrivningen:
- accrual → `production` (eller `overflow` med negativt belopp om `storage_capped` klippte; `lost_quantity` finns redan i funktionens kropp),
- resten av förändringen → kategorin som anroparen satt med `set_config('megaron.ledger_reason', 'army_ration', true)` (tx-lokalt) — **utan reason → `unlabelled`**, vilket syns som "Other".
Vinsten: en framtida, okänd skrivare kan inte gömma sig; den hamnar i Other och testet "Other = 0" går rött. Varje känd debiterare får en rad `PERFORM set_config(...)`/en Go-hjälpare `ledger.Reason(ctx, tx, "army_ration")` före sin UPDATE — en mekanisk ändring på de ~25 ställena, plus att `credit_settlement_good` sätter sin egen `p_reason` internt.
Alternativ (sämre): varje skrivare gör egen INSERT i ledgern. Då kan en glömd skrivare ge ett falskt Other = 0 om den också råkar vara noll — och 25 kopior är redan problemet idag.
**Strukturvakt:** ett arkitekturtest (som `g1Allowed`) som greppar produktionskod efter `UPDATE settlement_goods` utanför en vitlista, så ingen ny skrivare smyger in ostämplad.

### Dagsräkningen
- Facit: `Δ = closing − opening` per (stad, vara, tick). Closing = `settled()` vid dagens slut. Opening = föregående closing — **lagras i en snapshot-rad**
  (`settlement_ledger` med `category='closing'`, `amount` = lagret) som skrivs av en ny **dagsavslutare** (housekeeping-prio 80, per stad, claimad enligt CLAUDE.md:
  `INSERT INTO processed_tick_claims … ON CONFLICT DO NOTHING`, scope = hash(world, "ledger_close")).
- Dagsavslutaren **settlar även rader som ingen rört** (UPDATE som sätter `calc_tick = current_world_tick()` för rader med `rate <> 0`), så att accrual hamnar på rätt dag
  och inte klumpas ihop på en senare. Det är den enda nya skrivaren mot `settlement_goods` och går genom samma trigger.
- `Other = Δ − Σ(poster)`. Per konstruktion 0 om alla skrivningar gick genom triggern; `unlabelled`-raderna ÄR Other. Ytan visar Other när `|Other| > 0.05`.
- Idempotens: dagsavslutaren claimas per (event, stad); triggerposter ligger i samma tx som sin skrivning och upprepas därför bara om skrivningen upprepas (och alla tick-handlers är redan claimade). Events förblir audit.
- Ytor läser `GET …/settlements/:id/ledger?tick=N` (ny; en rad per post, "Other" sist). `ticklog` (`api/handlers/province.go:3185-3230`) byter källa.

## 4. Öppna frågor till Claude/Timothy (jag har inte designat om något)
1. **Trigger vs per-skrivare-INSERT.** Jag rekommenderar triggern ovan. Godkänn, eller säg att du vill ha explicit INSERT per ställe. Triggern är en migration ⇒ `systemctl restart poleia`.
2. **Produktion mitt i dagen:** accrual skrivs på den dag skrivningen sker (dagsavslutaren ser till att ingen rad släpar > 1 dag). Räcker det, eller ska en dag som delas av en recompute mitt på dagen delas i två poster? (Jag föreslår: summeras till en `production`-rad per dag.)
3. **Tempeltionde per tempel:** `temple_offering.go:290` skriver EN summa per vara. Order säger "per tempel". Tillåt kategori `temple_tithe:<temple-id-eller-gudnamn>`, eller räcker `temple_tithe` med per-tempel-uppdelning i ytan ur `planCityOffers`? (kräver annars att ledgern får en `ref`-kolumn.)
4. **`upkeep.go:407` och `kharis/tick.go:977` kör på `pool` utan tx** — ledgerposten blir en annan sats än debiten (trigger löser det automatiskt, per-skrivare-INSERT gör det inte). Ännu ett skäl för trigger.
5. **Brons i `RecomputeProduction` är ej tickbundet** — ingrediensdrain sker vid varje recompute, inte per dag. Bokförs som `refining_in/out` på den dag recompute körs; ok?
6. **Gudastraffet halverar grain** (`kharis/tick.go:1289`): kvar som `punishment`; det är det enda "rot"-liknande som finns kvar. Bekräfta att det är avsiktligt.
7. **Retention 14 speldagar** — ok, eller kortare? Tabellstorlek: ~städer × 12 varor × ~10 kategorier × 14.
8. Vad ska ledgern göra med **`StorageOverflow`**: separat kategori `overflow` (negativt belopp) skriven av triggern ur `storage_capped`-skillnaden? Min rekommendation: ja.
