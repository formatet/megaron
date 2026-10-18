# Slice 9 — restfolket är hundradelar av bästa matgubben

Restfolket fick tidigare hela omlandets kornpotential. Nu producerar `population % 100`
hundradelar av **en** bästa möjlig matgubbe, korn eller fisk, inklusive hexens byggbonus.
Arbetet är osynligt och tar ingen verklig plats; beläggning minskar inte restproduktionen.
Korn vinner lika avkastning. Närjord och konsumtion är oförändrade.

**Kod:** runtime `42f7010e`, sista Go/testcommit `766777b4` på `codex/restfolk`, bas `a2738fa1` (slice11 inkluderad).
**Status:** alla grindar gröna, hash + STOPP till Claude; ingen merge/push/deploy eller liveomräkning av Codex.
**Provenance:** naturlig server health `42f7010ec49b620ca0aa906f5cb2e10120598115 / 183`;
livekällans läsande health `a2738fa / 183`. Baslinje `e38acc13 / 183`.
**Kvar vid integration:** kör operatörens `recompute-all` omedelbart efter deploy så lagrade
takter får nya regeln. Den kan sänka befintliga städers matnetto; ändra inte balansen för att dölja detta.

## Kontrakt

| Fält | Innehåll |
|---|---|
| Problem | 42 restpersoner producerade mer än flera riktiga matlag. |
| Spelarsanning | r restpersoner ger r/100 av bästa möjliga matgubbens verkliga avkastning. |
| Invariant | En ägare; samma hex-/byggnadsutbyte; ingen plats tas; hela hundratal ger noll restmat. |
| Scope | AutomaticFood, recompute/start/prognos/required, API/CLI/webb/Codex, dry-run, livscykelprov. |
| Non-scope | Konsumtion/närjord/lagring/lojalitet, verkliga arbetares omfördelning, liveomräkning. |
| Acceptans | Scheria1042/1000; fisk/farm/otillåten plats; prognos=utfall; 60tick tre städer; rollback-dryrun. |
| Stopp | Annan spelregel än Timothys beslut om hundradelar/bästa matgubbe krävs. |
| Bevis | Namngiven gammalkod RED, fysiska mutationer, färsk Go/vet/JS, naturlig CLI/Firefox och privat livekopia. |

Blockerar läsbar mat-/produktionskedja. Funktionsändring med spelartext, ingen ny bildfamilj.

## Ägare och livscykel

`economy.AutomaticFoodForPopulation` väljer grain/fish med positiv tillåten platskapacitet,
använder `HexYieldPerWorker(rate, multiplier)` och multiplicerar endast med r/100.
API:s additiva `automatic_food.remainder_fish_per_tick` skiljer fisken från kornet.
`GrainPerTick` förblir korn-only; `FoodPerTick` inkluderar fisken.

Recompute skriver respektive vara, även när ingen riktig fiskare finns; stale restfisk
försvinner vid hela hundratal. Grundningsprognos, startplacering och food_required använder
samma båda matbidrag. Befintliga ProvinceGet/overview serialiserar samma ägare; CLI och
webb visar grain + fish separat, Codex citizens förklarar regeln.

Tillväxtens befintliga matgrind läser redan grain+fish och räknar om efter varje
populationsändring. Ingen ny tillväxtformel behövs. Nya hela gubbar följer slice11:s
stadslås/kapacitet och befintliga matplacering. Slakt, rekrytering, grundning, byggändring,
blockad och övriga recompute-anrop är fortsatt konsumenter av samma ägare.
Befintliga urval av nåbara/avstängda hexar och prognosens FOW-filter ändras inte.

Scheria-fixturen har nio plains, fyra hav och fem olivhexar utan farm. Vid1042:
**0,50 grain +0,42 restfish**, demand5,21. Fem placerade fisklag ger5,42fish och matnetto+0,71;
ett eller noll lag räcker inte. Lager/Sitos förändrar inte food_required=5.
Vid1000 är båda restvarorna0. Separat1142-fixtur gör bortglömd restfisk synlig också i
antalet start-/required-arbetare (5 med bidraget,6 utan).

## Baslinje och experiment

`baseline.log` är fysisk gammalkod på färsk183: `TestRestfolk_BestWorkerNotCatchmentSum`
kompilerar och faller med3,78 i stället för0,42. Samma körning sparar tre60tick-baslinjer.
Baslinjen är från före slice11, vars lås inte ändrar dessa sekventiella matförlopp.

| Hypotes | Prov | Utfall |
|---|---|---|
| En bästa arbetare ersätter summan | Nio plains; grain/fish, byggbonus, ingen plats, lika utbyte | Namngivet RED → GREEN. |
| Alla konsumenter måste räkna fisk | Forecast/start/required/stored rates; hela hundratal | Samma verkliga PG-förlopp, mutationer fångar varje borttaget fiskbidrag. |
| Dry-run måste faktiskt räkna | Samma reconcile/recompute i TX, läs deltan, rollback | Fullrader oförändrade; normal apply sparar exakt förutsägelsen. |
| Minskad restmat får påverka tillväxt | Samma startplaceringar, Sitos/FoodTick/growth60tick | Scherias brist stoppar tillväxt, hills/kust fortsätter; inga hungriga så länge lager föder. |

## 60 tick före/efter

Kontrollerade PG-fixturer, **inte naturligt spel**. Startpopulation1042, grainlager1200,
samma verkliga placeringar i respektive arm. Sitos, FoodTick och tillväxt körs varje tick.
Alla mellanvärden vid0/10/…/60 finns i [soak60.json](soak60.json).

