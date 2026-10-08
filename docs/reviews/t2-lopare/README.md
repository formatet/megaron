# T2 — löparens öde till sjöss

Kontrakt före kod, bas daf740ddd0efa051adc78c945b92d3c0c5d94d48, codex/t2-lopare.
Problem: stormförlist transport lämnar sin löpare aboard; sänkt/kapad bärare förseglar felaktigt löparen.
Spelarsanning: storm/ingen räddare innebär förlust med omedelbar fullständig dispatch till avsändaren; strid/kapning räddar löparen till ett verkligt skepp, som den lämnar i nästa hamn och fortsätter från.
Invariant: inga andra nyheter färdas omedelbart; fienden kan inte läsa eller hålla löparen; inga dubbelutfall eller gammal terminaltimer som levererar efter förlust/räddning.
Scope: carrier-death/capture TX och nytt hållbart vittnesmål, messenger-projektion/landstigning, events/migration/main, webbdispatch/Keryx/Codex och prov/rigg. Non-scope: T1-risk/tärningar/andra stridsutfall, gudar/riter, Umami/U.
Acceptans: tre röda verkliga livscykelprov → grönt; full fresh Go/vet/JS; fyra begärda mutationer; riktig privat payload och webb/Keryx; Firefox/Chromium/WebKit desktop/mobil om webben ändras.
Stopvillkor: kanonlucka tas till Claude. Arkitektur: utfall en gång där skeppet dör/kapas, i samma TX, uppåt som ny händelse enligt G1; ingen direkt messenger-SQL från combat/transport.
Kedjegrind: bevisar fysiska order-/transportkontrakt; bud över havet ska inte fastna på en obefintlig återkomst.

Implementation och slutgrind klara. Utökad kanon från Claude 23:05: upplösning av svält/desertion skriver också utfall i döds-TX; vid/intill aktiv bosättning går löparen i land, till sjöss förloras den. Gäller egna och redan räddade löpare.

`internal/carrier` äger fysiska vittnesmål och har bara `events` som intern kant. G1-listan och arkitekturprovet innehåller paketet och dess konsumenter i samma slice. Migration 162 är additiv: tillämpat vittnesmåls-id och partiellt eventindex. Gamla eventtyper ändras inte.

Fyra nya privata typer på messenger-streamen: `CarrierPassengerLostV1`, `CarrierPassengerRescuedV1`, `CarrierPassengerLandedV1`, `CarrierPassengerRedirectedV1`. Storm, strid, transportkapning, upplösning och faktisk hamnankomst skriver i sin egen TX; messenger äger projektion/notis. Pending-vittnesmål följs även före projektion, så en räddare kan hinna dö eller nå hamn mellan scans. Gammal terminaltimer spärras, generation uppdateras och tillämpat id ger en enda arkiverad dispatch vid dubbelscan. Landstigning finns kvar när skeppet redan vänt och lämnat hamnen.

Förlust ger `MessengerLostAtSea` på första passage-scan, med fryst HELT kuvert (brev, handelsvillkor eller order), avsändarens Wanax-id, avsändningstid och ursprungligt från/till. Privata skepps-/räddar-id och position publiceras inte i undantagsnotisen. Räddning ger ingen fjärrnotis vid strid eller landstigning: API/utkorg visar `no word`, kartan och eyes döljer löparen, även väntnotiser spärras. `MessengerRescuedAtSea` arkiveras först vid fysisk hemkomst med svar; envägsorder ger ingen magisk hemrapport. Limped är omdirigering på samma levande skepp, inte räddning/förlust.

Webbdispatch, Keryx och Codex visar hela kuvertet/hemrapporten. De fyra JSON-fixturerna kommer från verkliga arkiverade DB-notiser i livscykelproven, inte handbyggda förväntningar.

Bevis före slutgrind:

