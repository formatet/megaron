# Inventering — böner steg 2b, fart (Sonnet, 2026-11-07, master 7ebb20fe)

Inga kodändringar i detta steg. Slutsats: **ingen tabell saknar ett säkert sätt att köa om ankomsten** — STANNA-villkoret slår inte till.
Men ordern förutsätter två saker som koden inte har; båda löses utan omdesign (se "Avvikelser").

## 1. Tre omräkningsvägar

### units (skepp) — mall `combat.ExecuteRecall`
- `combat/recall_redirect.go:59–290`. Position nu: `LoadActiveRoute` (movement/route.go:43) + `RoutePositionAt` (combat/route.go:62) med `anchor.MilliAt(now)`.
- Rutten är `movement.StoredRoute` (route.go:11): start, offset, End, hexar, `Costs`. Boundaries `b_i = start + (D*P_i + T/2)/T` (movement.go:102) — **hela färden skalar likformigt med D**.
  Därför kan inte "kvarvarande Costs × 0,75" göras genom att bara ändra `EndTick`: det skulle flytta även redan gångna gränser (positionen hoppar).
  Lösning (ny ren funktion `movement.SpeedUpRemaining`): skriv om `Costs` som verkliga milli-längder (summa = ny D), gångna gränser (`b ≤ nu`) oförändrade, framtida gränser skalas om mellan `nu` och ny ankomst. Då är `b'_i = start + P'_i` exakt och position nu är identisk före/efter.
- Ny ankomst köas av `enqueueUnitArrivalTx` (combat/unit_arrival_queue.go:17): nytt `UnitArrivalV2` med `RouteKey = sha256(march_route)`; det gamla ignoreras i `unit_arrival.go:91/137` eftersom nyckeln inte längre stämmer. Samma väg återanvänds — gamla ankomsten gör alltså ingenting.
- `units.depart_tick/arrive_tick/arrives_at/departs_at` uppdateras som recall gör. `stillness_*` rörs inte. `unit_expeditions` (explore) rörs inte: raden har egen `leg_arrive_tick`/TurnTick; skepp på expedition snabbas **inte** upp (rapporteras, inte tyst).
- Bara `Version==2`-rutter med klocka (annars faller LoadActiveRoute/enqueue tillbaka på tick-väg): äldre rutter hoppas över.

### transports — journey-JSON
- `province/trade_journey.go`: `TradeJourney{Clock, StepCosts, TravelTicks, TimeFactorMilli, Thread, Stillness}`. `Validate` (:97) kräver `Clock.EndTick-StartTick==TravelTicks` (:104) **och** att `TravelTicks == round(Σcost·3·faktor/2)` (:131) **och** `Thread/Stillness ⇔ faktor 750`.
  Det sista kan inte stå kvar för en omräknad färd (kostnaderna blir milli-längder). Lösning: nytt fält `speed_blessing` (uuid-sträng); för en märkt färd hoppar Validate över den härledda varaktigheten (resten gäller).
- Position: `transport.SavedPosition` (transport.go:285) går via `j.Route()` → samma movement-kärna, så samma `SpeedUpRemaining` ger oförändrad position.
- Ankomst: **alla** händelser bär `transport_id` i payload (TransportArrival `transport.go:83`, TradeDelivery/GiftDelivery/ShipmentDelivery/TradeReturn via `economy.EnqueueFreightTx` journey_clock.go:28; ShipmentV2/GiftShipment/trade.go-structarna har `json:"transport_id"`).
  Omköning = `UPDATE scheduled_events SET due_tick, process_after WHERE payload->>'transport_id'=$1 AND processed_at IS NULL AND failed_at IS NULL AND event_type IN (...V2/V3 + legacy)`. Plats-på-plats i stället för ny rad+ignorera: precedens finns (unit_arrival_queue.go:48), ingen unik nyckel krockar, och det gamla jobbet kan inte köras (det finns inte). Ordern bad om "gammal händelse gör ingenting" — uppfylls trivialt: det finns aldrig två.
- Transporter utan `journey.Clock` (pre-V2) hoppas över.
- Nyckeln `transports.due_tick/arrives_at` uppdateras. `ship_unit_id`-fartygets `stillness_*` rörs inte.

### messengers — `land_route` (mig 214) + `arrives_at` + `passage_generation`
- `messenger/land_route.go:18` `SaveLandRouteTx` fryser en `StoredRoute` per fri landetapp; `EnqueueLandTx` (:82) köar `MessengerArrivalV2/ReturnV2/OrderDeliveryV2` med `passage_generation` i payload + deadline = rutten.
  Samma `SpeedUpRemaining` på `land_route`; händelsen (payload->>'messenger_id') uppdateras på plats (due_tick, process_after); `passage_generation` oförändrad så generationskontrollen (handler.go:103/order_delivery.go:124) släpper igenom.
- Omfattas: `status IN ('outbound','returning')`, `land_route IS NOT NULL`, `passage_status IS NULL`, `carrier_transport_id/carrier_unit_id IS NULL`.
- **Sjöpassage**: en budbärare ombord (`passage_status='aboard'`/`awaiting_passage`) har ingen egen `land_route` under sjöbenet; dess landningsben startas av bärarens ankomst (`passage.go:1104`, `scheduleCompletion` :1285). Den snabbas alltså **inte** separat — den följer skeppet/karavanen (bärarens snabbare ankomst ger tidigare landning), och landningsbenet får vanlig tid. Lösningen = filtret ovan.
- Offer-bärande budbärares utgångstid (`expires_at`) rörs inte.

## 2. Avfärdskonsumenter (där en aktiv välsignelse ska läsas)
| Konsument | Plats | Plan |
|---|---|---|
| transport (land+sjö) | `transport/departure.go:14 BlessDeparture`, anropas från `prepareJourney` (transport.go ~:215) | efter `ConsumeDeparture`: om ej redan 750 → täckande blessing (stad = `originID`, realm, namngivet skepp via `ShipUnitID`) ⇒ ×0,75 + märke |
| budbärare (löpare) | `messenger/departure_prayer.go:15 BlessRunner`, anrop `api/handlers/messenger.go:458`, `unit.go:464` | efter `ConsumeDeparture`; märke sätts på raden efter INSERT (märke i `land_route`) |
| skepp (units) | `combat/march_start.go:874–890` (`stillness`/`departureFactor`) | om `!stillness` och täckt ⇒ `departureFactor=0,75`, märke i `march_route` |
Märkning: `StoredRoute.Sped` (units, budbärare — ruttens märke följer med benet och nollas naturligt när ett nytt ben byggs), `TradeJourney.SpeedBlessing` (transporter). Budbäraren speeds efter `SaveDepartureTx` (rutten finns då), före ankomstköandet.

## 3. Avvikelser mot ordern (kod vs order)
1. **Likformig skalning** (ovan): "skala kvarvarande StepCosts" duger inte rent; milli-omskrivning + ny `SpeedUpRemaining` behövs, och `Validate` får en undantagsgren för märkta färder.
2. **Namngivna land-mål** är budbärare/karavaner, inte `units`: `prayer_blessings.unit_id` har FK mot units. Mig 247 lägger `mover_id/mover_kind` (ingen FK; budbärare eller transport) och mjukar CHECK.
3. Ankomst köas om **på plats** (se transports) i stället för ny rad + ignorera gammal.
4. En namngiven karavan/löpare kan inte täcka *framtida* avgångar (den finns inte än); avfärdskroken läser därför bara city/realm (+ namngivet skepp).
5. Thread/Passage tas ur katalogen men konstanterna `EffectNext*Departure` och `departure_prayers` ligger kvar (väntande rader hedras).
