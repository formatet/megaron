# Reträtt — två lägen

Reträtt väljs nu med **Hold to the last man** eller **Retreat when it goes badly** på alla fyra ytor. Procentval vägras i API före dispatch och finns inte i webb/Keryx. Migration173 normaliserar lagrat tillstånd; gamla brev lämnas orörda och tolkas först vid ankomst, med **hold:true företräde** även när procent finns bredvid.

Gren `codex/retreat-tva-lagen` från **2b83b1f6**, worktree `/home/tk/wt/codex-retreat-tva-lagen`. Inventering **c9e6a780 före kod**, server **e97191db**, legacy reset **ca0e5eea**, fyra ytor **962d0ef3**, granskningens hold-fix **939c9d9a**. Claude granskar/integrerar; ingen merge/push/deploy från Codex.

Bevis: färsk Go/vet/598 JS, sju distinkta mutationsmål, normalt web-/Keryx-/API-flöde utan SQL-skrivningar, Firefox1280+390. [Samlat bevis](verification.json). BILD i bunten; ingen särskild merge-grind enligt text/knapp-ordern.

## Kontrakt

| Fält | Kontrakt |
| --- | --- |
| Problem | Fasta procenttrösklar och fem UI-val gör reträtt mer komplicerad än beslutad kanon. |
| Spelarsanning | Två lägen: Hold to the last man / Retreat when it goes badly (= by_loyalty, standard). |
| Invariant | API tar bara dessa lägen; gamla lagrade fasta trösklar blir by_loyalty. Stridsmotorns sideRouts ändras inte. |
| Scope | Requestvalidering och lagring, en ny additiv datamigration, Keryx, webb, Codex/verblista, prov/rigg. |
| Non-scope | Stridsbalans, routing, ny per-unit pre-battle-inställning, offer-slicen. |
| Acceptans | Förbjuden retreat_at_loss → begriplig400 + mutation; migration normaliserar legacy och lämnar hold/övriga data; färsk full Go/JS/vet; Firefox1280+390 och normalt användarflöde. |
| Stopvillkor | Oklart hur gamla redan avsända bud ska migreras avstäms med Claude; inga historiska utfalls-event skrivs om. |
| Bevis | Inventering/baslinje först, persistent migrationsprov, HTTP/domänprov, mutation, faktisk webb/CLI/API. |

Kedjegrind: bevisar nåbar och begriplig stridsorder, utan ny brons-/elitmekanik.

## Inventering före kod

| Lagringsyta | Form/ägare | Åtgärd |
| --- | --- | --- |
| player_world_records.retreat_default | JSONB från migration145. {} = loyalty; retreat_at_loss = fast andel KVAR; hold_to_last_man = håll. Skrivare combat.SetRetreatDefault; kopieras till ny battle_participants via seedStandingOrdersSQL. | Ny additiv migration173 ersätter aktiva fasta trösklar med loyalty. Aktiv hold bevaras även med en gammal ignorerad procentnyckel bredvid. Nya defaults endast två lägen. |
| battle_participants.standing_orders | JSONB från migration114. Endast här bor per-unit reträtt, aktiv strid. Skrivare combat.SetStandingOrders (garrison direkt, fält via Runner), läsare battle.sideRouts. | Migrera aktiva legacytrösklar, bevara hold/andra nycklar; setter ersätter modefälten atomiskt. sideRouts/legacyengine-fixturer orörda. |
| scheduled_events.payload.standing_orders | Pending ScheduledOrderDelivery bär StandingOrdersOrder med Go-fältnamn RetreatAtLoss/HoldToLastMan; levererar till samma setter. | Orörd bytevis. Besked Claude08:39: Ett ställe vid tillämpning efter ankomst: hold:true vinner; annars legacyprocent/gammal hold:false-reset → loyalty. Brevet ändras inte. |
| messengers.order_payload | Beseglat brev sparar motsvarande order för passage/rekonstruktion och förlustytor; nyckelform verifieras mot dispatchkod. | Orörd, även under passage. Samma översättning vid tillämpning täcker rekonstruerade pendingbrev; historiska utfall lämnas. |
| events.payload | Append-only utfall StandingOrdersChanged och audit; inte stridens aktiva projektion. | Orörd historik, äldre procentfält förklarar det som faktiskt hände. |
| standing_orders-tabellen | Migration140: återkommande varutransport, inte battle retreat. | Orörd — samma namn men annan mekanik. |
| units | Ingen retreat-kolumn; unit-kort får in_battle, inget nytt förhandsval. | Orörd datamodell. |

API: realm GET/PUT retreat-default (handlers/retreat_default.go); unit POST standing-orders (unit.go1834). Per-unit validering måste ske **före dispatch**, inte bara efter att budet kommit fram. Den gamla settern tillät partiella samtidiga modefält; två lägen blir ett uttryckligt val.

Webb: ui/retreat.js delar presets/body/defaultlabel/realm/per-unit. war.js använder samma body; per-unit visas bara in_battle. Keryx retreat-default har tre mode-flaggor, unit retreat-order saknar by-loyalty och accepterar partiell procent+hold. Codex battle.md och stances.md beskriver procentval. Dessa fyra ytor ändras i samma slice.

[DB-baslinje](baseline-db.txt) kommer från isolerad rigg, **inte produktion**: åtta realm-rows {} och inga battle participants. [Process](baseline-provenance.txt) är cdfe6ab/mig172 och har både äldre aktiv värld och arkiverad BILD-värld; auto-provenance väljer den nyaste arkiverade. Den är inte master2b83b1f6-processen. Källbaslinjens färska Go/JS körs separat.

## Grindar

