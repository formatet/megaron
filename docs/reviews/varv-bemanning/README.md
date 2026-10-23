# ORDER28 — implementation av godkänd binär modell

**Aktuellt kontrakt (Claude 2026-10-23 06:03):** minst EN bemannad plats ger full befintlig takt åt ALLA parallella bygge-/reparationsjobb; noll pausar utan progress eller ETA. Betalda order och restarbete består. Dagens kapacitetsgränser kvar, inklusive att gamla TrainComplete-cap även räknar landutbildning. Det äldre förslaget nedan om halvfart är FÖRKASTAT.

Scope: verklig placement-service, sparat tickarbete, atomisk reglering vid alla förändringar, deployövergång samt server/Keryx/webb/Codex. Invariant: ingen obemannad progress, inget dubbelt materialuttag eller färdigställande. Fitting_out/landutbildning ändras inte. Acceptans: noll/en, paus/återuppta, flera samtidiga, livscykel, övergång+replay. Stoppa vid kanonändring. Bevis: separat original-master/fresh203 endpointbaseline, fresh204 riktade tester, fysisk nollbemanningsmutant, fyra ytor i isolerad rigg, EN full fresh vid slutet+JS/vet.

---

# ORDER 28 — inventering och modellförslag

2026-10-23. Bas: `7f0cdebe`, gren `codex/varv-bemanning`.
**Steg 1 endast. Ingen spelkod, migration, körning av rigg eller liveändring. Modellen nedan väntar på Claudes godkännande.**

Kanon: `temenos_arbete.md`, ”Stadens arbetsplatser utan produktion” (Timothy 2026-10-22), samt `megaron_todo.md` SENARE/Byggbart utan beslut punkt 1. Obemannat varv bygger och reparerar inte. Skalan är ännu inte beslutad.

## Kodmätt nuläge

| Del | Beteende och källa |
|---|---|
| Arbetsplatser | `server/internal/economy/recompute.go:99`: `workplaceSlotTable["shipyard"] = {0,3,6,10}`. Varvet är uppgraderingsbart (`province/building.go:135`). |
| Verklig bemanning | Ingen varvsbemanning används av bygg- eller reparationsvägen. `economy/placement_yield.go:392` hämtar stadens arbetsplatser via `production_rules`; varvet saknar sådan regel (`province/building.go:131`). Endast tempel läggs till separat som tjänstearbetsplats, `placement_yield.go:462`. Tabellens varvsplatser innebär alltså inte att spelaren redan kan bemanna dem genom nuvarande meny. |
| Bemanningsgränssnitt | `server/cmd/keryx/cmd_staff.go:56–130` använder placement-options och kräver en roll i byggnadens Goods-lista. En separat varvstjänst måste införas genom hela placeringslivscykeln; ingen låtsasproduktion av skepp som lagervara. `settlement_placement.good_key` har FK till goods (`db/migrations/120_settlement_placement.up.sql:33`), vilket också måste hanteras i modelleringen. |
| Skeppsbygge | `server/api/handlers/province.go:2307`: kräver att varvsbyggnaden finns. Ingen nivå- eller bemanningsfaktor. Varje fartyg skapas som naval/forming med `build_complete_at` och ett atomiskt köat TrainComplete (`:2548–2599`). Alla i en beställning får samma sluttick: detta är parallella timers, ingen seriell arbetskö. |
| Bygggräns | `province.go:2224–2245`: högst tio väntande TrainComplete per stad enligt scheduler-queryn. Queryn filtrerar INTE på naval och kan därför även räkna landutbildning; detta är ett befintligt avvikelsefynd, inte en avsiktlig ny regel. |
| Grundtider | `province/training.go:110–113`: galley 3, war_galley 5, merchantman 4 världsticks. Material och besättning dras vid beställning. |
| Byggslut | `combat/train.go:102–119`: forming→garrison och ETA nollas. Kontrollerar varken varvets existens eller bemanning vid färdigställandet. Statusvillkoret skyddar mot dubbelt färdigställande. |
| Reparation | `api/handlers/unit_repair.go:84–114`: kräver varv och räknar pågående repairing mot 3/6/10 platser. Detta är reparationsplatser, inte bemannade arbetare. Skeppsbyggen delar inte denna gräns. |
| Reparationsstart | Material betalas i förskott, status repairing och fast ETA sätts (`unit_repair.go:115–154`). Därefter schemaläggs slutet UTANFÖR transaktionen (`:157`): fel där kan lämna ett betalt jobb utan timer. Vid kommande ändring bör jobb och schemaläggning vara atomiska. |
| Reparationstid/slut | `combat/ship_repair.go:47,94`: ett tick per saknad skrovpoäng. Sluthandlern återställer hela skrovet och status, utan varvs-/bemanningskontroll (`:153`). Ingen separat lagring av partiellt reparerat arbete. |
| Avbrott | `combat/cancel_training.go:84–102` kan avbryta forming/training och återbetala reservationen. Ny arbetsstatus får inte bryta denna väg. Repairing omfattas inte av den avbrytningen. |

