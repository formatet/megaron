# Slice22 implementation — kontrakt före kod

Claude START2026-10-19 08:31 efter godkänd mätning e10735a6; 21b överlämnad c9947e5b.
Problem: lokalt timmer saknas på cirka26% av möjliga grundningsplatser.
Spelarsanning: macchia/kullar/floddal erbjuder lite timmer och ett sågverk kan förbättra samma hex.
Invariant: timber0.33 per gubbe/tick,1basplats och lumbermill-koppling på exakt tre nya terränger; olivlund/ceder/sten orörda.
Scope: migration189, economy/hex_rules, regressionsprov via verklig PG och HTTP, Codex samt klientverifiering.
Non-scope: kartgenerering, cederomvandling, stenbalans, annan spelmekanik eller live-data.
Acceptans: placerbar timmerarbetare utan skog; korrekt ett/fem platser och nivåbonus; sågverksbygge godtas på nya terränger; recompute/migration säkra; fyra ytor samma sanning.
Stopvillkor: hash+STOPP, Claude äger merge/push/deploy och vaultintegration.
Bevisplan: oförändrad baslinje188, nya namngivna RED föreproduktion, fresh189GREEN, fysisk mutation per terräng och regel, naturligt isolerat CLI/webbflöde där möjligt, full Go/vet en gång vid slutet.

## Resultat och proveniens

Serverimplementation149f0eba: migration189 och explicita timmerplatser/sågverkskoppling på maquis, kullar, floddal. Spelar-Codex förklarar samma regel och rättar tidigare skogskrav i grundningsråden. Klienternas befintliga placement-options-väg återanvänds; ingen keryx-/webbproduktionskod behövde ändras.

Oförändrad baseline på färsk188: economy4,644s och handlers37,774s gröna (`baseline188.log`). Nya HTTP-testet blir namngivet RÖTT på alla tre terränger: ingen timmeroption och sågverksbygge422 (`before-RED.log`). På färsk189 blir samma placerings-/byggtester GRÖNA (`target-GREEN.log`). Första nya testversionen jämförde felaktigt API-grundtakten med effektivt utbyte efter sågverk; testet rättades att kräva grundtakt0,33 och marginalutbyte0,561/1,023. Ingen produktionsfix behövdes för den testmissen.

| Grind | Bevis |
|---|---|
| Funktion | Autentiserad HTTP erbjuder1plats/0,33, accepterar arbetare och nekar en andra trots ledig arbetskraft. L1sågverk ger5platser/0,561; fem arbetare ger2,805. L3ger1,023/gubbe och5,115totalt. Upprepad recompute behåller5placeringar och takt. POSTbuild tillåter sågverk på samtliga tre terränger. |
| Mutationer | Sex fysiska: ta bort produktionstakten respektive Go-regeln på var och en av tre terränger. Alla ger namngivet RÖTT; byte-exakt restore→ny färskPG16→GRÖNT. `mutations.json` och `mutation-logs/`, kör med `python3 docs/reviews/macchia-timmer/mutate.py`. |
| Migration/livscykel | Ny egenPG16, verkligt populerad188→189→188→189. Exakt samma stads-/lager-/byggnads-/placeringsrader; samtliga44gamla reglers ID/innehåll oförändrat; endast tre nya rader0,33. Att köra upSQL igen duplicerar inget. Down återställer hela baseline-snapshoten. `migration-proof.json`; reproduktion `migration_probe.py`. |
| Recompute-all | Den riktiga binären kör dry-run och två apply på oförändrat/frozentick100. Dry-run lämnar hela snapshoten orörd. Andra apply är exakt no-op; lager37timber/12stone/99grain, befintlig olivsågverkstakt1,7 och valid placering bevarade. Ingen automatisk timmerplacering tillförs. |
| Semantik/fyra ytor | placement-options skiljer0,33grundtakt från0,561effektiv marginal medL1. API, keryxcity/place och webbens riktiga rad visar samma option; Codexartiklar buildings/catchment/founding/goods uppdaterade. Codex/grid31JS-test gröna (`js.log`). |
| Användare/visuell | Två separata naturliga nyvärldar: vanlig registrering, CLIlogin/join/founding/settle/place, browserCity→sammahex→minus→plus. Firefox1280+390/DPR1, ingenpageerror/horisontellöverströmning. Granskade1:1-skärmdumpar `native-a/`, `native-b/`. Ingen DB-skrivning eller replayad API-payload i dessa körningar. |
| Drift | Ingen merge/push/deploy och ingen recompute mot live. Claude integrerar. Migration189 är enda produktionsartefakten iSQL, men Goändringen gör normal rebuild tillämplig; verifiera ändå processcommit/schema189/dirtyfalse enligt vanlig deployrutin. |

Runtimesvar ur processerna (`native-a/proof.json`, `native-b/proof.json`): båda `commit=149f0eba72a0cda30a3764316f3f9f6eed2174eb`, `migration=189`, `status=ok`. Detta är prover från den kodrena servercommitten; senare Codex-/beviscommits ändrar ingen serverproduktion. Världarna är olika, med egenPG/Redis/process och vanliga användarflöden. Inga authconfigar eller privata testlösen ingår i repot.

Native-reproduktion: `python3 docs/reviews/macchia-timmer/native.py /tmp/macchia22-new-proof`. Scriptet bygger server/keryx, startar egna nya containrar och städar endast dessa. Det väljer en verklig ny timmerterräng i den naturligt grundade staden; om platsen saknar sådan terräng är det ett uttryckligt riggutfall, ingen fabricerad karta. Båda sparade körningarna träffade kullar; de tre terrängerna täcks genom riktiga HTTP-fixturer, inte genom att kalla två kullprov ett treterrängsprov.

## Avgränsningar och handoff

- Mätningens26,19%→2,96% utan lokalt timmer är geografisk potential, ingen garanti om tillväxt eller arbetarfördelning.
- UI:t rundar0,33till0,3 i befintlig radformatering; API och Codex bär exakt0,33. Formateringens allmänna precision ändras inte i denna slice.
- Den första migrationsproben råkade detektera Postgres init-fas via Unixsocket före TCP-serverstart. Riggen rättades till TCP-readiness och kördes från en helt ny container; produktionsmigreringen ändrades inte för detta.
- BILD/TEXT-bedömning och vaultintegration till Claude/Timothy. Sten/ceder/olivlund ändras inte; deras gamla44regler jämförs exakt i migrationsprovet.
- Inga nya verb, notisarter eller Umami-event införs.

Slutprovets katalogtest bar en gammal förväntan om endast två skogsterränger. `TestBuildingCatalogue_ExposesTerrainGate` har uppdaterats till alla fem och jämför nu varje rad, inte bara de första två. Rättelsen är testcommit dd448c8b och ändrar ingen produktion. Första fullsvepet föll endast där; omkörningen sker mot en helt nyPG189 efter rättelsen.

**Slututfall:** full färskPG16/migration189 Go-svit på dd448c8b HELGRÖN, inklusive api/handlers och internal/world; `full-final-GREEN.log`. Slutlig `go vet ./...` grön och31Codex/gridJS-test gröna. Runtimeprovens serverproduktionsbytes verifierade identiska med149f0eba även efter dokument- och testrättelsecommits. Hash+STOPP efter beviscommit; 21c nästa enligt Claudes kö.