- `baseline.log`, `red-transport-storm.log`, `red-three.log`: grön bas och de tre ursprungliga faktiska röda livscyklerna.
- `red-upkeep.log`: hamn/hav röda; `red-boarding-stall.log`: död bärare och fjärrnyheter röda före sina spärrar.
- `green-final-focus.log` och `mutations/restored-*.log`: berörda livscykler och 20 upkeep-fall för egna/räddade löpare gröna.
- `mutations.log`: åtta riktiga mutationer, en i taget med färsk DB. Kuvert borttaget, landstigning borttagen, dubbelscan duplicerad notis, storm som räddning, carrier läckt till sender, hamn som förlust, hav som hemhamn, gammal terminaltimer tillåten. Var och en ger namngivet rött prov; alla källor återställda och gröna.
- Browserriggen är uttryckligen renderingsbevis med verkliga arkiverade DB-payloads och produktionsmoduler/CSS. Preferences-GET är en märkt read-only HTTP-fixtur. Go-proven bevisar den verkliga DB-livscykeln; riggen påstår inte en livebackend eller iOS-paritet. Firefox först, sedan Chromium/WebKit, desktop och 390×844. Bilder och maskinresultat i `browser/`.

Ingen merge, push eller deploy. Äldre rader utan nya vittnesmål har kvar det avgränsade legacy-skyddsnätet; ny fysisk död/kapning är kontrakterad via vittnesmål.


Slutgrindens källor: produktionskod `8926303882045ee4e3a55bdffd79058375acebe9`; API-riggens uppdatering `d9d7c392` följer verklig PassageScan-projektion och den aktuella generationens schemalagda terminaltimer. Första fullkörningens sex gamla API-riggar stannade vid den numera avsiktligt spärrade gamla timern (`red-api-timers-full.log`). Ingen spärr togs bort; `green-api-scheduled-lifecycle.log` visar alla arrange/pickup-förlopp gröna med riktig hamnprojektion/ny timer. Produktionskod är identisk mellan dessa två commits.

Full fresh Go på `d9d7c392` är grön: 24 testpaket, migration 162, exit 0 (`final-go.log`). Full vet på samma kod/prov är grön (`final-vet.log`). `+ändringar` i Go-loggens header avser bevisfiler som browserriggen skrev under körningen; `git diff d9d7c392 -- server web tools CLAUDE.md` var tomt före slutproof-commit. Browser6 efter sista Codex-prosan är gröna (`browser.log`/`browser/results.json`, 30 bilder); 450 JS utan skips gröna (`final-js.log`), Temenos/Keryx build gröna (`final-build.log`). Visuellt granskade Firefox mobile loss-bottom, WebKit mobile loss-bottom och Firefox desktop rescue på faktisk skärmbild: fulla långa kuvert kan skrollas, slutkontroller är nåbara och hemrapporten ryms.

BILD-grinden före merge tillhör Timothy/Claude. Här finns konkreta bilder för den granskningen; inga mänskliga bild- eller textplaytestgodkännanden påstås. Ingen iOS-app ändrad eller testad i denna repo-slice.

Mutation-/arbetskopiekontroll vid första överlämningen ce681afd: samtliga åtta mutationer gav namngivet rött utan build-fel/timeout, återställda messenger/upkeep/API gröna; inga produktions-/prov-/verktygsändringar efter `d9d7c392`. `git diff --check` och Python-kompilering gröna. Endast bevis/loggar/bilder ändras i sista commit. Basen är uttryckligen låst `daf740dd` enligt START; Claude ansvarar för rebase, merge och deploy mot senare master.


## Granskningskomplement: varför de sex riggarna ändrades

Notation: **S** är skeppets faktiska `UnitArrival.due_tick`, **C** är det tick då PassageScan konsumerar dess landstigningsvittnesmål och **L** är `CourierTravel` från den faktiska landstigningsplatsen till löparens aktuella mål. Boarding köar redan en terminaltimer i generation **g** med planerad `due_tick = S + L`. Även noll geografiskt avstånd avrundas till minst ett tick i befintlig reselogik.

Produktionsordningen är `UnitArrival`/terminaltimer på prioritet **10**, PassageScan på **20** (`internal/events/priority.go`). Ankomsten skriver `CarrierPassengerLandedV1` i sin TX. Om en gammal terminaltimer hinner köras medan vittnesmålet väntar blir den en no-op. Scan projicerar den faktiska landstigningen, tar bort carrier-referenserna och köar terminalen i generation **g+1**, `due_tick = C + L` (vid fortsatt landväg). Om scan redan hunnit först blir den gamla timern i stället en no-op genom generationsvakten. Den nya timern körs när dess `due_tick` förfaller; en nödvändig ny sjöpassage ger i stället riktig `awaiting_passage` och en ny boarding senare.

