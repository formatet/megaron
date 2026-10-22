# ORDER 12 — inventering före kod

2026-10-22. Gren `codex/roddare`, bas `5262aa6e`, worktree
`/home/tk/wt/codex-roddare`. **Ingen produktionskod ändrad. Väntar på Claudes ok.**
Restkontrollen lämnades först separat med STOPP, `0c8ef8f3`.

## Kontrakt och föreslagen regel

Problem: skeppens mat är ett skrovstal medan roddarna enligt Timothys beslut
ska äta som soldater. Spelarsanning: faktisk besättning äter soldatranson;
avfärdens lastning, den dagliga dragningen och matmätaren beskriver samma behov.
Invariant: en ransonsägare, ingen dubbeldragning, silver oförändrat; laid_up
fortsatt ingen skeppsmat/sold. Stöder kedjegrindens fysiska logistik.

**Föreslagen formel:** `UpkeepSpecs["spearman"].Grain / 100 × faktisk Crew`,
och `× FieldGrainFactor` när ute. Nya/rekyteringsprognosticerade fulla skepp
använder `unit.CrewFor(type)`; befintliga skepp använder lagrad Crew, även vid
förluster. Ett skepp med Crew=2 får inte automatiskt räknas som Crew=20.
100 är soldatkohortens storlek, inte en ny kopierad matranson.

| Typ | Full CrewFor | Gammalt, alla aktiva statusar | Föreslaget hemma | Föreslaget ute |
|---|---:|---:|---:|---:|
| Galley | 20 | 0,04 | 0,10 | 0,20 |
| War Galley | 50 | 0,06 | 0,25 | 0,50 |
| Emporos / merchantman | 10 | 0,03 | 0,05 | 0,10 |

Alla tal är mat/tick. Silver stannar vid 1,5 / 2,5 / 1 per skepp.
Orderns ”0,002 per roddare” gäller Galley; gamla motsvarigheten var 0,0012 för
War Galley och 0,003 för Emporos. Den nya formeln använder samma personranson
för alla tre och inga av dessa gamla kvoter.

### Statusmappning att godkänna

| Status / situation | Förslag |
|---|---|
| garrison | hemma, grundranson |
| repairing | hemma, grundranson |
| fitting_out | hemma, grundranson; besättningen är redan dragen |
| marching | ute, fältranson; även retur, passage, explore, patrol och landuppdrag |
| positioned | ute, fältranson; även passage_wait och annan stationär väntan till sjöss |
| freighting | ute, fältranson under hela bunden resa/rutt, även mellan ben |
| laid_up | noll mat och silver, oförändrat |
| forming / training / disbanded | ingen ny tickdebitering; behåll befintligt aktiva-statusfilter. Rekryteringsprognosen visar färdigt, fullbemannat garrison-skepp. |
| embarked | landlastens befintliga fältranson, separat från roddarnas; naval är inte embarked |

**Freighting mellan ben behöver ett uttryckligt ok:** mitt förslag låter statusen
betyda ute under uppdraget, även när `FreightPort` tillåter mat från faktisk ägd
hamn. Matkälla och ransonsstorlek är olika frågor: FreightPort styr redan
var maten får tas. Alternativet ”hamnranson mellan ben” kräver samma fysiska
hamnklassning i tick/prognoser/läsytor och kan inte införas genom enbart
statussträngen. Förslaget ovan undviker en ny sådan parallell klassning.

## Ägare, beroenden och alla produktionsläsare

Nuvarande `economy/ship_ration.go` äger UpkeepSpecs, UnitUpkeep, VoyageRation,
VoyageProvisions. `combat/upkeep.go` exporterar kompatibilitetsanrop. Formlerna
ska stanna där; ingen klientformel. **Nuvarande signaturer saknar Crew**:
UnitUpkeep(kind,category,size,status), VoyageRation(kind,shipSize,cargo,size).
Naval size är skrovantal=1 och får inte användas som besättning.

Förslag: lägg till ett **obligatoriskt Crew-argument** i båda funktionerna och
combat-adaptrarna. Byte av befintligt int-arguments betydelse eller valfri
fallback skulle låta glömda anrop kompilera med fel besättning. Tick/läsning
skickar faktisk Crew, nyrekrytering skickar CrewFor. Economy behöver då inte
importera unit; G1 tillåter för närvarande **inte economy→unit**, och inget nytt
paketberoende behövs.

