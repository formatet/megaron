# RÖ1 — inventering före implementation

## Sammanfattning

Bas **b0762ebf**, gren `codex/rorelsetabellen`, worktree `/home/tk/wt/codex-rorelsetabellen`.
**INVENTERING + STOPP för Claudes granskning. Ingen produktionskod ändrad.** Ingen deploy eller runtime-mätning påstås.
RÖ1 är beslutad kanon (Timothy 2026-10-12, `temenos_terrain.md` §Beslut RÖ1).

Tre konkreta fynd:

1. `CourierSeaTicks` har **en exekverbar produktionsläsare**: budets vanliga **flod** i
   `province/pathfind.go:203`. Hav är redan spärrat i courier-grafen; riktiga havspassager följer bäraren.
   Förslag: flodgrenen läser den gemensamma naval-kostnaden för `river` (0,5 ticks), inte en egen budkonstant.
   Behåll river-passability och ford-vadning; en ändring till krav på verklig bärare även på flod är en annan mekanik.
2. **Karavan ×1,5 finns redan**, i `province/trade_journey.go:68` och sparad-data-validatorn på rad 105,
   för **både land och naval**. Ingen ny faktor får läggas ovanpå. Orderns uppgift om att regeln aldrig byggts
   motsägs av aktuell kod och Codex-artiklarna routes/trade/transfers. Förslag: behåll och mutationsbevisa den.
3. A*-golvet för naval läser kusthavet (`pathfind.go:356`). Med RÖ1 blir djuphavet billigast:
   golvet måste följa **0,2**, annars kan A* välja fel väg. Det befintliga kustdetour-provet är byggt för gamla tal.

## Slice-kontrakt

| Fält | Kontrakt |
|---|---|
| Problem | Historiska terrängtal används för nya färder; RÖ1 och land/skepp-vadställe saknas. |
| Spelarsanning | Ny marsch/prognos använder samma RÖ1-kostnad; bud till havs följer sitt skepp. |
| Invariant | Pågående frysta rutter, deras kostnader och ankomstticks bevaras. Stormregler ändras inte. |
| Scope | province kostnadsägare/A*, nedströms prov, serverprognoser och CLI/webb-konsumenter, Codex, balansspakar. |
| Non-scope | Ingen omprissättning/migration, ingen ny flodpassagemekanik, inga nya skepps-/stormfaktorer. |
| Acceptans | RÖ1 per gren; vadställe land 2,5/naval 1; värden ×2/bud land ÷2/karavan ×1,5; frysta färder bevaras; stormandel före/efter redovisas. |
| Stopvillkor | Denna inventering lämnas till Claude före ändring av budets sjö/flodgren; kanonkonflikt/stormtuning kräver besked. |
| Bevisplan | Literaloberoende tabellprov, fysisk mutation per terräng/fordgren/karavanfaktor, före/efter stormsimulering, full färsk Go/JS/vet i implementationens slutpaket. |

**Kedjegrind: bevisar** geografi → brist → brons → elit genom ärlig restid och sjöförbindelser.
Spelaren möter det först i marschprognosen (webb och Keryx) och leveransens ankomstdag.

## Ägaren och direkta läsare

