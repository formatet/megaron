# Slice 13 — sann svältsignal

## Sammanfattning

`StarvationDamage` följer nu denna ticks faktiska `grain_shortage`-attrition i garnisonen,
även när den sista enheten upplöses. Noll stadskorn efter folkets måltid är ingen svältbevisning.
**Slutlig produktionskod: `8e8cc920`, tvåstadsprov `88797ee0`, gren `codex/sitos-armen`, bas `51e805a4`, migration183.**
Claude granskar/integrerar; Codex har inte mergat, pushat, deployat eller ändrat liveprojektioner.

Timothys senaste beslut (ordertillägg12:30) ersätter tidigare scope: **Sitos tas bort i separat13b före14**.
Denna slice innehåller därför **ingen** DebitArmyGrain, Sitos-debitering, grain_from_sitos,
armé-Sitos-text eller Codex-tillägg. Arméns kornbetalning är som på master. Tidigare försökskod
c2741b17/cf626c78 återtas av8e8cc920; granska **slutdiff mot51e805a4**, inte enbart sista committen.
Faktisk svält med otillgängligt magasinkorn kvarstår tills13b gör ett enda matlager.

Proveniens: läsande live-dump återställd privat i egen PG16, migration183, världstick397;
[source-health.json](source-health.json), [dump-SHA och per-tick-utfall](snapshot-measurement.json),
[recept](measure_snapshot.py). Ingen acceptance-compose användes. Färska DB-testloggar
identifierar commit/migration; privata authdata och dumpen ingår inte i git.

## Slice-kontrakt

| Del | Kontrakt |
|---|---|
| Blockerar | Felaktig svältsignal försvårar korrekt försörjning och fortsatta tillståndsnotiser längs geografi→brist→brons→elit. |
| Spelarsanning | Audit påstår garnisonssvält bara efter faktisk förlust denna tick; även sista förbandets förlust ska synas. |
| Invariant | Upkeep är ensam attritionsägare; audit ändrar inte pop/lojalitet/kharis/units/goods. Historiska events ändras inte. |
| Scope | Pre-loss-garnisonsläge additivt i UnitAttrition, auditpredikat i kharis, verkliga regressions-/mutationsprov och mätning. |
| Utanför | Reservdebitering/nya Sitos-texter,13b borttagning/migration,14 notiser,15 texter,16 rivning. |
| Acceptans | Semantisk masterRED för falsk audit; riktig upkeep→kharis samma tick ger signal vid100 och1 man; inga gamla utfall räknas som aktuella. |

## Baslinje

Färsk privat snapshot, världstick397, migration183. I fönstret **324–385**:

| Stad | Utfall | Slutsats |
|---|---|---|
| Phaistos | 62StarvationDamage,62UpkeepSettled,62SitosGranaryStored, **0UnitAttrition**; båda spearmen100 vid snapshot397 | 62 falska audit-/krönikeposter, inte62 belagda förlusttick. |
| Scheria | 4UnitAttrition(reason grain_shortage), förlust6+6+1+5, tick352–355 | **18 faktiska förlorade män**; separat försörjningsproblem löses av13b, inte denna auditfix. |

Snapshot397: Phaistos Sitosgrain441,70/fish445,40; Scheria grain326,64/fish23,76.
Mätningen stöder att korn kunde ligga otillgängligt i magasinet. Snapshot397 är inte samma tick som385;
gamla events är utfall, ingen komplett projektion-/rathistorik. Historiska enheters stadsrelation i
mätningen använder nuvarande home/payer/settlement. Inga förluster eller historiska rater gissas fram.

Gamla kharisgrinden läste grain0 **efter** folkets FoodTick, trots betald armé.
[baseline.log](baseline.log): riktiga gamla produktionsfilen51e805a4 ger
`TestStarvationSignal_NoFalseAuditAfterPaidUpkeep` **namngivetRED**, inte kompileringsfel.
Recept [baseline_signal.py](baseline_signal.py) återställer exakta bytes i finally.

**Gjorde de falska posterna annan skada?** Fysisk gammalkod lägger1StarvationDamage men
hela före/efter-hashar för settlement(pop), realm(kharis), goods, units och loyalty_events är
identiska. Ny kod lägger0falska poster och har också identiska projektioner.
[audit-before.log](audit-before.log), [audit-after.log](audit-after.log),
`TestStarvationSignal_AuditHasNoStateEffects`.
[Produktionsinventering](starvation-readers.txt) hittar bara emissionen i kharis och läsaren i chronicle för typen;
ingen pop-/lojalitets-/kharisprocessor eller DB-trigger konsumerar den. Krönikeraden hävdade
felaktigt hunger. Alltså falsk audit/flavour, ingen belagd modellskada **från de62posterna**.
Gamla event/krönikor skrivs inte om och inga kompensationer görs. Detta bevisar inte att staden aldrig hade andra problem.

