# Hex för hex — A, enhetsresor och passagerare

## SAMMANFATTNING

En sen avgång räknades från tickens början; skeppet kunde därför ritas på målhexen direkt, och annonsens ETA skilde sig från ankomstjobbet. Nya versionerade enhetsresor har en sparad faktisk avgång, kadens och proportionella prefixgränser som styr position, sikt, risk, passage och deadline. Den avrundade totala restiden ändras inte.

**Kod:** `8c4c6073` → `fe078375` → `ab7fd51d` → `ced237c5` → `9fb2c8b8` → **`ae5988c8`** på `codex/skepp-omedelbart`. Ingen merge/push/deploy. **BILD:** [åtta nativebilder](index.html), Firefox1280×900/390×844, granskade1:1. Health **ae5988c8 / mig213**,944 produktionshashar matchar slutkoden. Ägd privat rigg; acceptansens provenance-kommando är inte tillämpligt, motsvarande full proveniens finns i [proof.json](bild-handover/proof.json).

**Test:** slutlig färsk fullGo på ae5988c8: **samtliga26 paket gröna** (API62.109s, world142.345s),0 röda.769JS, vet och fyra day_guard gröna. Fyra fysiska mutanter assertion-röda, exakta återställningar och färskt grönt prov. Två ytterligare kö-/övergångsregressioner rött→grönt.

## SLICE-KONTRAKT

**Bevisar kedjegrindens rörelseyta.** Timothy/Claude valde A: behåll avrundad total; på tre lika steg under1 tick nås hexarna vid1/3,2/3,1 efter faktisk avgång. Starthexen behålls till första gränsen; sista hexen och ankomstdeadlinen sammanfaller. Gamla sparade rutter och event behåller sin tolkning.

**Avgränsning bekräftad av Claude09:52:** A är enhetsresor och deras passagerare. Fraktens `TradeJourney` och budets egna landben får nästa slice, på samma klockägare. Ingen sådan migrering eller wikiombyggnad ingår här. Varvet7ad899ac orörd.

## BASLINJE

Initial read-only utredning och ursprungsloggar: [rapport](initial-investigation.md), [Go-repro](go-repro.md), [inventering](hex-for-hex-inventory.md). Historiska originalrutten är överskriven; rapportens trestegsrutt är uttryckligen en rekonstruktion. Ingen prodskrivning gjordes.

Ursprungliga bevis: `af3c1d75` JS och `42a8d4ff` riktig StartMarch på färsk DB. Avgångsprovet är nu anpassat till den nya uttryckliga starthexregeln, med gränsavgång, rapportens+44s och en tickarbetare som ligger104s efter. Det kontrollerar även liveposition medan världens tickkolumn ännu inte uppdaterats, prefixminne, ETA och gamla redan hämtade jobb.

## EXPERIMENT OCH RESULTAT

| Problem | Slutlig ägare/ändring | Bevis |
| --- | --- | --- |
| Avgång före ordern, skilda klockor | `movement.StoredRoute` v2 sparar dispatch-offset, absolut avgångsankare och kadens; normaliserade kostnadsprefix behåller restiden. `tick.Anchor` konverterar initialt/vid historisk batchläsning. | `TestShipReport43640c42_DeparturePhase`, `TestTimedRoutePrefixFromActualDeparture` |
| Tickarbetarens poll klampar kartan | V2 läser ruttens sparade ankare, oberoende av världstickens pollfas. Go-position, API/CLI, bärare och webben använder samma fönster. | live-position-delfallet och faktiskt Firefox-/Keryx-prov |
| Framtida rutthexar i minnet | Befintlig PlaceReach-progress sparar varje nådd v2hex i tidsordning, även utan namnrätt. V2 hoppar över UnitArrivals gamla omsökning/helaruttsvep. Befintlig ögonradie gäller fortfarande: synlighet från ett aktuellt öga är inte samma sak som att redan ha passerat en rutthex. | prefix_memory-delfallen före/vid varje gräns; `TestTimedAdvanceStopsBeforeFutureHexMemory` |
| Mellan-tickskorsningar | Svepta `movement.Presence`/`FirstContact`-intervall, stabil tidsordning och omräkning efter stopp; kvarstående enhet får inte kontakt längs sin övergivna framtida rutt. | `TestTimedEncounterCatchesCrossingBetweenTickSamples` och integrerat stopp-/siktprov |
| Del-tick, storm och retry | `last_milli`/`last_storm_id` för v2; samma inträdesgränser, inget framtida väderkontrollsteg och ingen dubbelträff under sammanhängande vistelse. | `TestTimedStormContactsDoNotReachFutureHexes`, befintlig stormsuite, cutoff-mutant |
| Passage | Buren position/prefix läses från skeppet, samma ETA; versionerade deadlinejobb för budet när bäraren har v2klocka. | hela API-passage/pickup- och messengerpaketen, Keryx aktuellt läge |
| Ankomst och ersatta kurser | `UnitArrivalV2` har deadline+exakt rutthash. Köidentitet och rutt kontrolleras under lås. Endast ohämtad identisk legacyrad kan ersättas med ny typ; hämtad rad behåller identitet och får inte verkställa v2. | två nya stale/claimed-delfall rött→grönt, befintlig single-recall-suite; `TestDeadlineEventsWaitForSavedInstantAndOrderWithinTick` |

### Varför inga per-hex-jobb behövs