De gamla riggarna körde samma riktiga handlers men hoppade mellan några valda händelser. Efter skeppsankomst körde de terminalen från boarding utan landstignings-scan. Fem använde dessutom skeppets ankomsttick som världens tick trots att den laddade terminalhändelsen kunde ha ett senare `due_tick`; handlern förutsätter att workern kontrollerat förfallotiden. Det var inget bevis för att löparen nådde målet vid skeppsankomsten. Vi behöll produktionsvakterna och rättade drivningen.

| Rigg (exakt testnamn) | Förut och varför det nu spärras | Riktig produktionsväg som riggen nu driver |
| --- | --- | --- |
| `TestArrangePassage_OutboundRoundTrip_ShipCarriesBothLegs` | Send → Arrange/boarding → skepp ut → gammal `MessengerArrival` utan scan → Reply → skepp hem → gammal `MessengerReturn` utan scan. Den första landningen har pending-vittnesmål; därför ingen leverans och ingen giltig Reply. Samma vakt gäller hembenet. | Båda faktiska skeppsankomsterna skriver landstigning. Scan vid S projicerar, köar respektive Arrival/Return i ny generation på S+L. Först verklig Arrival ger Reply; verklig Return ger `arrived`. |
| `TestArrangePassage_Unanswered_ShipCarriesHomeViaStayEnd` | Utlandning → gammal Arrival utan scan → förväntat StayEnd → hemben. Pending-vakten gör Arrival till no-op, så StayEnd har aldrig köats. | Scan efter utlandning → ny Arrival på C+L → Arrival köar faktiskt StayEnd → StayEnd/StartReturnLeg → boarding hem → hemkomstscan → ny Return på C+L. |
| `TestArrangePassage_OrderToOwnUnit_ShipGoesHomeImmediately` | Boarding → skeppet landsätter vid tom strand och vänder direkt → gammal `OrderDelivery` utan scan. Orderns claim-UPDATE avvisar pending-landstigningen, så målunit får ingen stance. | Strandsvittnesmål överlever att skeppet redan vänt. Scan på S projicerar den tomma strandens verkliga koordinater och köar ny OrderDelivery på S+L till landunit. Den nya generationen utför stance, utan väntan eller hemrapport från skeppet. |
| `TestArrangePassage_Pickup_ShipFetchesFromDifferentOwnPort` | Fetch till främmande hamn → riktig release/boarding hem → skeppet landar i sin andra egna hamn → gammal Return utan scan. Pending-landningen spärrar Return; löparen blir kvar `returning`. | Scan efter faktisk landning i ANDRA hemhamnen → gångväg till löparens ursprungliga hemstad → ny Return på C+L. Skeppet stannar i sin egen hamn; löparen går sista sträckan. |
| `TestArrangePassage_DispatchedMidTick_RunnerBoardsAtDispatchNotNextScan` | Direkt boarding vid mid-tick dispatch → skeppsankomst → gammal Arrival utan scan. Pending-landstigning spärrar Arrival. | Direkt boarding är kvar. Scan behövs för fysisk LANDSTIGNING, inte för att börja åka: faktisk skeppsankomst → scan på S → ny Arrival på S+L. |
| `TestPickup_UnitInland_ShipArrivesFirst_BoardsViaR4` | Pickup-dispatch boardar orderlöparen → skeppet väntar utanför stranden → gamla OrderDelivery körs på sitt förfallotick, men utan landstignings-scan. Claim avvisar pending-vittnesmålet; hämtad unit börjar aldrig gå mot stranden. | Strandsvittnesmål → scan på S → nya OrderDelivery på S+L till inlandsunit → riktig march → unitens egen ankomst/R4 boardar den på väntande skepp → skeppet går hem. |

