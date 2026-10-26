# Dispatch kinds and actionable destinations

Current snapshot: 74 NotifyPlayer call sites (18 dynamic), 59 current kinds after the explicit traces below; 36 reader-only archived kinds. `dispatch-kind-index.json` holds the measured sets. Reader extraction accepts only the outer four-space `case` arms in notifText; nested divine outcomes such as `ship_loss` and `sea_blessing` are not notification kinds.

Before this slice, every non-letter kind offered generic **Take me there**, centring a map hex or leaving a disabled button when no target resolved. The new action is payload- and current-entity-dependent. No row below promises that an archived/deleted target still exists.

| Kind | Current / archive | Actual action when the stated target exists |
| --- | --- | --- |
| ArmyArrival | archive reader | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| BattleBegun | archive reader | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| BattleLost | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| BattleWon | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| BuildComplete | current | Staff named workplace → exact owning City Production + selected hex/building; non-workplace → Show named city |
| CaravanRaided | current | Show cargo in transit → actual live cargo row; delivered/missing cargo → named own city goods; foreign known site → map panel |
| CaravanSeized | current | Show cargo in transit → actual live cargo row; delivered/missing cargo → named own city goods; foreign known site → map panel |
| CityAnnexReady | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| CityCollapsed | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| CityOccupied | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| CivilianStarvationEnded | archive reader | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| CivilianStarvationStarted | archive reader | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| ColonyFounded | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| DivineBlessing | current | Open cult in named city → exact city for read, offering and rite; no city/site → no CTA |
| DivinePunishment | current | Open cult in named city → exact city for read, offering and rite; no city/site → no CTA |
| ExpeditionReport | archive reader | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| ExpeditionTurnedHome | archive reader | Show ship / Review unit → actual live War card; dead unit → named existing city/site or no CTA |
| FoodShortfall | archive reader | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| ForeignMarchSighted | archive reader | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| ForeignMarchSightedV2 | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| GarrisonStarvationEnded | archive reader | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| GarrisonStarvationStarted | archive reader | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| GiftDelivered | archive reader | Show cargo in transit → actual live cargo row; delivered/missing cargo → named own city goods; foreign known site → map panel |
| GiftDispatchedV2 | archive reader | Show cargo in transit → actual live cargo row; delivered/missing cargo → named own city goods; foreign known site → map panel |
| GiftLost | archive reader | Show cargo in transit → actual live cargo row; delivered/missing cargo → named own city goods; foreign known site → map panel |
| GoodsCrafted | archive reader | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| HexBlockaded | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| HexTaken | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| HexUnblockaded | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| KharisBandChanged | archive reader | Open cult in named city → exact city for read, offering and rite; no city/site → no CTA |
| KharisDeclining | archive reader | Open cult in named city → exact city for read, offering and rite; no city/site → no CTA |
| MarchStalled | current | Show ship / Review unit → actual live War card; dead unit → named existing city/site or no CTA |
| MessengerArrival | current | Reply / Read correspondence → exact counterpart thread when person id exists; otherwise known city/site |
| MessengerLostAtSea | archive reader | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| MessengerRescuedAtSea | archive reader | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| MessengerReturned | current | Reply / Read correspondence → exact counterpart thread when person id exists; otherwise known city/site |
| MetropolisFounded | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| OccupationDefended | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| OfferAccepted | current | Reply / Read correspondence → exact counterpart thread when person id exists; otherwise known city/site |
| OfferDeclined | current | Reply / Read correspondence → exact counterpart thread when person id exists; otherwise known city/site |
| OfferExpired | current | Reply / Read correspondence → exact counterpart thread when person id exists; otherwise known city/site |
| OpeningDispatch | archive reader | Show ship / Review unit → actual live War card; dead unit → named existing city/site or no CTA |
| OrderFailed | current | Show ship / Review unit → actual live War card; dead unit → named existing city/site or no CTA |
| OutpostCaptured | archive reader | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| OutpostEstablished | archive reader | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| PassageStalled | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| PickupTimedOut | current | Show ship / Review unit → actual live War card; dead unit → named existing city/site or no CTA |
| PickupWaiting | current | Show ship / Review unit → actual live War card; dead unit → named existing city/site or no CTA |
| PlaceFirstReached | archive reader | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| RiteAnswered | current | Open cult in named city → exact city for read, offering and rite; no city/site → no CTA |
| RoadEdgeBuilt | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| ScoutReport | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| SentryAlerted | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| SettlementBurned | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| SettlementCaptured | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| SettlementDefended | archive reader | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| SettlementLooted | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| SettlementSacked | archive reader | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| ShipCameHomeForFood | archive reader | Show ship / Review unit → actual live War card; dead unit → named existing city/site or no CTA |
| ShipCaptured | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| ShipDamaged | current | Show ship / Review unit → actual live War card; dead unit → named existing city/site or no CTA |
| ShipFoundered | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| ShipLost | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| ShipRepaired | current | Show ship / Review unit → actual live War card; dead unit → named existing city/site or no CTA |
| ShipStormDamaged | current | Show ship / Review unit → actual live War card; dead unit → named existing city/site or no CTA |
| ShipStranded | current | Show ship / Review unit → actual live War card; dead unit → named existing city/site or no CTA |
| ShipSweptFromSea | current | Show ship / Review unit → actual live War card; dead unit → named existing city/site or no CTA |
| SiegeLifted | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| SiegeStarted | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| SitosFundLow | archive reader | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| SitosGranaryRelease | archive reader | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| SitosIntervention | archive reader | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| StandingOrderDispatched | current | Manage route → existing route row in Economy Automation; deleted route → no CTA |
| StandingOrderPaused | current | Manage route → existing route row in Economy Automation; deleted route → no CTA |
| StorageCapacityWarning | archive reader | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| StorageOverflow | archive reader | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| SubsistenceWarning | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| TempleDemolitionWrath | archive reader | Open cult in named city → exact city for read, offering and rite; no city/site → no CTA |
| TradeDelivery | current | Show cargo in transit → actual live cargo row; delivered/missing cargo → named own city goods; foreign known site → map panel |
| TradeLost | archive reader | Show cargo in transit → actual live cargo row; delivered/missing cargo → named own city goods; foreign known site → map panel |
| TradeReturn | archive reader | Show cargo in transit → actual live cargo row; delivered/missing cargo → named own city goods; foreign known site → map panel |
| TrainComplete | current | Show ship / Review unit → actual live War card; dead unit → named existing city/site or no CTA |
| TransferDelivered | current | Show cargo in transit → actual live cargo row; delivered/missing cargo → named own city goods; foreign known site → map panel |
| TransferDeliveredV2 | archive reader | Show cargo in transit → actual live cargo row; delivered/missing cargo → named own city goods; foreign known site → map panel |
| UnitArrived | current | Show ship / Review unit → actual live War card; dead unit → named existing city/site or no CTA |
| UnitAttrition | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| UnitDeserted | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| UnitExploreReturned | current | Show ship / Review unit → actual live War card; dead unit → named existing city/site or no CTA |
| UnitFetched | current | Show ship / Review unit → actual live War card; dead unit → named existing city/site or no CTA |
| UnitLostAtSea | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| UnitRecalled | current | Show ship / Review unit → actual live War card; dead unit → named existing city/site or no CTA |
| UnitRedirected | current | Show ship / Review unit → actual live War card; dead unit → named existing city/site or no CTA |
| UnitReturnedStarving | archive reader | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| UpkeepUnpaid | current | Show named own city → City Production; otherwise Show known location → selected map hex + real action panel; absent target → no CTA |
| agora_ready | archive reader | Open community chat → Account |

