# Åtta kandidater — inte slutrangordnade

Bas `daf740dd`. Churn avser 60 dygn före basens committertid (exakt intervall i raw/provenance.json), i kandidatens angivna källfiler: distinct git commits samt tillagda/borttagna rader. Filens churn omfattar även annan kod och är inte funktionens churn. Rename behandlas som delete/add. Kopietal är semantiska ställen, aldrig summan av överlappande dupl/SQL-träffar. Call-sites är syntaktiska produktionsanrop till angivna namn; antal konsumentfiler är deduplicerat. Det är varken dynamisk frekvens eller en bevisad call-graph. Fyra ytor anger beteendets räckvidd, inte fyra duplicerade implementationsägare.

| ID | Kandidat/ägarrad | Ankare / kopior | 60d commits; +/− rader | Helper-anrop / konsumentfiler |
|---|---|---|---|---|
| A | Rekrytering: konkret beslut och atomisk domänhandling (4/15/16) | 4; se avgränsning | 41; +1680/−1368 | 4 / 2 |
| B | Debitering på anroparens TX (7) | 5; se avgränsning | 57; +3717/−1988 | 6 / 3 |
| C | Kredit/refund med explicit cap- och spillpolicy (8/9) | 7; se avgränsning | 72; +3654/−2356 | 2 / 2 |
| D | ProductionEvaluation: gemensam inhämtning, skilda vyer (12/13/19) | 6; se avgränsning | 27; +1396/−945 | 36 / 20 |
| E | Hexavstånd: Go-kanon och SQL-adapter (17) | 10; se avgränsning | 25; +1287/−528 | 41 / 22 |
| F | Kontaktmängd: pensionera privat visibleOrigins (6) | 2; se avgränsning | 1; +73/−0 | 3 / 3 |
| G | Passage-/hamnpredikat och portval (10/18) | 8; se avgränsning | 32; +2963/−370 | 13 / 7 |
| H | Sparad rörelse kontra legacy interpolation (10/11) | 7; se avgränsning | 19; +1211/−340 | 22 / 11 |

## A. Rekrytering: konkret beslut och atomisk domänhandling

Kopior: `server/api/handlers/province.go:364` · `server/api/handlers/province.go:2005` · `server/internal/capabilities/province_verbs.go:77` · `server/internal/province/training.go:107`.

3 beslutsställen + katalog (inte fyra identiska kloner).

Konkret Recruit, status, capabilities; web + Keryx läser status och skickar samma request; Codex beskriver gates (4 ytor).

Hög: falsk affordability, fel crew/count, delbetalning/job. Verkliga rättelser 28b930ab och d6436593; ingen slutsats att varje kvarvarande kopia redan gett en bugg.

Möjlig vakt: G1 domänhem + AST-gräns för handler-DB; gemensamt Requirement/Decision-kontrakt med rollback/samtidighet. Referensslice steg 6.

Mätta namn: `canRecruit`, `CanRecruit`, `populationRequirement`, `PopulationRequirement`. Filvis churn och varje call-site finns i candidates.json.

## B. Debitering på anroparens TX

Kopior: `server/api/handlers/helpers.go:156` · `server/internal/combat/standing_orders.go:784` · `server/api/handlers/province.go:3390` · `server/api/handlers/settlement.go:599` · `server/api/handlers/messenger.go:471`.

2 generella helpers + 3 utvalda inline debitställen; hela SQL-inventeringen är större.

6 direkta helper-call-sites; dessutom inline barter/upkeep/offer debits. Build/Recruit/Repair/StandingOrder m.fl.; server + webb/Keryx + Codex ekonomiska villkor (4 ytor).

Hög: partial debit, calc_tick och revalidering. d6436593 visar verklig rekryteringsatomikbugg i en konsument, inte bevis att två helperkroppar i sig orsakat den.

Möjlig vakt: SQL/AST-vakt mot privata settled/debit-fragment i namngivna migrerade konsumenter; kontrakt på TX med multi-good avslag och samtidighet. G1 province-hem.

Mätta namn: `deductGoods`, `deductGood`. Filvis churn och varje call-site finns i candidates.json.

## C. Kredit/refund med explicit cap- och spillpolicy