## Vad deploy innebär

Nuvarande sparade jobb består av enhetsstatus, fast ETA och schemalagt slutevent; det finns ingen bemanningsberoende restarbetsmängd. En ren omstart behåller dessa timers. Att bara kontrollera bemanning när en ny order läggs gör gamla jobb oberoende av bemanning och tillåter att personal tas bort direkt efter beställningen. Att bara kontrollera vid sluttiden kan inte rekonstruera hur mycket arbete som utförts däremellan.

Övergången måste därför uttryckligen bevara betalning, besättning och redan utfört arbete och göra även gamla jobb pausbara. Föreslagen cutover vid ett sparat världstick: återstående normalt arbete = max(0, gammal due_tick − cutover_tick), per befintligt aktivt jobb. Jobb som redan är förfallna hanteras exakt en gång före/vid cutover; de får inte återuppstå som nya jobb. Från cutover ackumuleras arbete enligt nya regeln. Ingen omstart av hela byggtiden, ingen ny debitering och ingen automatiskt tilldelad arbetskraft.

Enhetsjobb och gamla schemalagda slut måste konverteras tillsammans så att gamla TrainComplete/ShipRepairComplete inte senare kör förbi pausen. Landutbildning och fitting_out är egna flöden och omfattas inte av denna order. Rivet/saknat varv ger noll takt; återbyggt varv kan återuppta kvarvarande jobb. Tid och prognos ska använda befintliga clock/tick-ägare.

Detta är kodbaserad övergångsanalys, ingen uppmätt inventering av antalet livejobb. Haiku-riggen har inte lästs eller ändrats i denna order.

## Föreslagen generös alfamodell — för godkännande

**Gemensam taktfaktor för varvet: `min(1, bemannade / 2)` på alla tre nivåer.**

| Bemannade | Takt per pågående jobb | Exempel: galär, 3 normala arbetsdygn |
|---|---|---|
| 0 | paus | ingen sluttid |
| 1 | 50 % | 6 dygn vid oförändrad bemanning |
| 2 eller fler | 100 % | 3 dygn vid oförändrad bemanning |

Två av tre platser ger full takt redan på L1. Samma absoluta krav vid uppgradering undviker att ett större varv plötsligt sänker takten för samma arbetare. Extra bemanning över två ger i detta förslag ingen ytterligare fart; detta måste sägas öppet i gränssnittet. Högre nivå behåller nuvarande extra reparationsplatser. Om varje extra arbetare ska ge mer kapacitet krävs i stället ett annat godkänt kontrakt.

**Parallellitet i detta förslag:** behåll befintliga gränser (byggbeställningar upp till nuvarande bygggräns, reparationer 3/6/10), och använd taktfaktorn på varje jobb. Detta är medvetet generöst: två arbetare kan driva flera parallella jobb. Det är en faktor på dagens beteende, inte en ny gemensam arbetsbudget. Claude behöver godkänna detta uttryckligen. En gemensam budget för bygge+reparation som delas mellan alla jobb är mer ekonomiskt strikt, men innebär också en separat förändring av ködisciplin, genomströmning och prioritering. Rekommendation: inför inte den samtidigt utan beslut.

Lägg order på obemannat varv som **betald, pausad order** med tydlig kvittens och arbetskraftsbehov, inte som påbörjat arbete med falsk ETA. Avsikten består tills spelaren ändrar den. Bemanningsändring reglerar först arbete fram till ändringens tick med den gamla takten, sedan gäller den nya. Befolkningsförlust, avmobilisering av placeringar, rivning och byte av ägare får inte lämna gammal takt i drift. Samma jobb ska automatiskt fortsätta efter bemanning; inget förnyelseklick.

## Nästa kodsteg efter godkännande

- Serverägd, sparad restarbetsmängd och beräkning av takt/paus/prognos; transaktionssäker livscykel och idempotenta slutevent. Enbart ny due-tid vid gammalt slutevent duger inte.
- Riktig tjänstearbetsplats i placeringsmodell, kapacitet, roster, idle-pool och rivning/reconcile. Följ FK och rollrepresentationen; återanvänd inte kultrollen för varvet.
- Server/API, webb och Keryx visar bemannade/platser, effektiv takt, pausorsak och villkorad ETA för bygge OCH reparation. Uppdatera även rekryteringsmöjligheter, orderkvittens och transporthjälp som visar reparations-ETA.
- Codex-artikel förklarar samma regel, parallellitet och återupptagning. Inga nya rutinnotiser behövs för varje tick.
- Bevis efter kod: noll/halv/full takt; ändring mitt i jobb; gammalt jobb över cutover; flera samtidiga bygge+reparation; rivning/återbyggnad; cancellation/refund; omkörning av slutevent; oförändrad landutbildning/fitting_out. Egen fysisk mutation av nollbemanningsgrinden, sedan full fresh svit EN gång samt JS/vet och fyra ytor enligt arbetsordern.

**STOPP efter inventering. Inga tester eller mutationer körda: endast denna rapport är ändrad.**
