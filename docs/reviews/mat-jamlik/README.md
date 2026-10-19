# Order 28a — jämlik mat

## SAMMANFATTNING

Korn, fisk, olja och vin äts proportionellt via en hjälpare av folk, armé,
skepp och karavaner. Tillväxtvariation följer faktisk måltid med tick; gammal
civil nödslakt/offer lämnas utanför. Proviant behåller varan under resan och
återlämnas även efter patrull/explore/pickup/skadad/erövrad retur.
Produktion `d22e1287` (granskningsfix för substantiell måltidsandel), tidigare `93b90bbd` (backend `0a92e208`, returfix `05b97416`, klient `c01112eb`).
Full fresh190 Go/vet/668JS gröna; efter returfixen hela combat/API och vet gröna.
Sju fysiska mutationer namedRED→byte-exaktrestore→freshGREEN. Verkliga rena
registreringar/CLI/hostmarscher/måltider/patruller/returer/Firefox1280+390 på
process05b97416 och93b90bbd, migration190.0SQL-skrivningar/API-replay.
Ingen merge/push/deploy; Claude granskar. Boskap28b/lagertak29 kvar separat.

## SLICE-KONTRAKT

| Fält | Kontrakt |
|---|---|
| Problem | Korn tar slut före andra matvaror; fiskstäder kan inte försörja armé/skepp trots mat. Lagerbaserad variation beskriver inte faktiskt ätande. |
| Spelarsanning | Korn/fisk/olja/vin äts proportionellt ur tillgänglig mängd. Alla ätande förbrukare använder samma hjälpare; tillväxtvariation följer dagens verkliga måltid. |
| Invariant | Samma ransoner, silverflöden, gamla audit-semantik och offer. Proviant återvänder som samma varor. Boskapens gamla nödföda orörd i28a; Claude18:37 avgränsade uttryckligen armé/skepp/karavaner till fyra varor. |
| Scope | economy mathjälpare/FoodTick, kharis tillväxt, combat upkeep/proviant/karavaner/återkomst, additiv persistence/API, keryx/web/Codex. |
| Non-scope | Hjordtak/avel/nödfödavärde28b, offer, nya ransoner, andra balansändringar. |
| Acceptans | 100korn/900fisk proportionellt; korn-only regression; 4 faktiskt ätna sorter växer snabbare än2; fish-only armé/skepp/karavan; per-varuretur; mutationer och naturligt prov. |
| Stopp | Konflikt mellan frusen eventsemantik och nytt ätande eller boskapens avgränsning kräver Claude/Timothy. |
| Bevisplan | Färsk DB-baslinje, pipeline-/livscykelprov, full färsk Go/vet/JS en gång vid slutet, fysiska mutationer, naturligt webb+CLI-prov. |
| Kedjegrind | **Bevisar** att matförsörjning möjliggör expeditioner och fysisk resursåtkomst utan falska kornspärrar. |

## BASLINJE

Första inventering på ff1ba813: fresh189 economy/kharis/combat GREEN,
fullJS GREEN. Claude integrerade26 som584a324a före implementationen;
grenen uppflyttades dit och färsk baslinje är grön separat (`baseline-integrated26-go.log`).

## LIVSCYKEL / ÄGARE

- Folket: FoodTick efter upkeep, innan growth. Dagens utfall lagras atomiskt med debiten och G2-claim.
- Tillväxt: redan verkligt unmet + food_net; variation måste läsa måltidsutfall, inte kvarvarande lager.
- Skepp: laddas i march_start, äts i upkeep, återlämnas i unit_arrival. Det är återlämning (inte kolonistförråd) vid~527.
- Stående karavaner: reserv och manifest delar samma källager; reserv måste fördelas per matvara innan manifestet bildas.
- Historisk skeppsproviant är korn; backfill måste bokföra den utan att skapa nya varor.
- Offer behåller individuella varor. Skördegåvans gamla mål inkluderar djur och får inte oavsiktligt ändras när matlistan blir fyra varor.

## RESULTAT / DATAKONTRAKT

Backend `0a92e208`, måltidsfärskhetsprov `cc754f82`, klient `c01112eb`.
Migration190 lagrar verklig civil måltid + dess tick och varorna ombord.
Historisk positiv skeppsproviant fylls som exakt samma mängd korn. Ny civil måltid
skrivs med debiten och befintlig G2-claim; tillväxten läser endast aktuell ticks
måltid. Ingen rekonstruktion från lagret efter att folket ätit.