## Dynamic arguments traced in this snapshot

- `server/internal/combat/battle.go` → `BattleWon`, `BattleLost`.
- `server/internal/combat/collapse.go` → `CityCollapsed`.
- `server/internal/combat/march_sighting.go` → `ForeignMarchSightedV2`.
- `server/internal/combat/road.go` → `RoadEdgeBuilt`.
- `server/internal/combat/sea_storm.go` → `ShipStormDamaged`, `ShipFoundered`.
- `server/internal/combat/ship_hull.go` → `ShipDamaged`.
- `server/internal/combat/ship_repair.go` → `ShipRepaired`.
- `server/internal/combat/shipyard_work.go` → `TrainComplete`, `ShipRepaired`.
- `server/internal/combat/upkeep.go` → `UnitAttrition`, `UnitDeserted`, `UpkeepUnpaid`.
- `server/internal/economy/hex_blockade.go` → `HexBlockaded`, `HexUnblockaded`.
- `server/internal/economy/hex_take.go` → `HexTaken`.
- `server/internal/economy/siege_dispatch.go` → `SiegeStarted`, `SiegeLifted`.
- `server/internal/economy/trade.go` → `TradeDelivery`.
- `server/internal/kharis/tick.go` → `SubsistenceWarning`.
- `server/internal/messenger/handler.go` → `OfferAccepted`, `OfferDeclined`.
- `server/internal/messenger/order_delivery.go` → `UnitRecalled`, `UnitRedirected`.

The economy trade notification wrapper currently has one producer call with `TradeDelivery`; the upkeep helper is called with `UnitAttrition` / `UnitDeserted`, and its unpaid helper uses `UpkeepUnpaid`. Hex blockade has the two explicit blocked/unblocked calls. The source positions in `notify-call-sites.json` identify the complete AST call-site snapshot, not a claim to infer arbitrary future dynamic strings.

## Evidence and limits

`dispatch_actions.test.mjs` checks targetless omission and explicit incident sites for each current literal/outer reader kind; separate realistic fixtures check staff city/site/god, live and dead unit, exact counterpart, live and deleted route, selected cult city, delivered goods and live cargo. This is deliberately not described as a full end-to-end scenario for every kind. Native Firefox and actual APIs prove the BuildComplete → exact colony placement, TrainComplete → current unit card, and maintained-temple omen/feed paths.

Omen is a marked gossip/feed record (`category=omen`, `omen.true=true`) produced with an `OmenObservedV1` audit event; it is not a new `NotifyPlayer("Omen")` kind. Old atmospheric gossip without fact_data is omitted by the server.

Named-context fixes in this slice: BuildComplete and both TrainComplete producers (legacy timer + shipyard worker); queue rows carry city/name/hex. Other dispatches keep their existing named payloads and gain action-aware navigation rather than invented city names.