Riggens `runUnitArrival` kör nu den riktiga PassageScan om fysisk ankomst lämnat ett pending-vittnesmål, med världens tick fortfarande S. Även StayEnd körs nu på sitt faktiskt köade `due_tick`, inte skeppets ankomsttick + en uppskattad vistelse. Terminaldrivarna väljer det verkligt köade event vars `passage_generation` matchar raden och flyttar världen till DET eventets `due_tick`. De syntetiserar varken färdig status, generation eller en ny terminalpayload. Detta kompletterar d9d7c392: innan denna precisering kunde inlands-pickup-riggen vänta med scan till den gamla terminalens tick, vilket visade en tillåten sen scan men inte normal drift på S. En full worker-loop simuleras inte av dessa acceptance-hjälpare; deploy-provet nedan kör dessutom den gamla timern direkt, utan filtreringshjälparen, både före och efter projektion.

## DEPLOY: redan aboard + gammal köad terminaltimer, inga vittnesmål

**Ja, den levande normala löparen kommer fram.** Migration 162 lägger till `carrier_witness_id = 0`; den rör inte befintlig passage-status, `passage_generation`, payload eller schemalagda terminalhändelser. Noll vittnesmål är INTE ett skäl att stoppa en timer. Arrival/Return/OrderDelivery kräver fortfarande att rätt passage-generation gäller och att löparen inte väntar på passage, är förseglad eller förlorad.

Två ordningar behöver skiljas:

1. Skeppet har redan landat med gamla binären när T2 deployas, eller ingen ny fysisk händelse har skrivit vittnesmål före terminalens förfall. Den befintliga timerns generation matchar, pending-frågan är falsk, och samma Arrival/Return/OrderDelivery slutför den redan planerade resan. Inget nytt landstigningsvittnesmål behövs bakåt i tiden. Detta är avsiktlig kompatibilitet med det gamla boarding-kontraktet, inte bevis på en ny fysisk ankomst eller en ny förlust/räddning.
2. Den ombordvarande löparens skepp landar med T2-kod efter deploy. Den verkliga ankomsten skriver även vittnesmål för den gamla raden (ingen spärr på vittnesmåls-id=0). Då konsumerar nästa PassageScan det, byter generation och köar ett nytt completion-event med faktisk C+L. Den gamla timern får ingen rätt att leverera genom pending-/generationsvakterna. Att workern hunnit markera gamla timern processed är ofarligt: scan köar en NY rad.

Om gamla bärare i stället redan var döda/kapade före deploy och saknar vittnesmål gäller fortfarande det uttryckliga legacy-skyddsnätet `detectLostCarriers` → `sealLostCarrier`, när scan ser `carrier_witness_id=0` och inga nya vittnesmål. Det är ett separat fall från en levande normal resa. Händelser som inträffar EFTER deploy producerar riktiga utfall även för gamla aboard-rader; någon allmän legacy-exemption från T2-utfall finns inte.

`TestCarrierDeploy_AlreadyAboardWithQueuedLegacyTimer` kör riktig Send/Arrange/boarding så att aboard-raden och den GAMLA köade payloaden/generationen redan finns utan vittnesmål. Det kör terminalhandlern direkt, utan riggens automatprojektion eller generationsurval:

- `landed-before-deploy`: riktig carrier-arrival används för den fysiska skeppspositionen; endast dess T2-vittnesmål tas bort för att återge en gammal-binär DB-snapshot (varken messengerstatus eller generation manipuleras). Den redan köade generation 1/tick 3 levererar, `carrier_witness_id` förblir 0.
- `lands-after-deploy`: faktisk T2-ankomst behåller vittnesmålet. Den gamla generation 1/tick 3 körs medan projektionen väntar och blir no-op, och markeras processed. Riktig scan på tick 3 köar generation 2/tick 4. Replay av GAMLA payloaden förblir no-op även efter scan; den NYA verkliga timern på tick 4 ger `delivered`. Detta täcker även en försenad scan/backlog på terminalens tick.

Båda kontrollerar att gamla payloadbytes är oförändrade och att vanlig resa inte skapar förlust-/räddningsnotis. Bevis `deploy-and-six-rigs.log`: båda deployfallen och samtliga arrange/pickup/synlighetsprov är gröna mot färsk migration-162-DB. Komplementet ändrar endast prov/drivning och dokumentation; produktionskod är oförändrad från 89263038.