`FoodGoods` är den enda listan för grain/fish/oil/wine. `PlanFood` gör proportionen,
`TakeFood` låser/debiterar inom konsumentens TX. Folket behåller gammal hel
nödslakt; andra förbrukare tar bara de fyra varorna (Claude18:37). Skeppets skalära
provianttotal och per-varubokföring uppdateras tillsammans; inkonsekvens ger fel,
inte en gissad konvertering. Stående reserv delas på samma sätt innan manifestet
byggs, så matlast och besättning inte överbokar samma källager.

Additiva API-fält: `food.goods`, `food.consumed`, `food.consumed_tick`,
`food.production_per_tick`, `food.consumption_per_tick`; `army_upkeep.food`;
roster `upkeep_food_per_tick` och `provisions_by_good`. Befintliga nominella
`grain`-alias finns för äldre klienter. Mat efter armén/tillväxtgrinden följer
fyra-varorsproduktionen; rekryterings-/offerrecept behåller sina individuella varor.

Audit: `FoodShortfall` behåller gamla faktiska grain/fish/livestock-fält och får
additiv `food_used`. `UpkeepSettled.grain_total` förblir faktiskt debiterat korn;
ny `food_total`/`food_by_good` redovisar resten. Silverflödena är oförändrade.
Ny faktisk attrition använder `food_shortage`; historisk `grain_shortage` läses
fortfarande. Nya civil-/garnisonsnotiser bär `food_category=true`; klienter använder
ny text endast då och behåller den gamla betydelsen i gamla payloads.

## GRINDAR

| Grind | Bevis |
|---|---|
| Kontrakt rött→grönt | `proportional-contract-red.log`: gammal90/900 mot rätt99/891. Fresh190 `contract-green.log`: fyra varor, atomisk måltid/retry, ingen debit vid avslag,2vs4 faktiskt ätna sorter trots lika slutlager, gammal måltid ger inte bonus, fisk-only armé/skepp/karavan, varuretur, verklig migrationbackfill och ledgerfel. |
| Kod | Full färsk190 `full-go.log` GREEN inklusive world140.201s; `vet.log` GREEN;668JS GREEN (`full-js.log`). Fullsviten på färdig klientcommitc01112eb, ingen mutation samtidigt. Efter det naturliga returfelet: fresh190 hela combat/API GREEN (`post-return-fix.log`), vet GREEN (`post-return-vet.log`), nytt resolve/retry-prov med4returvägar och fysisk mutationsbevis GREEN. |
| Klient | `client-go.log`, `client-js.log`: serverns värden används även när legacygrain motsäger nyafoodfält; alla4varor/manifest och gamla notisers betydelse provas. |
| Mutation | `mutations.py`/`mutations.json`: fysiskgrain-first, lagerbaseradvariation, fisk→kornretur, avstängd gemensam hemavlastning samt CLI/web utan vin; varje namngiven RED→byte-exakt återställning→GREEN på ny190 (JS utanDB). |
| Semantik |1:1 för fyra varor; civil nödföda oförändrad; samma matmängder och ransoner som före i korn-onlyprov; gamla auditfält behåller faktiska varor. |
| Användare | Naturlig registrering/CLIjoin, vanlig hostmarsch till känd kust, grundning, riktig blandad måltid, vanlig patrullpreview+dispatch, foodmanifest/retur, CLIstatus/city/unit/Codex och Firefox1280/390. NativeE GREEN på05b97416/190: faktisk måltid tick7 (4.9587118grain +0.0412882fish), blandad proviant ombord och tom ledger i garrison efter vanlig patrull.0pageerrors/overflow. NativeG GREEN på93b90bbd/190 med ny småmängdstext; samma riktiga kust/måltid/patrull/retur. NativeH också GREEN på93b90bbd/190 med synlig närbild av matraden (`native-h/food-row-mobile.png`) och manifest (`native-h/army-mobile.png`); faktisk måltid tick1 och verklig patrullretur med tom ledger. |
| Drift | Ingen merge/push/deploy; Claude granskar och integrerar. |

## RIGGDIAGNOSTIK / KÄNDA AVGRÄNSNINGAR

- Tidiga riktade körningar är utvecklingsloggar. Korn-före-fisk/army-fish-förbud
  i gamla tester ersattes av den nya regeln; silverprovet fick legitim per-varu-
  proviant i sin fixtur. Ett SQL-fixturfel (`numeric` kontraFLOAT8) rättades;
  färsk `api-ledger-green.log` och sedan fullsviten är gröna.