Kopior: `server/api/handlers/logistics.go:79` · `server/internal/economy/trade.go:285` · `server/internal/economy/trade_return.go:81` · `server/internal/transport/arrival.go:146` · `server/internal/economy/gift.go:115` · `server/api/handlers/province.go:1515` · `server/internal/combat/unit_arrival.go:448`.

4 exakt normaliserade UPSERT-kopior + 3 olika credit/refund-policyer; de sista är INTE samma kontrakt.

7 utvalda mutationsvägar för delivery/return/transfer/gift/build/arrival; egna replay-/notiskontrakt. Alla 4 ytor observerar stock och spill.

Hög konservationsrisk. Gåvans cap/spill/returned-prov finns i docs/reviews/gava, men ingen kvarvarande credit-dubblett har här belagts orsaka en historisk bugg. Spill och refund får inte tyst harmoniseras.

Möjlig vakt: Återanvänd storage-cap-vakten; ny SQL-kopievakt över migrerade UPSERT samt konservations-/replaykontrakt. province-adapter undviker transport→economy.

Mätta namn: `HandleGift`, `cancelQueuedBuild`. Filvis churn och varje call-site finns i candidates.json.

## D. ProductionEvaluation: gemensam inhämtning, skilda vyer

Kopior: `server/internal/economy/placement_yield.go:182` · `server/internal/economy/placement_yield.go:899` · `server/internal/economy/recompute.go:687` · `server/internal/economy/recompute.go:786` · `server/internal/economy/founding_forecast.go:58` · `server/internal/economy/catchment.go:35`.

3 vyfamiljer actual/fullcrew/founding; optionsägarna delas redan. 2 refining/bemanning-loopar dupl t50. CatchmentBasePotential är RTA-test-only..

Placeringsval, faktisk recompute, catchment-preview, founding och byggnadseffekt; samtliga 4 ytor. Antal direkta statiska call-sites nedan avser nuvarande shared helpers, inte antal kopierade formler.

Hög för brist→brons. d4ed8c36 rättade held_workers i options och 6b64cda3/ea53d693 ersatte handskrivna effektpåståenden; dessa visar vyparitetsrisk, inte att en total regelrewrite behövs.

Möjlig vakt: AST-vakt mot pensionerade helpers först efter konsumtionsprov; actual/fullcrew/founding kontrakt på samma data med blockad/FOW/tagning. Hex/refining förblir skilda.

Mätta namn: `LoadHexProductionOptionsAt`, `LoadHexProductionOptions`, `LoadBuildingProductionOptions`, `FullCrewPotential`, `RecomputeProduction`, `FoundingGrainNetPerTick`. Filvis churn och varje call-site finns i candidates.json.

## E. Hexavstånd: Go-kanon och SQL-adapter

Kopior: `server/internal/hexgrid/hexgrid.go:56` · `server/internal/province/hex.go:11` · `server/internal/religion/model.go:83` · `server/internal/world/mapgen.go:3765` · `server/cmd/keryx/cmd_map.go:13` · `server/internal/economy/siege.go:77` · `server/internal/combat/unit_intercept_scan.go:168` · `server/internal/transport/intercept.go:202` · `server/api/handlers/settlement.go:1450` · `server/internal/gossip/gossip.go:79`.

5 Go-formelägare + inline SQL (10 ankare här; fler hex-SQL statements redovisas separat).

Go geometry consumers enligt separata call-site/file-tal; SQL i siege/interception/reveal/gossip/arrival/join. Server + Keryx samt kart-/Codex-kontrakt, räckvidd på 4 ytor men ingen ny spelarmekanik.

Medel per konsument, bred spridning. Ingen verifierad historisk hexavståndsbugg hittades i denna slice; negativa koordinater/range och dubbla representationsägare är risken.

Möjlig vakt: G1-klassificera religion/world→hexgrid innan flytt; SQL hex_distance kontraktsprov mot Go och text-SQL-vakt mot namngivna inline-formler.

Mätta namn: `Distance`, `HexDistance`, `hexDist`. Filvis churn och varje call-site finns i candidates.json.

## F. Kontaktmängd: pensionera privat visibleOrigins

Kopior: `server/internal/capabilities/context.go:191` · `server/internal/province/contacts.go:9`.

