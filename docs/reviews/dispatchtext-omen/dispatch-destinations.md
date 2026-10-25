# Dispatchtyp → dagens knapp och mål

Bas1747b395. Read-only AST-inventering av ALLA produktionsanrop till NotifyPlayer, förenad med format.js historiska lästyper. Call-sites med dynamisk kind redovisas separat; dessa måste följas till sina värden före kod. Dagens generiska destination är **payloadberoende**, inte typgaranterad: q/r → stad via settlement/dest/destination/threatens-settlement-id → laddad enhet. Utan mål visas ändå en avstängd knapp. Inget markerings-/handlingsmål garanteras av den generiska knappen.

| Typ | Dagens knapp → yta | Faktiska payloadfält i literal-anrop | Föreslagen handlingsyta |
| --- | --- | --- | --- |
| ArmyArrival | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| BattleBegun | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| BattleLost | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| BattleWon | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| BuildComplete | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show city → rätt stadsåtgärd; BuildComplete → Staff building i citygrid med vald hex/byggplats |
| CaravanRaided | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show cargo/route/offer → Economy/Diplomacy med rätt post |
| CaravanSeized | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show cargo/route/offer → Economy/Diplomacy med rätt post |
| CityAnnexReady | Take me there → centrera karta; avstängd utan upplöst mål | name, settlement_id | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| CityCollapsed | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| CityOccupied | Take me there → centrera karta; avstängd utan upplöst mål | cause, choices, default, name, occupation_ticks_to_annex, role, settlement_id | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| CivilianStarvationEnded | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show city → rätt stadsåtgärd; BuildComplete → Staff building i citygrid med vald hex/byggplats |
| CivilianStarvationStarted | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show city → rätt stadsåtgärd; BuildComplete → Staff building i citygrid med vald hex/byggplats |
| ColonyFounded | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show city → rätt stadsåtgärd; BuildComplete → Staff building i citygrid med vald hex/byggplats |
| DivineBlessing | Take me there → centrera karta; avstängd utan upplöst mål | amount, name, settlement_id, type | Show cult → Kult för berörd stad/rike |
| DivinePunishment | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show cult → Kult för berörd stad/rike |
| ExpeditionReport | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| ExpeditionTurnedHome | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| FoodShortfall | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show city → rätt stadsåtgärd; BuildComplete → Staff building i citygrid med vald hex/byggplats |
| ForeignMarchSighted | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| ForeignMarchSightedV2 | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| GarrisonStarvationEnded | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show city → rätt stadsåtgärd; BuildComplete → Staff building i citygrid med vald hex/byggplats |
| GarrisonStarvationStarted | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show city → rätt stadsåtgärd; BuildComplete → Staff building i citygrid med vald hex/byggplats |
| GiftDelivered | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show cargo/route/offer → Economy/Diplomacy med rätt post |
| GiftDeliveredV2 | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show cargo/route/offer → Economy/Diplomacy med rätt post |
| GiftDispatchedV2 | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show cargo/route/offer → Economy/Diplomacy med rätt post |
| GiftLost | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show cargo/route/offer → Economy/Diplomacy med rätt post |
| GiftLostV2 | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show cargo/route/offer → Economy/Diplomacy med rätt post |
| GoodsCrafted | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| HexBlockaded | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| HexTaken | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| HexUnblockaded | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| KharisBandChanged | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show cult → Kult för berörd stad/rike |
| KharisDeclining | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show cult → Kult för berörd stad/rike |
| MarchStalled | Take me there → centrera karta; avstängd utan upplöst mål | name, q, r, reason, unit_id | Show unit/ship → War med rätt vald enhet; försvunnen enhet → händelseplats/rapport |
| MessengerArrival | Reply → brev med sender/counterpartWanax; annars generisk karta | variabel/äldre typ; följ källan | Reply → rätt brevtråd |
| MessengerLostAtSea | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| MessengerRescuedAtSea | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| MessengerReturned | Reply → brev med sender/counterpartWanax; annars generisk karta | variabel/äldre typ; följ källan | Reply → rätt brevtråd |
| MetropolisFounded | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show city → rätt stadsåtgärd; BuildComplete → Staff building i citygrid med vald hex/byggplats |
| OccupationDefended | Take me there → centrera karta; avstängd utan upplöst mål | name, occupation_ticks_to_annex, settlement_id | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| OfferAccepted | Reply → brev med sender/counterpartWanax; annars generisk karta | variabel/äldre typ; följ källan | Reply → rätt brevtråd |
| OfferDeclined | Reply → brev med sender/counterpartWanax; annars generisk karta | variabel/äldre typ; följ källan | Reply → rätt brevtråd |
| OfferExpired | Take me there → centrera karta; avstängd utan upplöst mål | counterparty_id, good_key, kind, messenger_id, quantity, resolution, settlement_id, silver | Show cargo/route/offer → Economy/Diplomacy med rätt post |
| OpeningDispatch | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| OrderFailed | Review unit → War-fokus med unit_id; annars generisk karta | messenger_id, reason, unit_id, verb | Show unit/ship → War med rätt vald enhet; försvunnen enhet → händelseplats/rapport |
| OutpostCaptured | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| OutpostEstablished | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| PassageStalled | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| PickupTimedOut | Take me there → centrera karta; avstängd utan upplöst mål | note, q, r, unit_id | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| PickupWaiting | Take me there → centrera karta; avstängd utan upplöst mål | note, pickup_unit_id, q, r, unit_id, waiting_until_tick | Show unit/ship → War med rätt vald enhet; försvunnen enhet → händelseplats/rapport |
| PlaceFirstReached | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| RiteAnswered | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show cult → Kult för berörd stad/rike |
| RoadEdgeBuilt | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| ScoutReport | Take me there → centrera karta; avstängd utan upplöst mål | cedar_deposit, copper_deposit, incense_deposit, q, r, silver_deposit, terrain, tin_deposit, unit_id | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| SentryAlerted | Take me there → centrera karta; avstängd utan upplöst mål | foreign_owner, foreign_type, q, r, sentry_unit_id | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| SettlementBurned | Take me there → centrera karta; avstängd utan upplöst mål | looted, name, role, settlement_id | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| SettlementCaptured | Take me there → centrera karta; avstängd utan upplöst mål | role, settlement_id | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| SettlementDefended | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| SettlementLooted | Take me there → centrera karta; avstängd utan upplöst mål | building_hit, looted, name, pop_lost, role, settlement_id | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| SettlementSacked | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| ShipCameHomeForFood | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| ShipCaptured | Take me there → centrera karta; avstängd utan upplöst mål | former_owner_id, q, r, unit_id | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| ShipDamaged | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show unit/ship → War med rätt vald enhet; försvunnen enhet → händelseplats/rapport |
| ShipFoundered | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| ShipLost | Take me there → centrera karta; avstängd utan upplöst mål | captor_id, q, r, unit_id | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| ShipRepaired | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show unit/ship → War med rätt vald enhet; försvunnen enhet → händelseplats/rapport |
| ShipStormDamaged | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| ShipStranded | Take me there → centrera karta; avstängd utan upplöst mål | q, r, unit_id | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| ShipSweptFromSea | Take me there → centrera karta; avstängd utan upplöst mål | arrives_at, name, q, r, reason, unit_id | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| SiegeLifted | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| SiegeStarted | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| SitosFundLow | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| SitosGranaryRelease | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| SitosIntervention | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| StandingOrderDispatched | Take me there → centrera karta; avstängd utan upplöst mål | arrives_at, goods, leg, standing_order_id | Show cargo/route/offer → Economy/Diplomacy med rätt post |
| StandingOrderPaused | Take me there → centrera karta; avstängd utan upplöst mål | reason, standing_order_id | Show cargo/route/offer → Economy/Diplomacy med rätt post |
| StorageCapacityWarning | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show city → rätt stadsåtgärd; BuildComplete → Staff building i citygrid med vald hex/byggplats |
| StorageOverflow | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show city → rätt stadsåtgärd; BuildComplete → Staff building i citygrid med vald hex/byggplats |
| SubsistenceWarning | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show city → rätt stadsåtgärd; BuildComplete → Staff building i citygrid med vald hex/byggplats |
| TempleDemolitionWrath | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show cult → Kult för berörd stad/rike |
| TradeDelivery | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show cargo/route/offer → Economy/Diplomacy med rätt post |
| TradeLost | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show cargo/route/offer → Economy/Diplomacy med rätt post |
| TradeReturn | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show cargo/route/offer → Economy/Diplomacy med rätt post |
| TrainComplete | Take me there → centrera karta; avstängd utan upplöst mål | count, settlement_id, unit_id, unit_type | Show unit/ship → War med rätt vald enhet; försvunnen enhet → händelseplats/rapport |
| TransferDelivered | Take me there → centrera karta; avstängd utan upplöst mål | dest_id, dest_name, goods | Show cargo/route/offer → Economy/Diplomacy med rätt post |
| TransferDeliveredV2 | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show cargo/route/offer → Economy/Diplomacy med rätt post |
| TransferLostV2 | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show cargo/route/offer → Economy/Diplomacy med rätt post |
| UnitArrived | Take me there → centrera karta; avstängd utan upplöst mål | name, note, q, r, stance, status, type, unit_id | Show unit/ship → War med rätt vald enhet; försvunnen enhet → händelseplats/rapport |
| UnitAttrition | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| UnitDeserted | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| UnitExploreReturned | Take me there → centrera karta; avstängd utan upplöst mål | arrives_at, name, q, r, unit_id | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| UnitFetched | Take me there → centrera karta; avstängd utan upplöst mål | note, q, r, ship_id, unit_id | Show unit/ship → War med rätt vald enhet; försvunnen enhet → händelseplats/rapport |
| UnitLostAtSea | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| UnitRecalled | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show unit/ship → War med rätt vald enhet; försvunnen enhet → händelseplats/rapport |
| UnitRedirected | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Show unit/ship → War med rätt vald enhet; försvunnen enhet → händelseplats/rapport |
| UnitReturnedStarving | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| UpkeepUnpaid | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| agora_ready | Open account → community chat; dessutom generisk avstängd kartknapp | variabel/äldre typ; följ källan | Open account → community chat |
| bountiful_harvest | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| chariot_loss | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| divine_recruits | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| garrison_plague | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| harvest_blessing | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| harvest_failure | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| sea_blessing | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |
| ship_loss | Take me there → centrera karta; avstängd utan upplöst mål | variabel/äldre typ; följ källan | Karta/rapport med vald plats om ett giltigt mål finns; annars ingen målknapp |

