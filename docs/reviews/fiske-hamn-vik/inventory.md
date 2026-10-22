# ORDER 14 — inventering före kod

Bas: ea97b332, codex/fiske-hamn-vik. Endast dokument och READ ONLY SQL.
Blockerar kedjegrinden: platsval och matbemanning måste visa verkliga fiskplatser.
Live /healthz: ea97b33 / migration198; SQL 2026-10-22 17:01:21 UTC, tick2892.

## Kontrakt och tal

| Vatten | Nu utan/med hamn | Föreslaget utan/med hamn |
|---|---|---|
| coastal_sea | 2/6 | 2/6 |
| coastal_sea med minst fyra landsidor | 2/6 | 3/7 |
| deep_sea | 1/1 | 0/4 |
| river/river_ford | 1/1 | 1/1 |

`economy/hex_rules.go` äger P0, BuildingExtraPlaces=4 och BuildingRatePerLevel=0.7.
Multiplier med hamn L1/L2/L3 är 1.7/2.4/3.1. Live r0: kust1, djuphav1/6,
flod/vad0.5. Djuphav med hamn L1 blir därför 0.283333 per arbetare och tick,
inte kustens1.7; inget beslut om att ändra dessa grundrater finns.

Hamnen är redan city-wide: BuildingSet.City + levelsAt/builtAt i placement_yield.go
applicerar dess nivå på samtliga relevanta hexar i produktiv catchment (radie2,
18 hexar). Djuphavsregeln behöver få building=harbour; ingen längre räckvidd eller
ny hamnbyggplats behövs. Viktigt: RuleBuildingGate härleder terränger ur samma
regel. Att lägga till deep_sea där får INTE ändra befintlig stadens
vatten-granne-grind eller göra hamnen hexbunden.

## Föreslagen vikdefinition — behöver Claude-ok

Endast coastal_sea får bonus. Räkna existerande grannar från hexgrid.Neighbors;
land = terräng som inte är sea/coastal_sea/deep_sea. River och river_ford räknas
som land här (flod genom land), liksom river_valley/delta. Saknad kartgranne
räknas inte som land. Minst4 gäller även5 och6. En ägare i economy, geometri i
hexgrid, inga klientkopior. Hämta grannterränger i en batch för relevanta kusthexar.

FOW: koloniprognosen filtrerar vilka produktiva hexar som visas. Bonus får inte
skicka grannterränger eller en ny karta; frågan om en synlig kusthex får avslöja
sin verkliga kapacitet när omgivande hexar ännu är okända bör avgöras före kod.
Rekommenderar verklig lokal kapacitet enligt befintlig optionsmodell, inga separata
grannuppgifter i API. Detta är ett uttryckligt granskningsval, inte implementerat.

## Sökning och konsumenter

Rå sökning på fisk, terräng och numeriska regelägare finns i consumer-search.txt.

- Ägare: hex_rules.go hexRules/hexGoodPlaces/placesAndMult; ny vikdata måste nå
  både placement_yield.go loadHexOptionsForHexes och recompute.go LoadHexCapacity.
  Det senare anropar hexRules direkt och får annars annan kapacitet.
- building_effect.go HexBuildEffects tar bara terräng/depositioner idag;
  relevant effekttext får inte lova kustens kapacitet på en vik. NeedsBuilding
  känner bara silver: djuphavets nollplatser måste också beskrivas rätt.
- hex_rules.go RuleBuildingGoods/RuleBuildingGate är härledda kataloger.
  placement_yield.go ChooseFoundingFarmHex använder också hexRules men bara grain/farm,
  så ingen fiskändring behövs i dess urval.
- founding_placement.go rankSlotsFromOptions/PlaceStartingWorkforce/FoodGubbarRequired
  använder PlaceCapPerGood och delad yield. founding_forecast.go använder samma options;
  automatic_food.go utesluter redan nollplatser från restfolkets bästa matval.
  RecomputeProduction konsumerar dessa options. Inga nya fiskkonstanter där.
- placement_reconcile.go, placement_lift.go och demolition behöver hålla samma cap
  vid hamnrivning. En djuphavsfiskare utan hamn får inte lämnas i ett nolljobb.
- settlement_placement.go visar options, global beläggning och effekttext och
  validerar Place mot PlaceCapPerGood. province.go Goods/LoadHexCapacity och
  LoadHexProductionOptions måste visa samma tal. world.go grundningsprognos
  konsumerar FoundingFoodForecastAt. Ingen ny endpoint behövs.
- Webb citygrid.js och dess produktions-/hexkort konsumerar serverns cap/rate;
  cmd_gubbe.go formaterar serveroptions. cmd_map.go terrängsymboler/navalpassage
  är inga fiskplatsformler och lämnas. cmd_unit.go grundning visar serverns forecast.
- Codex buildings.md påstår hamnbonus på "its hex" och behöver förtydliga city-wide
  kust+djuphav. citizens.md/food.md får fisk-/hamnregel eller länk till ny fishing-artikel;
  inga fristående fishing/harbour-artiklar finns i basen. Vault temenos_arbete har
  aktuella beslutet längst ned men äldre öppna bonusrader ovanför behöver rättas av ägaren.

## Livepåverkan (ögonblicksbild, ingen reservation inför deploy)

18 placeringar i alla världars data, samtliga i aktiv värld, alla med hamnL1:
Damon7, Lato5, Petras3, Zakros3. Alla ger nu1/6 per tick; efter ändringen
1.7/6. Summan av just dessa placeringar ökar3.0→5.1 mat/tick. Ingen befintlig
placerad djuphavsfiskare tappar produktion eller behöver flyttas i denna snapshot.
Övriga aktiva städer med djuphav i catchment: Phaistos1hex/hamn1,
Phalasarna5hex/hamn1, Plateia1hex/ingenhamn. Plateia har ingen placering där.

automatic-readonly visar för alla12 aktiva städer ett opåverkat matalternativ
med grundrate minst1, jämfört med gammalt djuphav1/6. Även restfolkets
bästa matval kan därför inte förlora på djuphavsgrinden i denna snapshot.
Vikbonusen ändrar antal platser, inte bästa rate; ger ingen restfolksbonus i sig.
22 kusthexar i aktiva världen kvalificerar under föreslagen landdefinition.
Dessa potentialtal är inte en exakt belägrings-/blockadprojektion eller ett löfte
om lagersaldo. SQL och råsvar bifogade. Upprepa kontroll före deploy.

## Genomförande efter ok

Inga liveskrivningar, ingen datamigration antagen. Om ny kontroll visar fiskare
utan hamn: redovisa och besluta om övergång före deploy. Räkna om projektioner
via befintlig säker ägare så nya regler inte väntar på en godtycklig senare tick.
Bevis: namngivet gammalkod-RED för djuphav0/4 och vik3/7, flodoförändring;
HTTP cap/placering, founding/AutomaticFood, kapacitetsaggregat, hamnrivning;
fysisk mutant→RED→exakt restore→GREEN; JS/vet och full fresh Go EN gång.
BILD för ändrad hex-/optionstext. Kontraktets stopp: land/FOW/övergång först ok.
