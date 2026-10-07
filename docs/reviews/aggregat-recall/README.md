# Aggregat-recall — premiss och kontrakt

## SAMMANFATTNING
Gren codex/aggregat-recall från master 77f584be (lagertak redan integrerat).
Claudes rivning 2e209682 är förfader till basen. Ingen produktionskod ändrad.
Premiss kräver svar från Claude om definitionen av samma plats; ingen merge/push/deploy.
Bevisar kedjegrindens fysiska order-/återkomstflöde, inte gamla asynkronitetsgrinden.

## SLICE-KONTRAKT
| Fält | Kontrakt |
|---|---|
| Problem | Ett recallverb når en enhet; grupprecall saknas. |
| Spelarsanning | Kunna återkalla flera egna enheter, med en fysisk kurir per faktisk plats och namngivna avslag. |
| Invariant | Ingen instant order utom befintligt CommandedInPerson; inget tyst bortfall; gamla events behåller sin betydelse. |
| Scope | Temenos batchrecall och levande order-/passagekedja, keryx recall --all/filter, webb och Codex, tester, verblista. |
| Non-scope | Riven legacy, shipping, andra nya orderverb och balans. |
| Acceptans | Delrapport för varje vald enhet; fysisk delad kurir; medlemsidempotens/dubbelordervakt; N hem i garnison via tick-loop; CLI/webb samma rapport; full Go/vet/JS och riktig klientrigg. |
| Stopvillkor | Samma plats är inte definierat av target/tick; Claude/Timothy måste fastställa innan produktionskod. BILD byggs men godkänns senare före merge. |
| Bevisplan | Oförändrad fresh-DB-baslinje; namngivna regressioner/mutationer; verkligt spelarflöde; 1:1-underlag senare för BILD. |

## PREMISSKOLL
- `UnitHandler.Recall` kräver owned unit, status marching, positionsdata och
  RequireShipInPort; fartyg i marsch avvisas. CommandedInPerson (nomadhost)
  använder samma ExecuteRecall direkt. Övriga får Runner från närmaste egna stad/host.
- Dubbelordervakten söker unprocessed/nonfailed ScheduledOrderDelivery med
  payload.unit_id och verb recall/redirect. Den ser inte en väntande sea-passage
  utan scheduled event ännu. Batch måste inventera både messenger och event.
- Kuriren riktas mot beräknad interception längs faktisk aktiv sparad rutt,
  annars befintlig path-interpolation. Saknas ärlig interception avvisas ordern.
- `OrderDeliveryHandler.Handle` claims messenger.status till arrived före
  ExecuteRecall. Samma messenger-id i N oförändrade recall-events får första
  medlemmen utförd, övriga blir no-op: ingen tunn serverloop kan lösa gruppformen.
- Passagekedjan reconstruerar ett OrderDeliveryPayload ur messengers.order_payload
  vid boarding/reserv. Gruppen måste överleva denna kedja och passage_generation.

## MÄTNING OCH CANONFRÅGA
[measure.py](measure.py) kör befintlig BuildRoute/RoutePositionAt med två syntetiska
plana rutter. Båda har target=(4,0), starttick=0, ankomsttick=10; vid tick5 står de
på (3,0) respektive (4,-1). Även ett nollbrett ankomstfönster är alltså otillräckligt
för samma plats. Detta är ingen DB/API/live-mätning; det är kanonisk ren positionering.
Råresultat: [premise.log](premise.log).

Fråga till Claude före produktionskod: gruppera vid gemensam interception OCH
exakt leveranstick där alla faktiskt står, eller konservativt endast identisk
aktiv rutt/ticks/dispatchorigin? Rekommendation är första formen. Inget antal
tick i ett fönster får gissas. Gamla recall/redirect måste förbli oförändrade;
ny gruppform behöver nytt namngivet verb/protokoll och medlemsidempotens.

## BASLINJE
`tools/gotest.sh ./api/handlers ./internal/messenger ./internal/combat -run 'Test.*(Recall|OrderDelivery|InterceptCourier)'` på färsk PG16/migration160, commit6313d541: alla tre paket gröna. Rålogg [baseline.log](baseline.log).

## Resume checkpoint
Lagertak överlämnat rent @3fc417fd; Claude har integrerat det i bas77f584be.
Aggregat-recall: kontrakt/premiss före kod, positionsmätning klar; väntar på
Claudes definition av samma plats. Fresh recallbaslinje är grön. Ingen produktionskod ändrad.
Worktree /tmp/megaron-codex-aggregat-recall-20261007, gren codex/aggregat-recall.
Nästa: svar på canonfråga → implementation enligt fyra ytor, fortlöpande commits.
BILD före merge senare; ingen merge/push/deploy av Codex.