| Läsare vid basen | Nuvarande ansvar | Föreslagen åtgärd |
|---|---|---|
| `province/movement.go:5` `TerrainMoveTicks` | Enda terrängtabellen. Plains/dal/delta 0,75; kust 0,4; djup 0,7; kulle/macchia 1,25; halvöken 2. | RÖ1-tal här. Håll berg spärrade via passability. |
| `province/pathfind.go:200` `moveHoursFor` | Courier flod separat 0,5, övrigt land ÷2; naval saknar egen fordgren. | En naval-fordgren 1; land-ford förblir tabellens 2,5. Courier flod använder naval-kostnadsgrenen; land/ford ÷2. Ingen andra terrängtabell. |
| `province/pathfind.go:75,92,428` `StepHours`, `StepHoursDB`, A* | Samma entering-hex-kostnad för sökning och sparade steg. | Behåll samma ägare för båda. |
| `province/pathfind.go:355` `minPassableCost` | A*-heuristik kusthav/plains/plains÷2. | Naval läser djuphav; land och courier följer nya slätten. Prov med billig djuphavsdetour. |
| `province/trade_journey.go:60,68` | Prissätter steg i milli-ticks; total ×1,5 rundas en gång, minst 1 tick. | RÖ1 går igenom `StepHours`; behåll ×1,5. Notera kustens ⅓ kvantiseras till 333 milli-ticks före totalrundning, befintlig precision. |
| `province/trade_journey.go:90` `Validate` | Läser endast sparade kostnader och verifierar befintlig ×1,5. | Behåll formeln och gamla data giltiga. Ingen terrängläsning här. |
| `combat/unit_arrival.go:1408` | Defensiv returfallback: startterräng × hexavstånd. | Följer tabellen automatiskt; vid naval-ford bör kategorikostnad användas så den inte får landpriset. Fallbackens existens ändras inte i denna slice. |
| `combat/ship_hull.go:333` | Defensiv portreturfallback: kusthav × avstånd. | Följer nya kustvärdet automatiskt; inte en fryst färds omskrivning. |
| `unit/model.go:124` `MarchHoursFactorFor` | Värden ×2, övriga ×1. | Bevara. Kommentarernas gamla tidsenheter rättas bara i berörda kontrakt. |
| `combat/march_start.go:126,172` | NavalSpeedFactor 0,6/1,4 + värd + besättning + last ×1,5. | Bevara multiplikatorer; terrängkostnad kommer från province. |
| `combat/expedition.go:74` `legTravelTicks` | Avrundar rå kostnad × TravelFactor, minst 1. | Bevara; används också av returben. |

## Nedströms läsare (komplett träfflista i cost-readers.txt)

Alla följande får nya kostnader via den gemensamma ägaren; de är inte egna terrängtabeller.

| Grupp | Produktionsställen |
|---|---|
| Ny marsch/prognos/frysta steg | `combat/march_start.go:700,733,776`; recall/redirect `combat/recall_redirect.go:152,164,186`; retur `combat/unit_arrival.go:1383,1533`; skadat skepps portretur `combat/ship_hull.go:317,336,349`. |
| Expeditionens destinationer och budget | `combat/expedition.go:148,216,278`, `expedition_outcomes.go:86`. |
| Bud och mötespunkt | `messenger/recall.go:55,70,179,201`; call-back `messenger/call_back.go:107`; passage `messenger/passage.go:128,141,159,185,193,912,1005`. Passageporternas rangordning använder courier-land-kostnad. Bärarbenet får ingen budfaktor. |
| Hämtning/passageval i API | `handlers/unit.go:668,1048,1052,1056`; `handlers/messenger_passage.go:333,338,384`; `handlers/messenger.go:364`. Flera FindPath-anrop kontrollerar bara nåbarhet men A* väljer ändå kostnadsberoende väg. |
| Varuresans mode/planner | `province/trade_route.go:18,26`; `transport/transport.go:173`; `handlers/province.go:3313,3379,3391`; `handlers/messenger.go:334,1140,1168,1197,1202`; `handlers/settlement.go:569`; gated kingdom `handlers/kingdom.go:1080`. |
| Automations-/erövrings-/returfrakt | `combat/standing_orders.go:327,355,369,373,603,611`; `combat/occupation.go:765`; `transport/intercept.go:468`; `economy/trade.go:567`. Returben planeras vid sitt eget avfärdstillfälle; redan sparat returben används enligt befintlig livscykel. |
| Legacy-positioner/nåbarhet/sikt | `province/interpolate.go:25`; `province/eyes.go:78,168,238`; `combat/march_encounter.go:230`; `combat/march_sighting.go:188`; `handlers/unit.go:324`; `handlers/world.go:1197,1216,1390,1760`; `handlers/foreign_units.go:177`. Dessa omplanerar endast befintliga legacy-grenar utan fryst rutt. Ingen ändring av sådana policies. |
| Frysta rutters rena läsare | `movement` läser kostnadsförhållanden utan terrängtabell; `combat/route.go`, `transport/SavedPosition`, `transport/arrival.go`, `economy/trade_return.go`, API `map_actor_routes.go`, `world.go`, stormscan och place-reach läser sparade rutter/journeys. **Ska inte byta till dagens terrängkostnad.** |

