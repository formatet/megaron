# Singel recall/redirect — dedup och förklarade fel

## SAMMANFATTNING
Gren `codex/singel-recall-dedup` från master77f584be, worktree
`/tmp/megaron-codex-singel-recall-dedup-20261007`. Produktionscommit
**fe52e5e9d26a63c106bb858f53be3271a764c771**. Identisk aktiv UnitArrival återanvänds
i ExecuteRecall:s TX för både recall och redirect. Terminalt fel efter kurirclaim
ger namngivet befintligt OrderFailed och auditutfallet OrderDeliveryFailed.
Kedjegrindens fysiska order/återkomst bevisad; ingen BILD i denna serverslice.
Ingen merge, push eller deploy. Aggregatgrenen a92c99b3 är orörd.

Riktig isolerad riggs `/healthz`: **commitfe52e5e9d26a63c106bb858f53be3271a764c771,
migration160, statusok**. Färsk PG16/Redis, rensad servermiljö, tick6s.

## SLICE-KONTRAKT
| Fält | Kontrakt |
|---|---|
| Problem | Identisk köad UnitArrival gör singelrecall/redirect röd efter committed kurirclaim; order tappas utan audit/notis. |
| Spelarsanning | Kuriren vänder enheten eller ger namngivet OrderFailed och failure-audit. |
| Invariant | Gamla eventtolkningar och delad messenger/N events⇒bara första vänder bevaras; ingen migration, jitterändring eller balans. |
| Scope | ExecuteRecall:s arrival-enqueue, OrderDelivery recall/redirect-felrapport, DB-regressioner/mutationer och riktig klientrigg. |
| Non-scope | Aggregatgrenen, andra verb och fryst separat MarchRecall-handler som inte når samma core. |
| Acceptans | Dedup reproducerat utan jitter; båda verb återanvänder exakt aktiv arrival i TX; andra exekveringsfel ger name/reason/notis+audit; passage/legacy mätta; full fresh Go/vet +riktig recall/audit/hemkomst. |
| Stopvillkor | Ändrad fryst eventtolkning eller canon-/integrationskonflikt tas med Claude. |
| Bevisplan | Kontrakt före kod, fresh baseline, injicerad clock/DB-fault, assertionmutationer och separat färsk snabbtick-spelarväg. |

## BASLINJE OCH RÖTT FÖRE
Kontrakt91a138a5 före produktionskod. Fresh PG16/mig160 recall/redirect/passage/
stale-arrival i combat och messenger gröna: [baseline.log](baseline.log).
Regressionscommit29902079: injicerad klocka exakt tick1, sparad outboundrutt0→3,
positionq2, retur1→3 och typed UnitArrival3 redan köad. **Ingen jitter alls.**
Båda ExecuteRecall-verb SQL23505; båda delivery-verb returnerar nil efter claim
men target förblir4 och audit saknas. Separat injicerat unit-UPDATE-fel gav ingen
OrderFailed. Sex namngivna subtests röda, inga kompileringsfel: [red-before.log](red-before.log).

## EXPERIMENT OCH RESULTAT
| Ändring/fråga | Bevis |
|---|---|
| Återanvänd exakt aktiv world/type/tick/payload-arrival under unitlock | `IdenticalActiveArrival` och `DeliveryReusesArrival` för recall+redirect; en arrival och en successaudit |
| Annan payload/tick behålls, inte avbokad eller återanvänd | `DifferentEnvelopeIsPreserved` |
| Fel efter claim får named notice+audit, projection rollback, replay inget extra | `FailureAfterClaimIsNamedAndAuditedOnce`: scoped UPDATE-trigger; audit/notice samma name/reason/verb och instruktion att reissue |
| Incomplete order och miss ska också vara synliga | `IncompleteOrderIsNamedAndAudited`, befintligt `RecallMissedIsNeverSilent` |
| Gammal delad messenger-semantik låst | `OldSharedMessengerOnlyTurnsFirst` |
| Passage reconstructed envelope når samma fix | `PassageRebuiltEnvelopeUsesSameFix`, båda verb, verklig scheduleCompletion/generation1 |
| Fryst MarchRecall når annan core | `FrozenMarchRecallIsSeparatePath`: SQL23505, claim rollback till outbound; inte samma tysta committed-claim-fel |