| Produktionsställe | Vad som måste följa med i samma slice |
|---|---|
| economy/ship_ration.go | Härled naval mat ur spearman och faktisk crew; silver orört, laid_up noll; VoyageRation använder samma ägare och fältranson + landlast. Ersätt gamla naval Grain-tal, inte bara några anrop. |
| combat/upkeep.go:291 | SELECT läser redan crew. Ge den till ägaren vid varje tick; kvarvarande atHomePort/provisionSourceFor väljer matkälla och ändras inte till ny ransonsägare. |
| combat/march_start.go:1356 voyageNeed | Har unit.Unit.Crew; samma nya ranson i marschpreview, dispatch, explore/patrol och andra fartygsmissioners lastning. Landlast fortsatt fältranson en gång. |
| transport/transport.go:232–248 prepareJourney | Dispatch och CreateShadow delar vägen. Läs crew jämte type/size. Tur/retur och stående rutts väntreserv använder samma VoyageRation. Return-ben lastar inte ny mat. |
| combat/standing_orders.go:391 | Läs crew i ruttens preflight/reserv före transportens verkliga lastning; samma VoyageRation. Landkaravanens standingOrderRation lämnas orörd. |
| combat/standing_orders.go:96 standingOrderNavalRation | Direkthämtar gammalt merchantman-skrovstal. **Ingen produktionsanropare finns**, bara definition/kommentarer och tester. Föreslår ta bort föråldrad död helper + dess rent gamla tabelltest, så ingen gammal naval Grain-läsare lämnas kvar. |
| combat/storage_forecast.go:43–61 | SELECT/Scan behöver crew för verkliga enheter; dess befintliga urval av stadsmat kontra ombordmat behålls. |
| api/handlers/unit.go:2875,2890 | UnitSummary har redan Crew. Provisionsdagar använder ny VoyageRation med faktisk crew och last; upkeep_food/grain_per_tick använder samma crew/status. |
| api/handlers/province.go:396,2697 | Rekryteringskapacitet och varning behöver full CrewFor(type), inte naval size=1. |
| api/handlers/province.go:armyUpkeep / settlementUpkeepDrain | Båda SELECT/Scan behöver crew för befintliga skepps mat i garnison/stadens aggregat. Behåll deras olika redovisningssyften. |
| api/handlers/unit_march_silver.go:84 | Anropar UnitUpkeep men använder endast Silver; måste följa ny signatur. Mat-/silverprognosens silver får inte ändras. |

Sökning över produktions-Go efter `UnitUpkeep(`, `VoyageRation(`,
`UpkeepSpecs`, `CrewFor(` och gamla 0.04/0.06/0.03 hittar inga ytterligare
navala matägare. `ship_load.go`, `ship_ledger.go`, `food_provisions.go` och
`ship_port.go` tar redan ett behov/lager och äger inga ransonskonstanter.
Retur/frigörande bevarar faktiska varor och ska inte skalas eller nyskapas.

## De fyra ytorna och dokumentationen

- **Temenos:** ägaren, tick, alla avfärder/preview, frakt, reserv och läsytor ovan.
- **Keryx:** cmd_unit.go läser upkeep_food/grain_per_tick och provision_days från
  API; cmd_recruit.go läser rekryteringsupkeep/sustainable. Ingen klientformel
  behövs. Uppdatera ransonsfixturer, kontrollera faktisk output och silver.
- **Webb:** ui/drawers/war.js och city_production.js läser API-tal; rekrytering
  och ekonomisiffror ska följa API. Ingen ny UI behövs; uppdatera fixturer och
  prova att rätt tal visas. **Sidofynd:** war.js:576 döljer fortfarande
  proviantbadge för freighting med en gammal kommentar om att servern inte
  modellerar fraktproviant. Det påverkar inte debiteringen. Föreslår lämna ny
  badge utanför ORDER12 (annars ny yta/BILD), men ta bort/förtydliga felaktig
  kommentar om samma fil ändå behöver beröras.
- **Spelar-Codex:** upkeep.md behöver regeln för roddare och hemma/ute;
  sea.md supplies behöver samma sanning och faktisk tunn besättning; units.md
  Fit out kan länka till regeln. Inga gamla 0.04/0.06/0.03-tabeller hittades i
  artiklarna, men generella ”vanlig skeppsranson” behöver göras begripliga.