- `backend-port-collision.log` är ett avbrutet Docker-startförsök på upptagen port,
  inte ett testutfall. Nästa körning på egen port startade ny DB och var grön.
- NativeA grundade inlandet och gav rätt korn-onlymåltid, inte blandad mat.
  NativeB hade ingen synlig kust och avbröt innan grundning. NativeC nådde verklig
  kust/måltid men riggen valde skeppets egen hamnhex; korrekt400-avslag. Slutreceptet
  använder vanliga hostmarscher för att hitta synlig kust och väljer känt vatten
  minst två hex från staden. Dessa försök är bevarade och räknas inte som slutbevis. NativeD hittade ett verkligt äldre returfel: explore/patrull återgarrisonerade utan avlastning. Ett nytt resolve+retry-prov blev namedRED på explore/damaged/captured-return. Avlastningen flyttades till befintlig gemensam faktisk hamngarnison, före carrier-witness/commit; pågående passage/land-leg behåller maten. Pickup-return ingår också i slutprovet. NativeE är grönt efter fixen. NativeF läste en tom övergångsrender mellan viewportbyte och API-svar; riggen fick explicit väntvillkor per viewport. NativeG funktionellt grön på93b90bbd, men en full drawer-bild råkade fånga tom render. NativeH tar dessutom själva matraden och avvisar blanka bilder med ett pixelnärvaroprov (inte ett läsbarhetsmått).
- Ingen hjordkapacitet/djurvärde ändras i28a;28b är separat. Ingen manuell
  slakt, ny matlagning, offeromtolkning eller ny matkvalitetsregel införs.
- Ticklogs äldre historik bygger som tidigare på aktuella produktions-/förbruknings-
  prognoser, inte rekonstruerade historiska måltider. Den nya måltidskolumnen är
  senaste faktiska utfall, ingen komplett måltidshistorik. Status visar det faktiska
  senaste målet med sin tick; äldre ticklogg får ingen påhittad ny varufördelning.

## REPRODUCERA

```sh
tools/gotest.sh
node --experimental-default-type=module --test $(rg --files web/static/js | rg '\.test\.mjs$')
python3 docs/reviews/mat-jamlik/mutations.py
python3 docs/reviews/mat-jamlik/native.py docs/reviews/mat-jamlik/native-h
```

## GRANSKNINGSFIX — substantiell variation

Claude19:12 krävde att en vara utgör minst10 % av måltiden för variationsbonus.
`economy.VarietyMinShare` skickas till den befintliga growth-SQL:en. Den summerar
positiva, kända matvaror i aktuell ticks faktiska måltid och räknar bara varor
som når andelen. Proportionell konsumtion ändras inte; smulorna mättar fortfarande.
Codex growth/food säger ”a real share”, utan att låsa andelstalet som invariant.

`TestGrowthVarietyRequiresSubstantialMealShare` går genom riktig FoodTick och
kharis growth med identiska senare lager/rater:1vin/1000korn och99/901 ger ingen
bonus;100/900 (exakt gräns) och300/700 ger bonus. Före fixen namngivet RED:
smulmåltiden gav37 tillväxt mot33 för renkorn. Efterfix GREEN. Befintligt2vs4-
och gammalmåltidsprov fortsatt grönt. Ny fysisk mutation tar bort andelsvillkoret
så gamla `>0` åter räcker: namedRED→byte-exakt restore→fresh190GREEN.
Även grain-first och lagerbaserad variation är omkörda mot nya källhashar.
Ny full fresh190/vet/JS och naturligt prov redovisas i `variety-share-*`/`native-i`.

NativeI avbröts vid en skärmdump när en normal tickrefresh ersatte DOM-elementet.
Riggen återförsöker nu bara just detta explicit identifierade DOM-byte, med samma
begränsade fem fångstförsök; andra browserfel kastas vidare. NativeJ är GREEN på
d22e1287/migration190, verklig måltid tick5 (4.9587118grain/0.0412882fish),
blandad proviant, garrisonretur med tom ledger och inga pageerrors/overflow.
Matraden `native-j/food-row-mobile.png` inspekterad vid1:1. Ingen SQL/replay.

Slutkontroll efter granskningsfix d22e1287: **hela färska Go190 GREEN** inklusive
world149.461s (`variety-share-full-go.log`), vet GREEN och668JS GREEN.
Inga källmutationer överlappade slutsviten eller nativeJ. De tre omkörda
mutationerna och fyra oförändrade mutationerna har alla aktuell källhash.
Manifest/verifiering kan upprepas med `python3 docs/reviews/mat-jamlik/refresh-proof.py`.
