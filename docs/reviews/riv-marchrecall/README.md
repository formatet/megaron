# Riv den frysta recall-eventkedjan

## SAMMANFATTNING
Gren codex/riv-marchrecall från master735044a9. Bevisar kedjegrinden:
levande recall/redirect fortsätter via orderkuvertet utan duplicerad död core.
Claudes explicita ägarbeslut i .agents/order-riv-marchrecall.md tillåter
rivning av en aldrig mer skriven fryst typ efter kontroll av tom live-kö.
Status: KLAR för Claudes granskning. Ingen merge/push/deploy, inga migrationer eller BILD.

## SLICE-KONTRAKT (före kod)
| Fält | Kontrakt |
|---|---|
| Problem | Fryst oanvänd handler duplicerar recall-core och har kvar gissad raklinje. |
| Spelarsanning | Spelarens recall/redirect går fortsatt via samma OrderDelivery/ExecuteRecall. |
| Invariant | Riva endast efter live total0 och noll nåbara skrivare/anrop; ingen eventomtolkning eller borttagning av levande testskydd. |
| Scope | Gammal handler/payload/eventkonstant/priority/registrering, endast dess tester, kommentarer och kirurgiska vaultnoter. |
| Non-scope | ExecuteRecall/OrderDelivery-beteende, aggregat, övriga deadcodefynd, balans/tid/eventdata. |
| Acceptans | Tom live-kö read-only före rivning; deadcode-test+symbolkorsning; noll kodreferenser; full fresh Go/build/vet och priorityvakt gröna. |
| Stopvillkor | Någon live-rad oavsett processed/failed, eller nåbar användare av kedjan: stanna och rapportera till Claude. |
| Bevisplan | SQL BEGIN READ ONLY, produktionsskrivarinventering, testfunktionsinventering, baseline på ny DB, bevarat levande ETAskydd+mutation, fresh fullsvit/vet och grep. |

## BASLINJE
Live CT126 poleia: 2026-10-07 18:28:36 UTC explicit READ ONLY, total/pending/
processed/failed = 0/0/0/0. Första read-only-kontrollen loggad till Claude.
Färsk PG16/mig160 recall/redirect/prioritybaseline grön (baseline.log).
Deadcode -test före rivning: deadcode-before.log. Handlern rapporteras inte
som död eftersom workerregistreringen gör funktionspekaren RTA-nåbar; detta
bevisar INTE en eventproducent. Inventeringen hittar noll produktionsskrivare.
Verktyget byggt lokalt med nuvarande Go1.27.1 från samma x/tools v0.49.0;
delade ~/go/bin/deadcode ändras inte. Gamla binärens Go1.26-build kunde inte
läsa maskinens stdlib, det utfallet räknas inte som kodbevis.

## SYMBOLER OCH TESTER — före rivning
| Kandidat | Beslut och bevarat levande skydd |
|---|---|
| Gammal handler, constructor, payload, eventkonstant, priority, main-instans+Register | Rivas: inga skrivare, tom kö; båda aktuella core-anrop lämnas |
| testPool i gamla testfilen | BEHÅLL/flytta: fyra levande direkta call-sites i arrival_notify/order_delivery*/dedup kvar efter gamla filernas borttagning |
| marchRecallFixture/newMarchRecallFixture/insertRecallMessenger | Rivas: endast gamla handlerns fem tests använder dem |
| setupMarchRecallRouteWorld, setupFordMarchRecallWorld, wantRouteFor | Rivas: endast två gamla handlertests; inga andra testanrop |
| TurnsUnitTowardOrigin/RedirectSetsNewTarget | Rivas: live HTTP courier Recall/Redirect och SingleRecall_DeliveryReusesArrival skyddar aktuell kurs |
| IdempotentReplay | Rivas: SingleRecall_OldSharedMessengerOnlyTurnsFirst + FailureAfterClaimIsNamedAndAuditedOnce skyddar levande claim/replay |
| TooLateNoOp | Rivas: live RecallMissedIsNeverSilent testar faktiskt named utfall |
| ArrivesAtMatchesTickSchedule | Gammalt test rivas men K4-skydd flyttas till aktuell OrderDelivery via singleRecallFixture |
| RedirectSavesRoute/CatchesWhereTimeSays | Rivas: combat Acceptance1_ExecuteRecall_RedirectSavesRoute, Acceptance2_ExecuteRecall_CatchesWhereTimeSays och HTTP RecallPrecheck_CatchesWhereTimeSays kvar |
| Frozen...SeparatePath i dedupfilen | Rivas endast denna testfunktion: provar kollisionsfel i borttagen kedja; resten av filen BEHÅLLS |
| EventUnitMarchRecalled/MarchRecalledPayload | BEHÅLLS: levande auditutfall, annan typ än den rivna schemalagda ordern |
| Övriga sju deadcode-fynd | BEHÅLLS: orelaterade, ingen generell deadcode-rivning i denna slice |