Extra faktisk deploy-mutation: Arrival-vakten utökades temporärt till att avvisa alla bevarade `currentGeneration > 0`, alltså även den legitima äldre aboard-timern utan vittnesmål. `landed-before-deploy` gav namngivet rött på just utebliven legacy-leverans (`deploy-reject-legacy-mutation.log`), inte build-fel/timeout. Källfilen återställdes byte för byte i `finally`; fulla API/messenger-kontrollen efter återställning finns i `review-complement-fresh.log`. Detta är ett extra prov utöver de åtta tidigare mutationerna, inte en ny produktionsregel.

Verifieringsomfång för granskningskomplementet: endast två API-prov-/riggfiler och README/loggar tillkommer mot ce681afd. Hela API- och messenger-paketen körs med färsk migration-162-DB efter sista riggändringen och mutationens återställning; full vet och diff-check tillkommer. Inga produktionsfiler ändras, så tidigare full fresh Go24paket/450JS/build/browser6 gäller fortfarande samma produktionskod. De gamla riggarnas första fullröda logg behålls, inklusive vilket fel varje rigg gav.


## TEXT/BILD-rättning efter Claudes kärngranskning

Spelartexten är nu **"Sent on day N"** (kuvertets frysta `sent_tick`) och **"Home on day N."** (hemrapportens frysta `home_tick`). Ingen rå `sent_at` visas i T2-kuvertet; den behålls bara som auditdata. Webb, Keryx och båda Codex-artiklarna använder samma ordval, utan "game day" eller "tick" i dagetiketten. Riggens tekniska "returned physically home" är borttagen.

`sent_tick` fanns inte i underlaget. Den ännu omergade additiva migration 162 får därför, enligt den uttryckliga rättningsordern, också en nullable `messengers.sent_tick`: först ADD utan default, därefter SET DEFAULT `current_world_tick()` för NYA inserts. Befintliga rader får alltså inte dagens tick som påhittad avsändningsdag. När en historisk dag saknas visas **"Sent on an unknown day"**, aldrig härledd från osäker UTC/ändrad tickkadens. `carrier` fryser värdet i kuvertet; ingen ny intern kant behövs. `home_tick` fryses i befintlig fysisk Return-TX och ändras inte av en senare replay.

Proven visar att avsändningsdag 500 överlever när live-radens sent_tick ändras till 999 efter förlust. Hemrapporten bevarar den verkliga completion-dagen även när Return replayas tio dagar senare. Keryx/JS täcker också dag 0 och äldre okänd dag, och förbjuder den råa UTC-strängen i dagtexten. Fixturerna exporterades om från riktiga arkiverade DB-notiser (`day-wording-fixtures.log`).

Slutkontroller efter rättningen: hela fresh Go på migration 162, 24 testpaket exit 0 (`day-wording-full-go.log`); full vet exit 0 (`day-wording-vet.log`); **451 JS**, inga skips (`day-wording-js.log`); Temenos/Keryx build exit 0 (`day-wording-build.log`). Befintliga fysiska utfall, terminalspärrar och nio tidigare verifierade mutationer ändras inte av ordvalet. Scope-diff innehåller endast de två dagfälten/defaulten, deras kuvert/hemrapport, spelartext, Codex och tillhörande prov/bilder. Inga combat-/transport-/upkeepregler eller andra slicar ändrade.

Nya Firefox-bilder med produktionsformatterare och de nya verkliga payloads, desktop 1280×900 och mobil 390×844, båda PASS (`day-wording-firefox.log`, `day-wording-firefox/results.json`). De fyra begärda bilderna är visuellt granskade på faktisk skärmbild:

- [Loss-top desktop](day-wording-firefox/firefox-desktop-loss-top.png)
- [Loss-top mobil](day-wording-firefox/firefox-mobile-loss-top.png)
- [Rescue desktop](day-wording-firefox/firefox-desktop-rescue.png)
- [Rescue mobil](day-wording-firefox/firefox-mobile-rescue.png)

Loss-top visar det verkliga korta brevkuvertet med dagetiketten och hela panelen; riggen provar också fulla trade/order-kuvert och sparar lång orderns loss-bottom. Tidigare browser6-bilder bevaras som den föregående överlämningens historik; dessa är rättningens aktuella Firefox-bilder. BILD-beslutet före merge tillhör fortfarande Timothy/Claude. Ingen merge/push/deploy från Codex.