## Experiment och slutlig ändring

| Hypotes/prov | Resultat | Beslut |
|---|---|---|
| 62auditposter betyder62 verkliga förlusttick | Motsägs av verkliga utfall och oförändrade projektioner | Audit följer faktisk attrition. |
| Nollkorn efter folkets måltid bevisar armésvält | MasterRED | Ta bort balanspredikatet. |
| Levande förband räcker som historisk hemvist | Sista mannen kan vara upplöst före kharis | Spara pre-loss-garnisonsläget i utfallet. |
| Behåll separat magasin och debitera det | Tidigare implementation överges efter nytt Timothybeslut | Ingen sådan kod/text i slutdiffen;13b följer separat. |

`UnitAttrition` behåller befintlig förlustsemantik och får additivt `garrison_settlement_id` för
pre-loss-garrison spearman/war_chariot. Kharis kräver samma värld, aktuell tick och grain_shortage
med detta registrerade stadsläge. Ingen JOIN/EXISTS mot levande förband. Gamla event utan fält
omtolkas inte retroaktivt. `StarvationDamage.reason=no_food` och auditens krönikerendering är oförändrade.

**Positivt verkligt livscykelprov:** `TestStarvationSignal_ActualLossEvenLastGarrison` kör riktig
UpkeepHandler följd av riktig KharisHandler, både100 och1 man. Båda fallen har också en andra stad B i samma värld med en matad100-mannagarnison;
A får1audit, B får0 och behåller100 utan förlust. I1-fallet är enheten disbanded och
dess settlement_id rensat före konsumenten. Aktuellt förlustutfall ger ändå exakt en auditpost.
Negativt prov seedar en tidigare ticks riktiga förlust, noll stadskorn och kvarvarande garnison:
inget gammalt utfall får räknas som dagens svält. Upkeep-/attritionskostnader ändras inte.

**Produktionsordning:** `cmd/server/main.go` registrerar båda handlers och kör en `worker.Run`.
`events/scheduler.go processBatch` claimar `ORDER BY due_tick,priority,id`, sorterar samma nycklar och
dispatchar synkront. Upkeep50 före kharis60. Befintliga riktiga PG-prov
`TestProcessBatch_DispatchesInTickPriorityOrder` och `...ClaimsHighestPriorityAcrossBatchBoundary`
är gröna tillsammans med positivt faktiskt-förlust-prov i [final-lifecycle.log](final-lifecycle.log).
Detta bevisar normala lyckade handlers i samma tick, inte att ett misslyckat upkeep automatiskt spelas om före kharis.

## Grindar och fyra ytor

| Grind/yta | Slutbevis |
|---|---|
| Server | Faktisk garnisonssvält signaleras utan nya skador; enda mekaniska utfallsutökning är additivt pre-loss-läge. |
| API/CLI/webb/Codex | Ingen ny spelartext, operation, API-reservformel eller notiskind i denna krympta slice. Sitosytorna kvar på master tills13b;14 äger tillståndsnotiserna. |
| Kod | Full färskPG16/183-Go [final-go.log](final-go.log), vet [vet.log](vet.log),654JS [js.log](js.log); fullGo/vet/JS på8e8cc920, därefter enbart teständring88797ee0 med nytt färskt riktat livscykelprov. |
| Mutation | Fyra fysiska kompilerande namedRED→exakt-byte-restore→färskGREEN: förra ticks svält, saknat pre-loss-läge, beroende av kvarvarande garnison och svältsignal till fel stad. [mutations.log](mutations.log), [recept](mutations.py), enskilda red/restored-loggar. |
| Semantik/användare | Verkliga UpkeepHandler→KharisHandler och appendade utfall, ingen fabricerad 500 eller tillskriven historisk förlust. Nya visuella/textytor utgår; därför inga screenshots för denna slutdiff. |
| Drift | Ingen migration, omräkning eller deploy av Codex. Claude granskar/integrerar och verifierar körande hash. |

## Metodisk lärdom

- Skilj flavour-event, faktisk förlust och projektionens skrivare innan skada tillskrivs eventet.
- Registrera förlustens hemvist i utfallet; kvarvarande levande förband är ingen säker historik.
- Utvärdera slutdiffen efter ett ändrat beslut; övergivna prover är inte bevis för den levererade funktionen.

## Kända avgränsningar

Ingen försörjningsfix i denna slice: armén äter fortfarande enbart stadens korn enligt tidigare väg.
Reservflytt/borttagning väntar13b; start/slut-notiser väntar14. Historiska falska audit-/krönikeposter kvarstår.
Ingen retroaktiv kompensation, full projektionreplay eller nya bild-/textbedömningar.