| Stad | Arm | Matnetto tick0 | Population tick60 | Matlag tick60 | Matnetto tick60 | Grainlager tick60 | Sitos tick60 | Unmet |
|---|---|---:|---:|---:|---:|---:|---:|---:|
| Scheria,3 fisklag | Före | +2,070 |1102|4|−0,830|293,57|330,60|0|
| Scheria,3 fisklag | Efter | −1,290 |1042|3|−1,290|273,58|312,60|0|
| Hills,farmL1 | Före | +2,077 |1384|9|+3,904|515,88|413,10|0|
| Hills,farmL1 | Efter | +0,397 |1384|9|+0,544|426,17|413,10|0|
| Kust | Före | +1,970 |1430|9|+3,550|257,34|426,90|0|
| Kust | Efter | +0,710 |1430|9|+2,650|172,49|426,90|0|

Ingen automatisk omfördelning av befintliga gubbar görs för att kompensera balansbeslutet.
Scheria-fixturens matunderskott är avsiktligt synligt, samtidigt som faktisk svält fortsatt
avgörs av FoodTicks obetalda måltid. Timmer/sten i dessa matisolerade fixturer är0 i båda armarna.

## Operatörens dry-run på privat livekopia

`recompute-all -dry-run` var tidigare en lista över gamla kornrater utan omräkning.
Nu låses staden, verklig reconcile/recompute körs, grain **och fish** läses före/efter,
och allt rullas tillbaka. Fel räknas och ger exit1; normal körning committar samma väg.
Testet innehåller en gammal byggplacering och verifierar också rollback av omplaceringen.

Live lästes endast med `pg_dump`; dumpen ligger privat utanför git (innehåller kontohemligheter).
Återställningen kördes i egen PG16 utan server/ticks. Heartbeat-/tickclaim-tabellernas stora
historik exkluderades; samtliga spelmekaniska tabeller/rater och schema183 bevarades.
[Recept](snapshot_dryrun.py) ansluter bara till sin privata container.
[Bevis](snapshot/snapshot-proof.json) innehåller dump-SHA och hash/count för **alla public-tabellrader**
före/efter: identiska. Apply gjordes endast på privat klon, gav exakt samma deltan; nästa dry-run0ändringar.

| Stad i snapshot | Grain före → efter | Fish före → efter |
|---|---:|---:|
| Amnissos |37,9500 →32,2167|2 →2|
| Phaistos |6,1760 →4,2000|25,5000 →26,1460|
| Scheria (redan1102) |0,6800 →0,5000|4 →4,0200|
| Tylissos |4,9000 →4,9000|0 →0|
| Itanos |10,8067 →7,6067|0 →0|
| Mallia |7,7400 →5,9400|0 →0|
| Kydonia |2,1800 →0,5000|2 →2,4200|

7 previewade,6ändrade,0fel. Dessa siffror är en tidsbestämd snapshot, ingen prognos för
framtida livepopulationer. Ingen migration behövs; Go-formelns lagrade resultat kräver omräkning.

## Grindar och reproduktion

| Grind | Bevis |
|---|---|
| Kod | `full-go-final.log` exit0, färsk183 på766777b4 (endast bevisfiler ocommittade), world150,022s; `vet-final.log` exit0; `js.log`654pass/0fail/0skip. |
| Mutation | [mutations.py](mutations.py), `mutations.log`: nio kompilerande namngivna RED → exakt byteåterställning → färsk GREEN. |
| Semantisk | `lifecycle-dryrun.log`: Scheria, fisk/farm/hills forecast=start=required=recompute, rollover1000; ProvinceGet+overview; CLI+webb grain/fish. |
| Användare | [native.py](native.py): vanlig register/join/found och naturlig tillväxt på egen server; keryx placering/status, Firefox1280×900 och390×844. Ingen SQL-skrivning/API-override. `native-final/evidence.json`, skärmbilder och faktisk text;0pageerrors. |
| Drift | Läsande livehealth a2738fa/183 + privat snapshot rollback/apply-paritet; ingen deploy/liveomräkning av Codex. |

Mutationerna återinför gammal summa, tar bort fisk/byggbonus/platsgrind, utelämnar fisk i
recompute/start/required/forecast, samt committar dry-run. Varje fel fångas av namngiven assertion.
Mutationernas SHA anger respektive återställda fil; sista ändringen efter mutationerna var endast
kommandots föråldrade förklaringskommentar. Hela Go-sviten gäller den committade leveranskoden. Första `full-go.log` föll därför att
mitt nya dry-run-test lämnade en aktiv värld och störde retentionstester senare i paketordningen.
766777b4 städar den världen via t.Cleanup innan poolen stängs. Första körningen är ett
misslyckat mellanprov; endast `full-go-final.log` och `vet-final.log` är leveransbevis.

Reproduktion: `tools/gotest.sh`, `go vet ./...` i server, `node --test $(rg --files web -g '*.test.mjs')`,
`python3 docs/reviews/restfolk/mutations.py`, `python3 docs/reviews/restfolk/native.py /tmp/restfolk-natural`.
Snapshot: `python3 docs/reviews/restfolk/snapshot_dryrun.py PRIVATE.dump OUTPUT`; dump återdistribueras inte.

**Metodisk lärdom:** ett dry-run som inte kör formeln bevisar ingen uppgradering.
**Avgränsning:** prognos/lagerförändring bedöms enligt befintliga Sitos-/tillväxtregler; inga nya
pris-/underhållstal eller extra kompensationsbonusar. TEXT-begriplighet fortsätter i speltest.
