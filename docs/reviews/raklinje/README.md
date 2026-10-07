# Raklinjefallbackar vid recall — uppgift B

## SAMMANFATTNING
Utgår från master 77bda49c i codex/raklinje. Bevisar kedjegrindens fysiska
order: recall/redirect får bara flytta enheten över en passabel väg.
Ingen merge/push/deploy eller BILD; aggregatgrenen lämnas orörd.

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
