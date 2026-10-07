# Raklinjefallbackar vid recall — uppgift B

## SAMMANFATTNING
Utgår från master 77bda49c i codex/raklinje. Bevisar kedjegrindens fysiska
order: recall/redirect får bara flytta enheten över en passabel väg.
Status: KLAR för Claudes granskning. Namngivet avslag ersätter nåbar
raklinje/origin-gissning. Fem mutationer, full fresh Go/vet/379 JS och
separata riktiga CLI/webbresor gröna. Ingen merge/push/deploy eller BILD;
aggregatgrenen lämnas orörd.

## SLICE-KONTRAKT (före kod)
| Fält | Kontrakt |
|---|---|
| Problem | ExecuteRecall accepterar saknad väg som raklinje och okänd position som ursprung. |
| Spelarsanning | Recall/redirect genomförs på riktig väg eller nekas med förklarat skäl. |
| Invariant | Ingen kursändring, ny arrival eller successaudit om position/väg saknas; gamla messengerclaims och fryst MarchRecall bevaras. |
| Scope | combat/recall_redirect.go, recall HTTP/delivery felrapport, DB-prov, berörda nåbara testfixturer, Codex och bevisverktyg. |
| Non-scope | Fryst MarchRecallHandler, automatisk expedition/retur/ship-hull, aggregat, balans/tid/eventformat. |
| Acceptans | Position och ny väg måste finnas; named422 direkt eller named OrderFailed+audit vid delivery; rollback+replay; giltig och trivial väg fungerar; fresh fullsvit/vet/mutationer och riktig spelarväg. |
| Stopvillkor | Canon eller eventsemantikkonflikt tas med Claude. |
| Bevisplan | Fresh riktad baseline, regression röd före produktion, återställningssäkrade mutationer, fullsvit/vet och ny PG16/Redis-rigg via register/join/march/recall utan SQL-fixturer. |

## BASLINJE / PREMISS
Fresh PG16 migration160: baseline.log grön för Recall/Redirect/HostSelf.
ExecuteRecall har två produktionsanrop: api/handlers/unit.go (nomadic host
personlig order) och messenger/order_delivery.go (recall/redirect inklusive
passage återbyggt kuvert). En nyvägsraklinje och en origin-positionsgissning i
core; ytterligare positionsgissning i HTTP-precheck.
MarchRecallHandler är workerregistrerad men ScheduledMarchRecall har noll
produktionsskrivare: kan läsa gamla köposter, ingen ny spelare kan skapa dem.
Dess duplicerade fallback lämnas helt orörd som FYND, ingen rivning.
recall.go saknar numera raklinjefallback. unit_arrival.go och ship_hull.go har
separata automatiska returer, inga ExecuteRecall-orderanrop; utanför scope.
Arkitekturprogrammets sex äldre map_tiles-fixturer är en historisk uppgift:
nuvarande fixturer inventeras och endast verkliga brister repareras.


## EXPERIMENT
| Prov | Resultat |
|---|---|
| Saknad ny väg efter sparad marsch | TestRecallNoRoute_CoreRejectsWithoutMutation, båda verb; rött före, grönt efter |
| Saknad gammal väg utan sparad route | Samma provs positionarm; ingen origin-teleport |
| Kuriravslag + replay | TestRecallNoRoute_DeliveryNamesReasonOnce, båda verb; en named OrderFailed och en audit med identiskt route-skäl |
| Direkt hostorder | TestRecallNoRoute_HTTPRejectsNamed; named422 vid både kurs/positionsfel |
| Vanlig enhet vid okänd position | TestRecallNoRoute_HTTPDoesNotDispatchFromGuessedOrigin; ingen messenger skapad |
| Trivial riktig väg | TestRecallNoRoute_TrivialRouteStillApplies; accepterad med befintlig minimitick |

Åtta ursprungliga regressionssubtests röda i red-before.log på oförändrad
produktion. Visningsnamnet läses från enhetens befintliga namnmodell (inte
infantry-radens fält name som används för skeppsnamn); efterprovet kräver
ett icke-tomt faktiskt namn. Ingen befintlig fixtur saknar kartplattor i den
riktade gröna recall/redirectsviten. Sexfixtursnoteringen avser äldre kod;
frysta eller redan rivna vägar ges inga nya livscykler här.

## PREMISSENS GRÄNS
Anropsvägen är nåbar av spelare; själva fallbacken är normalt inte nåbar på
intakt statisk karta efter en legitim landmarsch. Retur och dispatchvaliderad
redirect ligger i samma sammanhängande komponent. Ingen normal spelarorder
river map_tiles. Saknad karta/äldre enhetsdata eller DB-fel kan ändå nå
fallbacken. Regressionerna degraderar därför kartan efter marschens sparade
rutt; de bevisar leveransens auktoritet, inte en normal spelarhandling som
förändrar terrängen. SQL används endast i DB-regressioner, inte spelarsetup.