2 regler med exakt samma SQL efter kommentar-normalisering; kontrollfixture för mätningen.

3 direkta call-sites: handler world.loadVisibleOrigins, transfer capability och privat contacted-check. Brev/handel/gåva når web+Keryx; Codex beskriver kontakten (4 ytor).

Hög informationsrisk vid drift, liten teknisk slice. Ingen belagd historisk FOW-bugg från just denna kvarvarande kopia. 31224eb7 visar angränsande verklig silverfilterbugg; SELLABLE≠SHIPPABLE ska bevaras, inte användas som argument att slå ihop katalogfilter.

Möjlig vakt: AST-förbud mot privata visibleOrigins efter migration till province; kontakt/live/minne fortsätter vara skilda begrepp, samma FOW-repro och mutation av privat återkopia.

Mätta namn: `VisibleOrigins`, `visibleOrigins`. Filvis churn och varje call-site finns i candidates.json.

## G. Passage-/hamnpredikat och portval

Kopior: `server/api/handlers/messenger_passage.go:399` · `server/api/handlers/messenger_passage.go:412` · `server/internal/province/pathfind.go:157` · `server/api/handlers/messenger_passage.go:485` · `server/internal/messenger/passage.go:501` · `server/internal/combat/ship_hull.go:417` · `server/internal/transport/carrier.go:155` · `server/internal/economy/trade_return.go:195`.

3 olika terrängpredikat (naval match har samma lista); 2 passage eligibility SQL-kopior; 3 nearest-port SQL-läsningar, 2 t100 loopkloner. Små separata ägarslicar, inte ett gemensamt IsPassable för allt..

Passageval/server-preview, A*, reroute/shipyard/return/interception; påverkade utfall via 4 ytor. Direkta helper-call-sites nedan exkluderar inline SQL.

Hög lifecycle-/destinationrisk. f867e596 rättade en verklig limped destination i samma port/returflöde, inte bevis att loopklonen gav felet. river_ford är navigerbart/gångbart men inte dry-land landing.

Möjlig vakt: G1 province-policyprimitiver; predikatmatris plus SQL/AST-vakt över migrerade kopior. Bevara shipyard preferens, portens tillstånd, egen ägare och verklig rutt.

Mätta namn: `isDryLandTerrain`, `isSeaOrRiverTerrain`, `isPassable`, `IsPassable`, `NearestOwnPort`, `nearestOwnShipyardSettlement`. Filvis churn och varje call-site finns i candidates.json.

## H. Sparad rörelse kontra legacy interpolation

Kopior: `server/internal/movement/movement.go:134` · `server/internal/province/interpolate.go:20` · `server/internal/province/eyes.go:415` · `server/internal/transport/intercept.go:503` · `server/internal/transport/transport.go:136` · `server/internal/economy/trade_return.go:130` · `server/internal/transport/arrival.go:96`.

4 legacy positionsvägar jämfört med shared movement-kärna + 2 journey validation-kopior; CurrentPosition är RTA-test-only.

FOW/loadLiveEyes, interception, unit/world-vyer och returfrigörande; server + webbkarta/Keryx + Codex position/reselogik (4 ytor). NULL-journey är explicit legacykontrakt.

Hög men större substrate. 6e90d076 och 3911cbe3 rättade verkliga route/recall-fel. Det bevisar inte att alla legacyfallbacks kan rivas nu; programme steg10 väntar efter playtest.

Möjlig vakt: AST-förbud mot gamla interpolatorer med namngiven krympande legacy-undantagslista; sparad rutt/entering-costs kontrakt. Ingen automatisk deletion av RTA-test-only.

Mätta namn: `InterpolatePosition`, `InterpolateAlongPath`, `CurrentPosition`, `straightLineHexPosition`, `SavedPosition`, `RoutePositionAt`. Filvis churn och varje call-site finns i candidates.json.

Förslag till Claude: A passar den redan beslutade referensslicen; F är en liten tydlig regelflytt med bevisad kopia. B/C bör följa i kontrakterade konsumentsteg. D behöver paritetskontrakt före flytt. E kan vaktas tydligt men har ingen belagd aktuell bugghistoria här. G delas i predikat, passage-read och portval; H följer programmets väntan efter playtest. Detta är underlag, ingen slutlig prioritering eller implementation.