## VAULT / TEXTER
Orderns temenos_march_recall.md finns inte i vaulten (även rg --files saknar
match); Claude underrättad. Befintlig fryst-not i temenos_orderlopare_plan
är mekanikens aktuella hem och uppdateras där, samt arkitekturprogrammets
steg10. Markerat RIVEN PÅ GREN/EJ MERGAD, inga livepåståenden före integration.
Spelkodex har noll referenser till den gamla typens namn; spelarverbet kvar.
Äldre processrapporter/loggar är historiskt bevis och skrivs inte om.


De sju orelaterade deadcode-symbolerna lämnas alla kvar: handlers.loadLaborCapacities
(labor), ProvinceHandler.Marches (annan handler), Client.patch och keryx.die
(CLI), economy.rankedFoodSlotsAt (grundande), Worker.RegisterWithTimeout
(worker-API), kharis.templeTierMultiplier (kult). Ingen är beroende av den
rivna eventkedjan; att ta bort dem kräver egna premiss-/testkontroller.
Delade combat.LoadActiveRoute/RoutePositionAt/BuildRoute/TravelFactor och
province.FindPath/InterpolatePosition/tick.LoadAnchor behålls: används av
aktuell ExecuteRecall och övrig levande rörelse. tickPriorityArrival är kvar
för enhetsankomster; combat.Broadcaster används av levande messengerhandlers.


## EXPERIMENT / GRINDAR
| Kontroll | Utfall |
|---|---|
| Live READ ONLY före rivning | total0/pending0/processed0/failed0, 18:33:05 UTC; live-before.log + live-check.sql |
| Live READ ONLY inför överlämning | Samma nollutfall 18:41:03 UTC; live-handover.log |
| Bevarat levande K4-skydd | TestSingleRecall_ArrivesAtMatchesTickSchedule för båda verb; gammal skyddspunkt flyttad före rivning i 2011eeec |
| Mutation | Verklig live-core ETA ändrad till moveTicks×time.Hour ger giltigt assertion-rött för båda verb; återställd grön, mutation.log + mutation/*.log |
| Riktade live-regressioner | Recall/redirect/arrival-dedup/passage/route/priority gröna, green-after.log |
| Kod | Full tools/gotest.sh NY PG16/mig160 ALLA paket gröna, world89.173s; build+vet gröna, full-go.log/build.log/vet.log |
| Priority | TestEveryScheduledTypeHasAPriority explicit grönt, priority.log; ingen prioriteringsvakt försvagad |
| Deadcode | Före/efter exakt samma sju orelaterade symboler; endast radnumret på RegisterWithTimeout ändrat |
| Kodgrep | Noll exakta gamla symboler/typsträng/filename i server+web+tools, grep-after.log |
| Semantisk | Runtime i ExecuteRecall/OrderDelivery och personliga order orörd; ändringar där endast kommentarer. Levande MarchRecalled-audit bevarad |
| Klienter | 379 JS-prov gröna; inga spelarytor/routade verb ändrade, inget nytt spelarflöde/rigg behövs för död kedja |
| Visuell/drift | Ingen BILD/ögonkoll, ingen merge/push/deploy enligt ordern |

Grep avser exakta gamla symboler och filnamn, inte substringen som också
finns i det levande utfallet MarchRecalled. Historiska docs/reviews-loggar och
rapporternas förebevis behålls. Ingen migration eller DB-rad raderas.

## METODISK LÄRDOM / AVGRÄNSNINGAR
Deadcode RTA-nåbarhet via workerregistrering är en annan sak än att det finns
en producent eller en live-köpost. En borttagen handlers testfil kan äga
hjälpare och ett regressionsskydd som måste få ett levande hem först.
Read-only-guard måste räkna ALLA rader, inte bara pending. Tom kö gäller de
angivna ögonblicken; inför senare integration kontrollerar Claude den igen.

## Resume checkpoint
Gren **codex/riv-marchrecall**, worktree
**/tmp/megaron-codex-riv-marchrecall-20261007**, bas **735044a9**.
Kontrakt/livepremiss **06e2efe2** → levande K4-skydd **2011eeec** →
rivning **b51d6dd96241c51e8864ab3de944d32dda7d297f**. Slutlig beviscommit
följer och exakt HEAD skickas till Claude; läs git -C worktree rev-parse HEAD.
Nästa: Claude granskar symbol/testinventeringen, gör egen mutationskontroll,
läser livekön igen och integrerar. Inget arbete väntar på Timothys ögon idag.
Alla andra agenters grenar och huvudträdet lämnas orörda.
Vault steg10 +orderlopare-not är kirurgiskt uppdaterade efter direkt omläsning,
med RIVEN PÅ GREN/EJ MERGAD. Egen todorad uppdaterad. Den angivna separata
vaultfilen saknas; inget nytt mekanikhem skapades, Claude underrättad.

Recept: tools/gotest.sh; server go build ./... och go vet ./...;
tools/recall_eta_mutation.py OUT; deadcode -test ./... (samma x/tools v0.49.0
byggt med aktuell Go, t.ex. GOBIN=/tmp/egna-tools go install
 golang.org/x/tools/cmd/deadcode@v0.49.0). Live: ssh root@10.0.1.92
'cd /tmp && su postgres -c "psql -X -d poleia -v ON_ERROR_STOP=1"'
< docs/reviews/riv-marchrecall/live-check.sql. SQL använder BEGIN READ ONLY.