## FYRA YTOR
Temenos nekar i core/HTTP och bär samma OrderReject.reason till deliveryns
befintliga named OrderFailed+OrderDeliveryFailed. Web format.js OrderFailed
visar redan name+verb+reason och API-fel visas av den befintliga errorvägen.
Keryx skriver API-felet och hela notispayloaden; hjälpen för recall/redirect
beskriver vägkravet. Spelkodexens marching.md beskriver samma spelarutfall.
Ingen ny notiskind, UI-layout, G1-kant, migration eller balansändring.

## KÄNDA AVGRÄNSNINGAR
MarchRecallHandler och dess gamla eventsemantik är helt orörda. Registrering
bevisar bara konsumtion av historisk kö, inte ny spelarnåbarhet. Dess origin-
och raklinjefallback är FYND för Claude/Timothy att bedöma, ingen rivning.
Automatreturer i unit_arrival/ship_hull är separata fynd utanför recallorder.
Singelns två-commit messengerclaim/kurs och befintliga tidsmodell är bevarade.
DB-/infrastrukturfel ger befintligt tekniskt HTTP-fel/generiskt named
OrderFailed; råa SQL-fel exponeras inte. Grafen kontrolleras, men denna slice
ändrar inte den befintliga best-effort-lagringen av route efter lyckad FindPath.


## GRINDAR
| Grind | Utfall |
|---|---|
| Kod | Full tools/gotest.sh NY PG16/mig160 grön, world90.9s; full vet grön; 379 JS gröna. Se full-go.log, vet.log, js.log. |
| Mutation | Fem giltiga assertion-röda och varje arm återställd grön; mutations.log + mutations/*.log. |
| Semantisk | Befintliga web/CLI-felrenderare visar name+reason; exakt domänskäl förs från core till HTTP/notis/audit. |
| Användare | CLI register/join/found→två Keryx-marscher→recall+redirect; actual per-unit audits och båda garnison. cli-live/proof.json. Separat ny browserrigg: riktiga War Recall/Redirect-knappar, Runner-text, båda audits/garnison, inga browserfel. web-live/proof.json. |
| Visuell | Ej BILD-slice, ingen bildändring eller ögonkoll enligt ordern. |
| Drift | Ingen merge/push/deploy, Claude integrerar. |

Båda riggarnas /healthz från respektive process: commit
3911cbe30cbab3116c698f8363b368d49d0691fe, migration160, statusok.
Browserriggen klickar riktiga War-knappar efter samma register/join/found och
två Keryx-marscher, utan API-fixturer eller snapshots. Ingen BILD bedömd.
SQL endast för read-only audit/köbevis; spelarsetup och order görs med API/CLI.
Full-Go-headerns +ändringar avser rapporten; all Go/klientproduktion motsvarar
3911cbe3. Ingen kvarvarande relevant saknad-map_tiles-fixtur föll i fullsviten;
inga sådana fixturändringar behövdes.

## METODISK LÄRDOM
Räkna eventskrivare och core-anrop separat från workerregistrering; skilj
nåbar kodväg från möjlig felgren på en intakt karta. Mutationer måste riktas
mot rätt verbs avslag: första generella OrderReject-träffen kan tillhöra
ett annat verb. Bevisverktygets PG-port måste väljas bland faktiskt lediga
portar när flera agentsviter delar maskin; portkonflikt räknas aldrig som rött.

## Resume checkpoint
Funktionell produktion: **3911cbe30cbab3116c698f8363b368d49d0691fe**.
Kontrakt d13d91a8 → rött a1298108 → grönt 3911cbe3. Slutlig beviscommit
följer och skickas till Claude; aktuell HEAD läses med git -C worktree rev-parse HEAD.
Worktree /tmp/megaron-codex-raklinje-20261007, gren codex/raklinje,
bas 77bda49c. Nästa: Claude granskar core/felrapport och sin egen mutation,
integrerar mot sin aggregatgren och bedömer separat frozen MarchRecall-fyndet.
Aggregatgrenen är orörd; ingen rebase, merge, push eller deploy av Codex.
Recept: tools/recall_route_mutations.py OUT; tools/gotest.sh; server go vet ./...;
node --test $(rg --files web/static | rg '\.test\.mjs$');
tools/single_recall_live.py OUT 3911cbe30cbab3116c698f8363b368d49d0691fe cli|web
med båda binärerna i OUT och serverns buildCommit stampad till produktionshashen.
Båda klientarmarna använder var sin färsk PG16/Redis och rensad servermiljö.
