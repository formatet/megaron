# Aggregat-recall — överlämning

## SAMMANFATTNING
`codex/aggregat-recall`, worktree `/tmp/megaron-codex-aggregat-recall-20261007`,
bas master77f584be efter Claudes recall-rivning2e209682. Produktionskod a6884dc3.
Fyra ytor byggda: batch-HTTP, `keryx recall --all` med target/intentfilter,
War→Army och Codex. Fysisk kurir per möteshex/exakt leveranstick; varje mottagare
har egen OrderDelivery, atomic receipt/kurs/ankomst/audit och namngivna avslag.
Bevisar kedjegrindens order-/återkomstflöde. Ingen merge, push eller deploy.
**BILD väntar Timothy före merge**, enligt arbetsordern. Gamla singelverb har ett
separat uppmätt fel (nedan), inte ändrat i denna slice.

Provenance ur varje riktig servers `/healthz`: **commit
`a6884dc32edef35e7649df484f1e70437ef10998`, migration160, statusok**.
Webb och CLI körda på var sin tom PG16/Redis-rigg med rensad spelmiljö, tick6s.

## SLICE-KONTRAKT
| Fält | Kontrakt |
|---|---|
| Problem | Recall når en enhet; gruppval och delrapport saknas. |
| Spelarsanning | Återkalla egna enheter med fysisk kurir per faktisk plats och namngivna avslag. |
| Invariant | Ingen instant order utom befintligt CommandedInPerson; inget tyst bortfall; gamla events behåller sin tolkning. |
| Scope | Batchval/dispatch, levande order-/passagekedja, CLI/webb/Codex, tester och verblista. |
| Non-scope | Riven legacy, andra orderverb, balans och gamla singelverbens exekveringsfel. |
| Acceptans | Egen leverans per medlem; dubbelordervakt; ALLA N till garnison; samma delrapport i CLI/webb; färsk full Go/vet/JS och riktig klientrigg. |
| Stopvillkor | Saknad platsdefinition till Claude före kod; BILD bedöms senare före merge. |
| Bevisplan | Oförändrad fresh baslinje, namngivna DB-regressioner/mutationer, autentiserad spelarväg på egna riggar. |

## BASLINJE OCH GODKÄND PREMISS
[measure.py](measure.py)/[premise.log](premise.log) kör kanonisk BuildRoute/RoutePositionAt:
två rutter med target(4,0)/start0/end10 står tick5 på (3,0) respektive (4,-1).
Target/ankomstfönster bevisar inte samma plats.
Claude17:15 godkände faktisk interceptionshex + exakt leveranstick, övriga egna
kurirer; 17:28 godkände att kuriren väntar där. 17:43 godkände att NYA gruppverbets
möte och retur ankras på eventets due_tick även när workern behandlar det sent.
Fresh PG16/mig160 baslinje före produktionskod6313d541: recall/orderdelivery/
intercept i handlers/messenger/combat gröna, [baseline.log](baseline.log).

## EXPERIMENT OCH IMPLEMENTATION
| Hypotes/fynd | Ändring | Bevis/beslut |
|---|---|---|
| N gamla recall-events med delad messenger vänder bara första | Behåll gammal claim, nytt `recall_group_member` + atomic medlemsreceipt | `OldSharedMessengerStillOnlyDeliversFirst` låser gamla beteendet |
| Pending event ser inte kurir som väntar på sjöpassage | Pendingvakt inventerar event + ordermanifest; samma unitlock vid enkel/batchdispatch | Dubbelorder, awaiting-passage och samtidiga dispatchprov |
| Gruppens payload måste överleva passage/reboarding | Komplett manifest sparas, alla medlemsevents reconstrueras med generation | `RecallGroup_PassageRebuildsEveryMemberAndRejectsStaleGeneration` |
| Första riktiga webbriggen missade exakt möte genom polljitter | Nytt verb använder due_tick för position, returstart/rutt/ETA | Tre ticks sen handler ger samma resultat; [jitter-tests.log](jitter-tests.log) |
| Retur kan ha identisk ankomst som redan köad kurs | Gruppverbet återanvänder identisk aktiv UnitArrival i samma TX | `ReusesIdenticalPendingArrival`; gamla verb orörda |
| Garnison ensam gav falskt rigggrönt genom expeditionens egen retur | Kräv faktisk MarchRecalled-audit för varje dispatched mottagare OCH garnison | Ny tom webb- och CLI-rigg: två mottagare/två audits/garnison i båda |

Tidiga falsifierade armar: [live-web-before-jitter.log](live-web-before-jitter.log),
[live-web-dedup-before.log](live-web-dedup-before.log). De räknas inte som acceptans.
Servercheckpoint f7157aaf, fyra ytor b6f90185, jitterfix bafd0b7a, dedupfix a6884dc3.
Batchendpoint `POST /worlds/{worldID}/units/recall`: välj `all:true` eller `unit_ids`;
all kan filtreras på target_q/r och intent. Svaret listar groups och varje results-rad.
Första nåbara exakta möte planeras per sparad rutt; gemensam hex/tick delar kurir.