## CourierSeaTicks — varje kvarvarande träff och förslag

`courier-sea-uses.txt` innehåller alla träffar i server/web/tools; inga runtime-användningar utanför province.

| Ställe | Typ | Förslag |
|---|---|---|
| `province/pathfind.go:133` | Konstantdefinition | Ta bort efter Claudes granskning. |
| `province/pathfind.go:203` | Enda produktionsläsaren, vanlig flod | Läs `moveHoursFor(terrain, "naval")` (0,5); ingen courier÷2 på vatten, inga nya havsvägar. |
| `province/pathfind.go:118,128` | Historik/konstantkommentar | Ta bort konstantens dokumentation; säg att hav är spärrat och flod läser skeppets gemensamma terrängkostnad. |
| `province/courier_pathfind_test.go:52` | Förväntad flodkostnad | Literal 0,5 + slättbud 0,5 i nytt RÖ1-kontrakt; testa flod och havsspärr separat. |
| `province/courier_pathfind_test.go:6` | Historikkommentar | Behåll havsspärrens innebörd utan gammalt konstantnamn. |
| `province/pathfind_ford_test.go:104,106` | Assertion/diagnostik jämför med konstant | Nytt literalprov ford-bud 1,25, vanlig flod 0,5; vadstället för fotfolk/naval egna mutationer. |
| `province/pathfind_ford_test.go:97` | Kommentar | Skriv vadning ÷2, inte egen vattenfart. |
| `messenger/recall.go:47,94` | Kommentar, den senare säger felaktigt sea legs | Uppdatera till flod enligt skeppets terrängkostnad; verkligt hav följer bäraren. |
| `messenger/passage.go:4` | Historikkommentar | Beskriv borttagen abstrakt havspassage utan kvarvarande identifierare. |
| `messenger/courier_travel_sea_wall_test.go:27` | Historiskt före-scenario | Spara historisk innebörd i prosa; uppdatera aktuella landkostnader/ETA i provet. |

Separat äldre `messenger/TicksPerHex=0,5`, `MessengerTravelTicks` och `MessengerTravelDuration`
har **inga externa produktionsanrop** (bara helperns eget anrop och tester). De driver inte CourierTravel.
Ingen omtolkning till sjöfart och ingen orelaterad utrensning föreslås.

## De fyra ytorna

- **Temenos:** march-preview och dispatch använder samma `PreviewMarch`/`StartMarch`.
  Pickup väntetid räknas från courier- och arméväg + `TravelFactor`. Varuresor läser `journey.TravelTicks`.
  Colonize-preview är en grundnings-/ekonomiprognos; färdtiden för colonize går via marschmotorn.
- **Keryx:** `cmd_march_preview.go`, `cmd_unit.go`, `output.go`, `cmd_goods.go`, `cmd_messenger.go`
  visar serverns duration/arrival-ticks. Ingen egen terrängtabell hittad. Spara API/CLI före/efter i slutpaketet.
- **Webb:** `ui/march_preview.js`, `ui/marchctx.js`, drawers war/economy/diplomacy använder API-ETA.
  `render/map.js`, `render/routes.js` använder frysta steg och ankomsttid. Ingen egen kostnadstabell hittad.
- **Codex:** `marching.md`, `sea.md`, `runners.md` bär förflyttningsregler; routes/trade/transfers beskriver
  redan karavan ×1,5 och frysta kostnader. Ingen artikel `terrain.md` finns: terrängtabell kan läggas i marching
  och länkas från sea. Ingen ny verbförmåga; verblistan behöver ingen paritetsändring bara för nya tal.
  Vault: `temenos_balans_spakar.md` och RÖ1-status/rapportlänk uppdateras efter bygget.