- **Vault temenos_enheter.md:** beslutet 2026-10-22 är redan infört, men rubriken
  ”Föreslagen matkalibrering, ej beslutad” längre ned beskriver samma fråga som
  fortfarande öppen. Vid godkänd implementation ska den ersättas av hänvisning
  till beslutet, inte lämnas som konkurrerande sanning. Inga vaultändringar nu.

## Befintliga skepp och utrullning — måste godkännas före kod

Samma proviant räcker betydligt kortare med ny ranson. Full Galley ute går från
0,04 till 0,20 (5×), War Galley 0,06→0,50 (8⅓×), Emporos 0,03→0,10 (3⅓×).
Ett tidigare korrekt provianterat aktivt uppdrag kan alltså svälta mitt i resan.
Ingen mat får multipliceras utan uttag i en fysisk hamn.

Read-only av **alla världars** icke-disbanded navala units, 2026-10-22
15:48:14 UTC, gav fyra, samtliga värld2178e795… tick2819:

| Id (kort) | Status | Faktisk crew | Ombord | Inverkan |
|---|---|---:|---:|---|
| 400abaf3 | garrison Galley | 20 | 0 | 0,10/tick hemma efter ändring, dagens0,04 |
| a3d5ee79 | garrison Emporos | 10 | 0 | 0,05/tick hemma, dagens0,03 |
| ec00d188 | garrison Galley | 2 | 0 | faktisk2 ger0,01/tick hemma, dagens0,04; viktig motvakt mot CrewFor på gamla skepp |
| c082acf0 | positioned Galley, passage_wait | 20 | 6,94e-18 | i praktiken redan tomt lager; även nuvarande0,04 saknar täckning. Ny ranson0,20 skapar inte denna befintliga tomhet. |

Ingen annan marching/freighting/exploring-ranson med positiv proviant hittades
vid **denna** avläsning. Den tidigare restavläsningen vid tick2812 såg samma
Galley marching med0,28; världen rör sig, så ett gammalt snapshot räcker inte
för deploybeslut. Rådata och exakt read-only SELECT finns bredvid rapporten.

**Rekommendation:** ingen backfill och ingen automatisk gratistilldelning.
Inför faktisk besättningsranson utan blandad gammal/ny ekonomi. Ta ny read-only
inventering precis före deploy; om ett skepp då har gammal proviant på ett
pågående uppdrag ska integrationen vänta tills det är åter i fysisk hamn,
eller Timothy uttryckligen välja hur övergången ska ske. Ge inte Codex tillstånd
att flytta/återkalla skepp eller fylla lager genom detta förslag. Den nu redan
tomma passage_wait-båten är ett separat observerat befintligt läge, inte ett
skäl att skapa mat genom migration.

Alternativ om omedelbar deploy under positiva gamla resor krävs: explicit
uppdragsbunden övergångsregel som behåller gamla ransonen till första hamn,
med sparad version och samma läsmätare. Detta är större scope/schema och
**rekommenderas inte utan separat ok**. Att bara slå om direkt under en gammal
finansierad långresa bör inte ske omedvetet.

## Bevisplan efter ok

1. Namngivet nytt ransonsprov RED mot nuvarande kod: alla tre skeppstyper,
   full/decimerad crew, hemma/ute/laid_up, silver identiskt. En spearman-radändring
   i provet ska påverka roddarransonen härlett, inte via kopierat tal.
2. Faktiskt tickprov drar ny ranson ur ombordvaror för marching/positioned/
   freighting; proviantering/preview/dagmätare för samma besättning och landlast
   överensstämmer; stod i hamn respektive retur och brist-rollback bevaras.
3. Stadens/recruitens prognoser, Keryx och webb visar faktiska tvåroddar- och
   fullbesättningsfall. Ingen ny layout; TEXT i playtest, FUNKTION via tester.
4. Egen fysisk mutation kopplar tillbaka naval till gammalt skrovstal eller
   ignorerar crew: namngivet prov RED → exakt återställning GREEN.
5. Full fresh Go-svit **en gång**, port57900, samt JS och vet. Gamla tester som
   explicit hävdar ”naval never doubles” måste ändras till den nya beslutade
   regeln, inte bara få en nya förväntade konstant utan crew-/statusbevis.

**Väntar på ok för statusmappningen, faktisk Crew i alla befintliga läsare och
övergång utan backfill med ny livekontroll före deploy. Ingen kod före detta.**
