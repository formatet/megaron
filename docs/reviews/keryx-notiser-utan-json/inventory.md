# Notification inventory

Source-discovery output: NotifyPlayer calls and direct notification INSERTs, including forwarded kinds and local closures; Codex icons and top-level notifText cases use the same extraction as codex.test.mjs. Committed delivery forwards an archived row, so its INSERT is the producer. Retired kinds remain supported.

78 known kinds; 42 lacked a detail line on fd76e664.

| Kind | Existing detail on base | Producer / archive source |
| --- | --- | --- |
| ArmyArrival | missing | Codex/client icons, Codex/client text |
| BattleLost | yes | server/internal/combat/battle.go#notifyBattleEnded, Codex/client icons, Codex/client text |
| BattleWon | yes | server/internal/combat/battle.go#notifyBattleEnded, Codex/client icons, Codex/client text |
| BuildComplete | missing | server/internal/combat/build.go#Handle, Codex/client icons, Codex/client text |
| CaravanRaided | yes | server/internal/transport/intercept.go#seize, Codex/client text |
| CaravanSeized | yes | server/internal/transport/intercept.go#seize, Codex/client text |
| CityAnnexReady | yes | server/internal/combat/occupation_check.go#Handle, Codex/client text |
| CityCollapsed | missing | server/internal/combat/collapse.go#collapseSettlement, Codex/client text |
| CityOccupied | yes | server/internal/combat/occupation.go#occupySettlement, server/internal/combat/occupation.go#occupySettlement, server/internal/combat/siege_capitulation.go#Handle, server/internal/combat/siege_capitulation.go#Handle, Codex/client text |
| ColonyFounded | yes | server/internal/combat/unit_arrival.go#foundColony, Codex/client icons, Codex/client text |
| DivineBlessing | yes | server/internal/kharis/tick.go#applyDivineBlessing, Codex/client icons, Codex/client text |
| DivinePunishment | yes | server/internal/kharis/tick.go#applyDivinePunishment, Codex/client icons, Codex/client text |
| ExpeditionReport | yes | server/internal/combat/expedition_outcomes.go#expeditionOutcome, Codex/client icons, Codex/client text |
| ExpeditionTurnedHome | yes | server/internal/combat/expedition_outcomes.go#expeditionOutcome, Codex/client icons, Codex/client text |
| FoodShortfall | yes | server/internal/economy/food_tick.go#tickSettlement, Codex/client icons, Codex/client text |
| ForeignMarchSighted | yes | Codex/client icons, Codex/client text |
| ForeignMarchSightedV2 | yes | server/internal/combat/march_sighting.go#notifySighting, Codex/client icons, Codex/client text |
| GiftDelivered | yes | server/internal/economy/gift.go#HandleGift, server/internal/economy/trade.go#deliverTradeNotice, Codex/client icons, Codex/client text |
| GiftLost | yes | server/internal/economy/gift.go#HandleGift, server/internal/economy/trade.go#deliverTradeNotice, Codex/client icons, Codex/client text |
| GoodsCrafted | missing | Codex/client icons, Codex/client text |
| HexBlockaded | missing | server/internal/economy/hex_blockade.go#SyncHexBlockade#dispatch, Codex/client icons, Codex/client text |
| HexTaken | yes | server/internal/economy/hex_take.go#DispatchHexTaken, Codex/client icons, Codex/client text |
| HexUnblockaded | missing | server/internal/economy/hex_blockade.go#SyncHexBlockade#dispatch, Codex/client icons, Codex/client text |
| MarchStalled | missing | server/internal/combat/unit_arrival.go#NotifyDeadLetter, Codex/client text |
| MessengerArrival | yes | server/internal/messenger/handler.go#notifyDelivered, Codex/client icons, Codex/client text |
| MessengerLostAtSea | yes | server/internal/messenger/carrier_fate.go#applyCarrierWitness, Codex/client icons, Codex/client text |
| MessengerRescuedAtSea | yes | server/internal/messenger/carrier_fate.go#persistRescueReport, Codex/client icons, Codex/client text |
| MessengerReturned | yes | server/internal/messenger/handler.go#notifyReturned, Codex/client icons, Codex/client text |
| MetropolisFounded | missing | server/api/handlers/found_metropolis.go#Settle, Codex/client icons, Codex/client text |
| OccupationDefended | yes | server/internal/combat/occupation.go#resetOccupationDefense, Codex/client text |
| OfferAccepted | missing | server/api/handlers/messenger.go#TradeAccept, server/api/handlers/messenger.go#TradeAccept, Codex/client icons, Codex/client text |
| OfferDeclined | missing | server/api/handlers/messenger.go#TradeDecline, Codex/client icons, Codex/client text |
| OfferExpired | missing | server/internal/economy/trade.go#Handle, Codex/client icons, Codex/client text |
| OrderFailed | missing | server/internal/messenger/order_delivery.go#NotifyDeadLetter, server/internal/messenger/order_delivery.go#notifyOrderFailed, server/internal/messenger/order_delivery_failure.go#failSingleRecall, server/internal/messenger/passage.go#sealLostCarrier, Codex/client text |
| OutpostCaptured | missing | Codex/client icons, Codex/client text |
| OutpostEstablished | missing | Codex/client icons, Codex/client text |
| PassageStalled | yes | server/internal/messenger/passage.go#notifyStalledOne, Codex/client icons, Codex/client text |
| PickupTimedOut | missing | server/internal/combat/unit_arrival.go#HandlePickupTimeout, Codex/client icons, Codex/client text |
| PickupWaiting | missing | server/internal/combat/unit_arrival.go#pickupArrived, Codex/client icons, Codex/client text |
| ScoutReport | yes | server/internal/combat/unit_arrival.go#reportScoutFindings, Codex/client icons, Codex/client text |
| SentryAlerted | missing | server/internal/combat/unit_intercept_scan.go#intercept, Codex/client text |
| SettlementBurned | yes | server/internal/combat/occupation.go#executeSack, server/internal/combat/occupation.go#executeSack, Codex/client text |
| SettlementCaptured | missing | server/internal/combat/occupation.go#executeAnnex, server/internal/combat/occupation.go#executeAnnex, Codex/client text |
| SettlementDefended | missing | Codex/client text |
| SettlementLooted | yes | server/internal/combat/occupation.go#executeSack, server/internal/combat/occupation.go#executeSack, Codex/client text |
| SettlementSacked | missing | Codex/client text |
| ShipCaptured | missing | server/internal/combat/naval_seizure.go#Handle, Codex/client icons, Codex/client text |
| ShipDamaged | yes | server/internal/combat/ship_hull.go#notifyShipDamaged, Codex/client icons, Codex/client text |
| ShipFoundered | yes | server/internal/combat/sea_storm.go#sail, Codex/client icons, Codex/client text |
| ShipLost | missing | server/internal/combat/naval_seizure.go#Handle, Codex/client icons, Codex/client text |
| ShipRepaired | yes | server/internal/combat/ship_repair.go#Handle, Codex/client icons, Codex/client text |
| ShipStormDamaged | yes | server/internal/combat/sea_storm.go#sail, Codex/client icons, Codex/client text |
| ShipStranded | missing | server/internal/transport/arrival.go#releaseArrivedShip, Codex/client icons, Codex/client text |
| ShipSweptFromSea | missing | server/internal/combat/unit_arrival.go#dispatchReturnHome, Codex/client text |
| SiegeLifted | yes | server/internal/economy/siege_dispatch.go#SyncSiegeState, Codex/client icons, Codex/client text |
| SiegeStarted | yes | server/internal/economy/siege_dispatch.go#SyncSiegeState, Codex/client icons, Codex/client text |
| SitosFundLow | missing | Codex/client text |
| SitosGranaryRelease | missing | server/internal/economy/sitos_tick.go#tickSettlement, Codex/client text |
| SitosIntervention | missing | Codex/client text |
| StandingOrderDispatched | missing | server/internal/combat/standing_orders.go#notifyDispatch, Codex/client text |
| StandingOrderPaused | missing | server/internal/combat/standing_orders.go#pauseOrder, Codex/client text |
| SubsistenceWarning | missing | server/internal/kharis/tick.go#emitSubsistenceWarning, Codex/client icons, Codex/client text |
| TradeDelivery | missing | server/internal/economy/trade.go#persistTradeNotice, server/internal/economy/trade.go#deliverTradeNotice, Codex/client icons, Codex/client text |
| TradeLost | missing | Codex/client icons, Codex/client text |
| TradeReturn | missing | server/internal/economy/trade.go#persistTradeNotice, server/internal/economy/trade.go#deliverTradeNotice, Codex/client icons, Codex/client text |
| TrainComplete | missing | server/internal/combat/train.go#Handle, Codex/client icons, Codex/client text |
| TransferDelivered | missing | server/internal/transport/arrival.go#Handle, Codex/client text |
| UnitArrived | missing | server/internal/combat/expedition_outcomes.go#expeditionOutcome, server/internal/combat/expedition_outcomes.go#expeditionOutcome, server/internal/combat/unit_arrival.go#arriveGarrison, server/internal/combat/unit_arrival.go#landArrived, server/internal/combat/unit_arrival.go#landArrived, server/internal/combat/unit_arrival.go#notifyLandColonizeFailed, server/internal/combat/unit_arrival.go#passageArrived, server/internal/combat/unit_arrival.go#pickupReturned, server/internal/combat/unit_arrival.go#pickupReturned, server/internal/combat/unit_arrival.go#exploreReturned, server/internal/combat/unit_arrival.go#damagedShipReturned, server/internal/combat/unit_arrival.go#sentryArrived, Codex/client text |
| UnitAttrition | missing | server/internal/combat/upkeep.go#notifyUnitLoss, Codex/client icons, Codex/client text |
| UnitDeserted | missing | server/internal/combat/upkeep.go#notifyUnitLoss, Codex/client icons, Codex/client text |
| UnitExploreReturned | yes | server/internal/combat/unit_arrival.go#dispatchReturnHome, Codex/client text |
| UnitFetched | missing | server/internal/combat/unit_arrival.go#boardPickupUnit, Codex/client icons, Codex/client text |
| UnitLostAtSea | missing | server/internal/combat/upkeep.go#cascadeCargoDisband, Codex/client text |
| UnitRecalled | missing | server/internal/messenger/order_delivery.go#Handle, Codex/client text |
| UnitRedirected | missing | server/internal/messenger/order_delivery.go#Handle, Codex/client text |
| UnitReturnedStarving | yes | server/internal/combat/unit_arrival.go#dispatchReturnHome, Codex/client text |
| UpkeepUnpaid | yes | server/internal/combat/upkeep.go#notifyUpkeepUnpaid, Codex/client icons, Codex/client text |
| agora_ready | yes | server/cmd/server/agora.go#ready, Codex/client icons, Codex/client text |
