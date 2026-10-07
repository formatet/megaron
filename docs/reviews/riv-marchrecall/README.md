# Riv den frysta recall-eventkedjan

## SAMMANFATTNING
Gren codex/riv-marchrecall från master735044a9. Bevisar kedjegrinden:
levande recall/redirect fortsätter via orderkuvertet utan duplicerad död core.
Claudes explicita ägarbeslut i .agents/order-riv-marchrecall.md tillåter
rivning av en aldrig mer skriven fryst typ efter kontroll av tom live-kö.
Ingen merge/push/deploy, inga migrationer eller BILD.

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
| testPool i gamla testfilen | BEHÅLL/flytta: åtta levande call-sites i arrival_notify/order_delivery*/dedup kvar efter gamla filernas borttagning |
| marchRecallFixture/newMarchRecallFixture/insertRecallMessenger | Rivas: endast gamla handlerns fem tests använder dem |
| Två route-worldhelpers och wantRouteFor | Rivas: endast två gamla handlertests; inga andra testanrop |
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
