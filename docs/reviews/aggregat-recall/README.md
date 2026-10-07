# Aggregat-recall — premiss och kontrakt

## SAMMANFATTNING
Gren codex/aggregat-recall från master 77f584be (lagertak redan integrerat).
Claudes rivning 2e209682 är förfader till basen. Serverimplementation finns i detta checkpoint; klientytorna återstår.
Claude godkände 2026-10-07 17:15 faktisk gemensam interception och exakt leveranstick; ingen merge/push/deploy.
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
Worktree /tmp/megaron-codex-aggregat-recall-20261007, gren codex/aggregat-recall, bas77f584be.
Servercheckpoint: batchval/namngivna avslag, fysisk kurir per faktisk möteshex/exakt tick,
nytt recall_group_member-protokoll med atomic medlemsreceipt och egen orderleverans.
Claude godkände 17:28 att kuriren väntar på enheterna vid möteshexen; annan väg får egen kurir.
Fresh-PG16/mig160: gammal recallbaslinje efter extraktion grön ([prepare-baseline.log](prepare-baseline.log));
tre gruppprov gröna ([group-tests.log](group-tests.log)): alla tre enheter till garnison via worker/ticks,
replay utan extra ankomst och oförändrad gammal messengerclaim.
Nästa: kantfall/passage/race, CLI/webb/Codex, mutationer, full Go/vet/JS, riktig klientrigg och verblista.
Detta är ett mellancheckpoint, inte färdig överlämning. BILD före merge senare; ingen merge/push/deploy.

## Godkänd form och protokoll
Claude 17:15: gamla recall/redirect behåller sin messengerclaim; gruppverb får medlemsidempotens och varje medlem eget ScheduledOrderDelivery. Gruppens manifest måste överleva passage/reboarding. Batchrapport listar grupper, singlar och namngivna avslag.

## Klient- och livscykelcheckpoint
CLI `recall --all --target q,r --intent` och webbens War→Army-kontroll använder
samma batchrapport, grupper/singlar och alla namngivna avslag. Codex förklarar
fysisk mötesplats/exakt dag och individuella leveranser.
Utökade fresh-PG-prov: filters/distinkta möten, samtidiga dispatch/replay,
terminalt medlemsavslag/dead-letter och manifest/generation efter passage gröna.
Atomic rollback-provet tillagt efter denna körning; fullsviten återstår.
JS: 383/383 gröna. Riktig klientrigg finns i tools/recall_live.py; ännu inte körd.

## FYND: workerjitter vid riktig klient
Första webbkörningen på b6f90185, PG16/mig160, två riktiga startkohorter:
batch200/grupp med två mottagare, men båda avvisades när Handle jämförde deras
senare walltime-position med möteshexen. Det deterministiska provet dolde polljitter.
Råfel sparat [live-web-before-jitter.log](live-web-before-jitter.log). Claude 17:43 godkände att ENDAST nya
verbets möte, returens start/rutt och ETA ankras på eventets due_tick.
Kurs/status-snapshot måste fortfarande stämma; ändrad mottagare avvisas med namn.
Gamla enskilda recall/redirect ska mätas på riktig rigg och rapporteras, inte ändras.
Jitterprovet godkänt: samma möte och returstart/rutt/ETA vid tickleverans och tre
ticks sen handler. Statusändrad mottagare ger ett namngivet avslag, replay ingen
extra notis. Terminala gruppavslag/dead-letter claims receipt före best-effort
notis och avslutar delad kurir; internt gruppverb visas som spelarverbet recall.