Se `player-surfaces.txt` för samtliga sökträffar, även tester och orelaterade snabb/långsam-ord.

## Tester med gamla tal

`cost-literals-tests.txt` är bred inventering med fil/rad: både verkliga kostnadsberoenden och kontrollträffar
(t.ex. upkeep/battle-sannolikheter) ingår. Orelaterade monetära/skade-/sannolikhetstal ska lämnas orörda.

Kända verkliga kostnadsberoenden:

- `province/pathfind_test.go`: raka vägar följer ägarfunktionen men kommentarerna har 0,75/0,4.
  **HeuristicAdmissibility_NavalDetourCheaperThanDirect** har literal 1,6 och gammal kust/djup-ordning;
  ersätt med en RÖ1-fixtur där billigare djuphavsdetour slår kort kustväg och fel heuristik ger rött.
- `province/pathfind_ford_test.go`: **RiverFordShipPaysSameCostAsLand** är nu uttryckligen motsatt kanon;
  ersätt med 2,5 land / 1 naval, bevara courier-vadning 1,25 och berg/havsspärr.
- `province/courier_pathfind_test.go`, `messenger/courier_travel_sea_wall_test.go`, `messenger/intercept_test.go`:
  flodkonstant/plains÷2 och land-ETA/mötespunkt behöver granskas mot RÖ1.
- `province/trade_journey_test.go`: step750, ford+plains total5; ny step1000 och total5 (rundning),
  omvänd väg kan få andra avrundningsutfall. Testa även längre slättkorridor för entydig ×1,5.
- `combat/march_route_test.go:560`: ford2,5 + slätt0,75 ger3; RÖ1 ger3,5 avrundat4.
  Övriga verkliga route/expedition/crew-/pickup-/passage-tester kan hårdkoda tickbudget; välj från råinventeringen.
- **Behåll historiska sparade fixturer** i `combat/route_test.go`, `place_reach_test.go`, stormprov,
  `transport/journey_test.go`, `economy/trade_journey_test.go`, API map/fardvag-route-proven med 750/400/2500
  när de provar fryst data. De ska fortsätta fungera och ska inte systematiskt uppdateras till nya tal.
  Samma princip för redan förseglade orders och migration-fixturer.

## Baslinje och kommande stormbevis

Ren numerisk baslinje på **oförändrad** b0762ebf:
`env -u DATABASE_URL -u TICK_SECONDS go test -count=1 ./internal/province ./internal/movement ./internal/unit`
från server/ — **alla tre gröna**, se `baseline-pure.log`. Detta är rena paketprov;
DB-gatade prov är inte en integrationstestclaim. Full färsk DB/Go/JS/vet återstår efter implementation.

Stormsimuleringen återstår; **inga före/efter-siffror påstås i inventeringen**.
Plan: samma seeds, sjökarta, storminitialisering, sparad kurs och avfärdstick i båda armarna;
verklig `StepStorm`, `SeaHexesPerStorm=100`, `StormDamage=2`, aktuella hull och scanens
inträdesregel (`sea_storm.go:295–313`) med samma frysta-ruttklocka. Mät förlisandel för 10/20/40
sjöhex, kust/djup, basgalär och relevanta fraktben. Rapportera population/seeds/avrundning och
osäkerhet; simulering är ingen naturlig spelresa. Plan-docens äldre 1/36-tal och −1-skada är historik,
inte aktiv kod. Ändra inte stormregler om nya farter avviker från tidigare målkurva.

## Grindar och avgränsningar

| Grind | Denna checkpoint |
|---|---|
| Kod | Ingen produktionsdiff; tre rena baslinjepaket gröna. |
| Visuell | Ingen BILD enligt order (tal). |
| Semantisk | Courier-flod och redan befintlig karavanfaktor redovisade för Claude. |
| Användare | API/webb/CLI-scenario återstår i implementation. |
| Drift | Ingen merge, push eller deploy; frysta rutter inte skrivna. |

Nästa handling: Claude granskar flodförslaget och karavanens befintliga faktor, sedan implementation.