| Grind | Bevis och praktisk gräns |
| --- | --- |
| Kod | [Full färsk Go/mig173](full-go.log), [vet exit0](vet.log), [598/598 JS](full-js.log). Ingen ändring av combat/battle.go eller sideRouts. |
| Migration | TestRetreatTwoModesMigrationNormalizesLegacyPreservesHoldAndLetters kör SQL mot legacyrader: aktiv procent → loyalty, hold/default oförändrade, hold+ignorerad procent → hold, andra JSONnycklar kvar. Brev och pendingpayload byteidentiska. |
| Semantisk | Samma hold-företräde vid migration och leverans. Setters ersätter hela läget atomiskt; realm-default påverkar nya strider, unit-override denna strid. Båda API:erna vägrar retreat_at_loss inklusive null. |
| Användare | Normalt ParityNomad-login → War → välj/Save båda lägen → läs verklig API → Keryx sätt/läs båda → gammal flagga och PUT vägras → originaldoctrine återställs. [Fakta](after/facts.json), [Keryx](after/keryx.txt), [logg](after-proof.log). Inga SQL-skrivningar, inga syntetiska browserpayloads. |
| Bild | Fyra efterbilder för båda lägena i Firefox1280+390, två förebilder. Valet ryms i viewport, inga JS-fel. Bilder tas efter drawer-animationens slut; ingen layout-/CSSändring. |
| Process | [Runtime](rig/runtime.json), [provenance](rig/provenance.txt), [schema173 dirty=false](final-schema.txt). CGO0 lokal Go på cachad isolerad acceptance-runtime, current migrations + worktree-webb; ingen clean Dockerbasebuild. Retreat-/war-assets bytejämförda och SHA finns i after/facts.json. |

## Mutationer och leveransprov

[Servergrindar](server-mutations.log): domainpercentage, realmpercentage, unitpercentage före courier samt legacyankomst. [Slutliga legacygrindar](legacy-server-mutations.log): översättning bort, false-reset bort, hold felaktigt bort. [Webbgrind](client-mutations.log): procent återinförd i requestbody. **Sju distinkta felmål** (legacyankomst har körts i båda serverbuntarna); varje mål gav namngivet rött och byteåterställd grön. [Återställd server](restored-server.log), [återställd webb](restored-client.log). Full Go/vet kördes efter slutlig hold-fix och återställda mutationer; full JS är grön mot JS-källan som är oförändrad sedan962d0ef3.

TestStandingOrders_FieldUnitOrderTravelsByCourierAndAppliesOnlyToActiveBattle provar de tre gamla brevformerna **procent**, **hold:false-reset**, **procent+hold:true**. Innan varje leverans sätts participant-läget till motsatsen av väntat resultat, så ett vägrat bud inte kan bli falskt grönt. Ankomstresultat och byteidentiska brev/queuepayload läses ur riktig test-DB. Samma tillämpningsgren används av passageåterskapade envelopes. Ingen ändring av events-historik, pendingbrev eller redan publicerade migrationer.

Migrationens down är avsiktligt en no-op: borttagna procentvärden går inte att återskapa. Hold och loyalty är redan giltiga för föregående motor, så rollback hittar inte på historiska värden. Setterprov täcker clearing av gamla procent/hold medan andra nycklar bevaras.

## Firefoxbilder

| Val | 1280 × 900 | 390 × 844 |
| --- | --- | --- |
| Före | [bild](baseline/desktop-baseline.png) | [bild](baseline/mobile-baseline.png) |
| Retreat when it goes badly | [bild](after/desktop-loyalty.png) | [bild](after/mobile-loyalty.png) |
| Hold to the last man | [bild](after/desktop-hold.png) | [bild](after/mobile-hold.png) |

Förebilder kör **cdfe6ab/mig172**, den verkliga äldre riggprocessen, vars produktions-UI matchar basmaster2b83b1f6. Efterbilder kör slutliga produktionskärnan **939c9d9/mig173**. Browserns mode/API och asset-SHA redovisas i respektive facts.json; sista beviscommit ändrar endast docs/loggar/bilder.

## Baslinje, reproduktion och avgränsningar

[Bas-Go](baseline-go.log) är färsk DB/mig172 på oförändrad2b83b1f6; [bas-JS](baseline-js.log) är598 gröna. Riggdatans realm/participant-inventering är separat. Den tidiga queued_delivery-räknaren använde fel eventliteral ScheduledOrderDelivery i stället för lagrat OrderDelivery och används **inte** som queuebevis; slutlig korrekt kontroll finns i final-schema.txt. Legacybeteendet bevisas av de persistenta leverans-/migrationsfixturerna.

[Verktygsindex](../../../tools/README.md) anger basbild, ombyggnad, normalt användarflöde och mutationer. Kör mutationer sekventiellt före fullsviter. Proof-scriptet återställer realm-doctrine i finally, inga nya världar, enheter eller strider skapas.

Verklig spelarinteraktion bevisar realm-valet. Per-unit-stridsvalet bevisas av kort-/bodyprov, riktig test-DB och fysisk courierleverans; detta bildscenario skapar inte en naturlig strid. Prosan som beskriver reträttvalet är borttagen; övriga War-kontroller/Lawagetas-rutan är oförändrade. Ingen ny pre-battle per-unit-inställning, ingen stridsbalansändring, ingen offerslice.

Metodisk lärdom: migrering och ankomsttolkning måste dela den gamla aktiva betydelsen; en felöversättning kan annars ändra en uttrycklig hold-order. Brevets bytes och stridens projektion är olika lager och ska provas var för sig.

**HASH + STOPP:** redo för Claudes kärngranskning/integration när samlat slutbevis är committat.