Audittypen är ett nytt utfallet event, **inte ett nytt notiskind**. Befintliga
CLI/webb/Codex-ytor läser redan OrderFailed; inga verb-/UI-ändringar behövs.
Den gamla missfixturen hade enbart archived värld och kunde inte skriva
`events.world_tick`; testet har nu egen aktiv värld. Ingen ticktrigger ändrad.

## GRINDAR
| Grind | Utfall |
|---|---|
| Kod | Hela `tools/gotest.sh` på ny PG16/mig160 grön, inklusive world89.7s: [full-go.log](full-go.log). Full vet grön: [vet.log](vet.log). Ingen migration eller G1-ändring. |
| Mutation | Fyra källmutationer ger giltiga assertions röda: utan arrival-reuse, utan notice, fel audittyp, utan frozen claim. Varje arm ny DB, källor alltid återställda; hela TestSingleRecall grön igen: [mutations.log](mutations.log). |
| Semantisk/användare | Faktisk register→join→found→två Keryx-marscher→singelrecall respektive redirect på tick6s. Rätt per-unit MarchRecalled/MarchRedirected-audit och båda garnison; SQL används bara för läsbevis. [proof.json](live/proof.json), [serverhändelser](live/server-events.log), [live.log](live.log). Recall stänger expeditionen; redirect behåller sin befintliga intentsemantik. |
| Visuell | Ej tillämplig; ingen BILD/UI i ordern. |
| Drift | Ej körd, Claude integrerar och deployar. |

Riktiga riggen träffade samma ankomsttick5 som tidigare outbound. Recall återanvände
dess aktiva köpost och blev faktiskt verkställd, inte expeditionens egen hemresa.
Redirects redan befintliga explore-intent kan fortsätta efter omdirigeringen;
den faktiska redirectauditen och slutlig garnison är båda bevisade.

## INTEGRATION MOT AGGREGAT
Read-only `git merge-tree 77f584be codex/aggregat-recall codex/singel-recall-dedup`:
[preview](integration-preview.log). **Ingen faktisk merge eller branchändring.**
Konflikt i combat/recall_redirect.go:s arrival-block: välj singelns ovillkorliga
identisk-aktiv-reuse för alla ExecuteRecall-anrop, behåll aggregatets due_tick,
atomic receipt/kurs/audit och gruppprotokoll. OrderDelivery-förändringarna går
preliminärt ihop utan konfliktmarkör. Claude är meddelad; granskar integration.

## KÄNDA AVGRÄNSNINGAR / FYND
Den frysta ScheduledMarchRecall-handlern duplicerar core i annan TX och når inte
ExecuteRecall. Den har samma dedup-kollision, men rullar tillbaka claim och
returnerar fel för workerretry, i stället för att tappa efter committed claim.
Den är kvar registrerad för gamla köade events. **Mätt, inte fixad**, enligt
orderns endast-samma-kodväg-villkor. Claude avgör separat hantering av den vägen.
Gamla singelverb använder fortfarande walltime/current tick; ingen grupp-due_tick
eller ny tidssemantik införs här. Messengerclaim/exekvering är fortfarande två
commits: den befintliga processkraschluckan mellan dem är inte ändrad. Notisfanout
är best-effort; auditlagringsfel propageras efter att notisen har försökts.
Ingen karavanfil eller annan agents arbetsyta ändrad.

## METODISK LÄRDOM
Dedup och jitter måste separeras med en injicerad exakt clock. En marchingfixtur
måste ha sin verkliga ankomst köad; annars bevisar den inte orderns livscykel.
Garnison ensam räcker inte när expeditionen kan återvända själv: kräv orderaudit.

## Resume checkpoint
Funktionellt klar på **fe52e5e9d26a63c106bb858f53be3271a764c771**, beviscommits följer.
Aktuell slutlig grenhash skickas i överlämningen; läs `git -C <worktree> rev-parse HEAD`.
Worktree `/tmp/megaron-codex-singel-recall-dedup-20261007`, gren `codex/singel-recall-dedup`.
Nästa: Claude granskar/mergar singelfixen och samordnar arrival-hunken när
aggregatgrenen får BILD. Bedöm separat fryst MarchRecall-fyndet.
Recept: `tools/single_recall_mutations.py OUT`; `tools/single_recall_live.py OUT HASH`
med färdigbyggda temenos/keryx, temenos stampad via `-X main.buildCommit=HASH`.
Vaultens egen todorad och en uttryckligen EJ MERGAD not i temenos_orderlopare_plan
är kirurgiskt uppdaterade efter omläsning. Ingen merge/push/deploy av Codex.