## GRINDAR
| Grind | Utfall |
|---|---|
| Kod | Full `tools/gotest.sh`, färsk PG16/mig160, samtliga paket gröna inklusive world88.6s: [full-go-final.log](full-go-final.log). Full vet grön: [vet-final.log](vet-final.log). JS383/383: [js-tests.log](js-tests.log). |
| Mutation | Fem riktiga källmutationer ger namngivna assertions röda: arrival-dedup, worker-walltime, first-member-only, pending-guard, non-atomic-receipt. Varje arm ny DB, källor återställda, samtliga RecallBatch-prov gröna: [mutations-final.log](mutations-final.log). |
| Semantisk | CLI/webb använder samma all/filter-body och grupper/singlar/alla namngivna utfall; CLI JSON/text + alias, webb action/rendering/escaping och Codex-prov. Inget nytt notiskind. |
| Användare | Register→join→found→två verkliga marscher→recall via riktig webbknapp respektive Keryx. Båda riggar: en kurir, två OrderDelivery, två MarchRecalled-audits, båda garnison, named host-avslag, inga browserfel. [webbproof](live-web/proof.json), [CLIproof](live-cli/proof.json), [CLI duplicate](live-cli/repeat-cli.txt). |
| Visuell | Desktop/mobile-underlag sparat under live-web och live-cli; mobil drawer overflow assertion grön. **BILD ej bedömd/godkänd; Timothy före merge.** |
| Drift | Ej körd; Claude integrerar/deployar efter granskning och BILD. |

## FYND: gamla enskilda recall/redirect vid snabb tick (ORÖRDA)
Riktig rigg tick6s: båda accepterades202 med courier_due_tick1, men handlers kördes
strax efter världens tick2. Båda kraschade sedan på identisk UnitArrival/dedup och
messengerclaim var redan committed: events markerades processed, ingen recall/
redirect-audit. Enheterna återvände senare genom sin expedition, inte ordern.
[legacyproof](live-single/proof.json), [serverhändelser](live-single/server-events.log).
Gamla verb saknar gruppens exakta möteshexvakt och använder walltime/current tick;
just det nya verbets hexavslag finns därför inte där. En lyckad gammal kursändrings
jitter kunde inte isoleras från kökollisionen. Fördröjd andra arm fick ett ärligt
HTTP422/no Runner can catch redan vid dispatch: [delayed.log](live-single-delayed.log).
Claude har begärt FYND, inte fix. Felgrenen i gamla handlern loggar och returnerar
utan OrderFailed; separat åtgärd kräver Claude/Timothy och frozen-event-hänsyn.

## METODISK LÄRDOM
- Tick-exakta möten måste exekveras på schemalagd speltid, inte workerns polltid.
- En marchingfixtur måste också ha sin riktiga ankomst köad; annars döljs dedupfel.
- Automatisk hemresa gör garnison ensam otillräcklig som recall-bevis; kräv orderaudit.

## KÄNDA AVGRÄNSNINGAR
Sjöpassage med okänd fysisk ETA och gamla/osparade rutter använder befintlig
enskild kurirväg; ingen gruppplats gissas. Manifestets passage/generation är provad,
men faktisk batch över vatten grupperas inte. Befintliga singelverbens ovanstående
fel kan därmed kvarstå i dessa fallbackvägar. Gruppen optimerar första nåbara
exakta möte, inte ett senare möte som skulle kunna ge större grupper.
Best-effort notisfanout kan fortfarande tappas vid processkrasch; audit/projektion
är atomic och replay säkert. Ingen ny migration, G1-kant, balans eller karavanändring.

## Resume checkpoint
Bygget och funktionella verifieringen är klara på produktionscommit
**a6884dc32edef35e7649df484f1e70437ef10998**. Rapportens beviscommits följer den;
aktuell grenhash skickas i överlämningen och kan läsas med `git -C` + `rev-parse HEAD`.
Worktree `/tmp/megaron-codex-aggregat-recall-20261007`, gren `codex/aggregat-recall`.
Nästa ansvar: Claude granskar diff/mutationsbevis, tar gamla singelfyndet vidare och
ordnar Timothys BILD före integration. Codex paus efter hashöverlämning; ingen
merge/push/deploy. Vaultens recallparitet/plan/todorad är kirurgiskt uppdaterade.
Reproducerbara verktyg: `tools/recall_mutations.py OUT`; `tools/recall_live.py OUT HASH web|cli|single`.
Inga nödvändiga bevis eller recept finns bara i /tmp.