Befintliga världsscaner och v2ankomstjobbet kör en gemensam gränsvis catch-up. Den sorterar nådda rutt-/vädergränser fram till aktuell tid, minns vad ögat nått och behandlar storm/kontakt/interception vid gränsen. Efter stopp laddas ruttägaren på nytt. Transaktionsbundna minnes-/kontaktclaims, stormprogress och världsprogress gör retry säker; första körningen börjar tidigast vid första v2avgången. Därmed tappas inte flera hexar som passerats mellan två poller, samtidigt som inga nya beständiga jobb skapas per hex.

**Deadline är gemensam, inte ett löfte om noll I/O-latens:** DB-verkställning sker vid första behöriga befintliga workerpoll. Slutriggen visar därför både målhex nådd och separat verkställd ankomst. Ingen ankomst är behörig före den sparade deadlinen. Positionens precision är den befintliga tusendels ticken.

### Event och data

Nya typer: `UnitArrivalV2`, samt `OrderDeliveryV2`/`MessengerArrivalV2`/`MessengerReturnV2` för passage med v2bärare. Gamla eventtyper omtolkas inte. En identisk, ännu ohämtad legacyköpost kan atomiskt ersättas med den nya typen när kursen byts; redan hämtade poster lämnas kvar och kan inte påverka den nya kursen. Utfallsnotiserna för storm/strid behåller sin innebörd.

Mig213 lägger endast till nullable stormprogress och `timed_route_progress`; gamla rutter konverteras inte. [Offline schema-prov](schema/proof.json):212→213→212→213, gamla `last_tick=42` bevaras, dirty=false.

## GRINDAR OCH PROV

| Grind | Resultat / artefakt |
| --- | --- |
| Kod | [Slutlig fullGo](hex-full-go-release.log), [vet](hex-vet-handover.log), [769JS](hex-js-final.log), [schema](hex-schema-final.log); fyra `hex-day-*-handover.log` |
| Mutation | [Fyra assertion-röda armar och restore](mutations-final/proof.json), [färskt återställt grönt](mutations-final/restored-green.log). De tre muterade produktionsfilerna matchar också slutkoden; senare hardening av köövergången har separata rött→grönt-prov. |
| Övergång | [Stale legacy-rött](hex-stale-legacy-red.log), [hämtad identitet-rött](hex-claimed-identity-red.log), [båda gröna + recall](hex-claimed-transition-green.log); [hela berörda domänpaket](hex-transition-green.log), [hela API](hex-transition-api-green.log) |
| Visuell | [A1–D2](index.html):8 native Firefoxbilder, alla granskade1:1,0 sidfel, ingen CSS-/terräng-/pixelstiländring. |
| Användare | Verklig registrering/join/grundning och marsch; uttrycklig lokal sjöleds-/skeppsfixtur.4 API- och Keryx-faser; faktisk klocka, inga mockade API-svar eller DOM-ersättningar. [FixtureSQL](bild-handover/fixture.sql) |
| Drift | Ingen prod/acc-ändring, merge, push eller deploy. Egna riggar och DB-containrar städas i `finally`. |

### Öppet redovisade tidigare körningar

Första fullGo hade gamla testförväntningar på valA/legacyevent. Fixturerna läser nu faktisk v1/v2typ och dess passagegeneration/deadline; viktad ford jämförs vid80% när forden nåtts, inte vid30% då nya kontraktet står kvar före den. Recall-jobb behåller separat kontroll för andra envelopes. Tidiga egna formatterings-/fixturekompileringsfel är inte använda som regressionsbevis.

Blandad combat/events/messenger→API-körning visade kända kvarlämnade varvsjobb utan deadline samt kolliderande testnamn mellan paket. Ingen varvskod ändrades; API har körts på egen färsk DB och hela slutkörningen börjar med API på tom DB. Full redovisning finns i de tidigare `hex-*.log`-filerna.

Bildförsök1 ändrade map_tile men saknade motsvarande grundningsprovince; försök2 gav skeppet100 proviant utan matledger. Båda var egna fixturfel. Försök3 fångade den riktiga tickfas-clampen vid liveframfart. Den nya proviantfixturen börjar tom och lastas av riktig marschlogik. Giltigt slutbevis är enbart `bild-handover/` på ae5988c8; dess944 hashar matchar slutproduktion. Tidiga schemaförsök träffade PostgreSQLs temporära initserver; slutverktyget väntar på nätverksservern.

## METODISK LÄRDOM

- Bevisa faktisk tickfas i en levande rigg; en fryst TestClock vid tickgränsen missade både avgångs- och pollproblemet.
- En hämtad köpost har egen identitet: att byta dess typ under en gammal workers fötter kan låta den skriva bort ersättningsjobbet.

## KÄNDA AVGRÄNSNINGAR / NÄSTA SLICE

**Överlämningsrad enligt Claude09:52:** fraktens `province.TradeJourney` → `transport.prepareJourney`/`SavedPosition` (`transport/transport.go`), API:s shadow-dispatcher i `handlers/province.go` och `handlers/messenger.go`, samt direkta returben i `economy/trade.go` och `economy/ship_hospitality_transport.go` har kvar legacyavgång/-deadline. Budets egna landben ägs av `messenger.ResolveDeparture`/`ResolveReturnDeparture`/`StartReturnLeg` och äldre `province.ReadMarch`-läsning. De ska få samma sparade ankare/prefix i nästa slice; kvarvarande klockor flaggades i [inventeringen](hex-for-hex-inventory.md) och chat.log09:30/09:52. A påstår inte att de är migrerade.

Sikt via annat öga eller befintlig ögonradie kan göra en framtida rutthex synlig tidigare; det är befintlig siktmodell. Prefixproven testar vilket minne själva resan skriver, och stopp-provet hindrar minne längs den övergivna framtida vägen. Ingen balans-, frakt-, budland- eller wikiartikelinventering är smygändrad här.