## Dynamiska NotifyPlayer-anrop

- server/internal/combat/battle.go:938:14 (kind behöver följas till värden)
- server/internal/combat/collapse.go:328:7 (kind behöver följas till värden)
- server/internal/combat/march_sighting.go:371:6 (kind behöver följas till värden)
- server/internal/combat/road.go:560:7 (kind behöver följas till värden)
- server/internal/combat/sea_storm.go:401:13 (kind behöver följas till värden)
- server/internal/combat/ship_hull.go:245:12 (kind behöver följas till värden)
- server/internal/combat/ship_repair.go:178:13 (kind behöver följas till värden)
- server/internal/combat/shipyard_work.go:100:12 (kind behöver följas till värden)
- server/internal/combat/upkeep.go:529:6 (kind behöver följas till värden)
- server/internal/combat/upkeep.go:581:6 (kind behöver följas till värden)
- server/internal/economy/hex_blockade.go:201:8 (kind behöver följas till värden)
- server/internal/economy/hex_take.go:233:7 (kind behöver följas till värden)
- server/internal/economy/siege_dispatch.go:104:7 (kind behöver följas till värden)
- server/internal/economy/trade.go:592:6 (kind behöver följas till värden)
- server/internal/kharis/tick.go:1279:6 (kind behöver följas till värden)
- server/internal/messenger/handler.go:519:6 (kind behöver följas till värden)
- server/internal/messenger/order_delivery.go:241:10 (kind behöver följas till värden)
- server/internal/messenger/order_delivery.go:276:8 (kind behöver följas till värden)
