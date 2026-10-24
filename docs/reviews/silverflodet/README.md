# Silverflödet

Kontrakt: verkställ megaron_plan_silverflodet_20261024 rad 1–10. Blockerar kedjegrinden: fria brevtransporter får inte upphöra av silverbrist. Spelaren möter ändringen i rekryteringskatalogen och arméns soldrad.

Invariant: inga befintliga förråd eller betalda köer ändras; gamla events behåller sin tolkning. Scope: katalog, sold, handelsleverans, stridstärningar, ny host, API-klienternas presentation och Codex. Ingen ny silverkälla, migration, integration eller deploy. Migration krävs → stoppa och fråga.

Bevis: röda kontraktsprov mot gammal produktion, beslutsspecifika mutationer, färsk full Go-svit, JS/Codex/day_guard och kontroller av API/CLI/webb. Inventering och slutresultat fylls i nedan.

## Inventering

- `tithe.go` användes endast av `DeliveryHandler`: 10 % över tröskeln togs från krediteringen och `TempleTithe` skrevs. Ingen tempelkassa/kharis fick något. Inga andra runtime-läsare finns. Funktionen och dess gamla policytester är borttagna, historiska eventrader/migrationskommentarer orörda.
- Governor är passiv. `create-world` och världsstartens `SetWorldStartWanaxes` skapar ingen egen silverutbetalning. `seedHost` äger ny120/rest60; grundning flyttar bärd börs, kolonier bär befintligt silver. `genesis_silver.go` sätter tak, inte saldo, och är orörd.
- **Kydonia/Gournia960 är historisk produktion mot taket**, inte bevis för960 startsilver. Read-only mätning: `silver_mining_totals`2139/2564; Gournia varnar1754 med666/net12.5 och overflow1778, Kydonia varnar2326 med841/net5 och overflow2350. Råbelägg: [live-silver-readonly.log](live-silver-readonly.log). Inga liveskrivningar.
- Keryx och War läser API:s katalog och `UnitUpkeep`. Inga hårdkodade priser behövde ändras i dessa produktionsklienter. Webb visar per man (1.5 timmer,1.6 ceder), Keryx hela fartyg (30 timmer,16 ceder). Codex uppdaterad för kostnader/sold/tärningar/120/handel utan tempelandel.
- Emporos reparation väljer redan levande katalogmaterial: den blir därför ceder utan ny reparationsregel. Äldre repair-test uppdaterat.
- POST Recruit gick först genom aggregatet “någon typ har byggnad+varor”. Det kunde dölja cederbristen. Det konkreta typvalets befintliga bygg-/varu-/populationskontroller ger nu felet. Claude godkände detta med krav på bibehållna vägranden. `TestSilverFlowRecruitAggregateRefusalsRemain` visar CanRecruit=false för byggnader/varor/befolkning och fortsatt422 + oförändrat DB-snapshot. Aggregatet lever kvar för affordance/action-listor.

## Bevis

- Gammal produktionskod RED: [baseline-red.log](baseline-red.log) och [baseline-http-trade-red.log](baseline-http-trade-red.log): katalog, faktisk POST-rekrytering, sold, rollSide50/ceil/5, host120, religiös handel100→90.
- Elva fysiska mutationer RED, namngivna assertioner, byteexakt restore: [mutations/results.json](mutations/results.json), reproduktion `tools/silver_flow_mutations.py`. Beslut: tre silverfria rekryteringar, Emporos-material, två fria skeppslöner, vagnsold, två tärningsregler, host, tionde.
- Riktig DB/HTTP: [final-contracts.log](final-contracts.log), [updated-targeted.log](updated-targeted.log). De första riktade loggarna visar även utvecklingsfel: en oanvänd import och en för bred testvariabeländring; båda korrigerade före slutproven. Tidiga breda tester hittade äldre balansassertioner som uppdaterats till kontraktet. Hold-to-last-man-fixturen har större anfallare så att den inte råkar avgöras av anfallarens egen reträtt med nya tärningar.
- `TestSilverFlowBasicShipsNeverDesertForSilver`: galär/Emporos hemma/ute, 74 riktiga upkeepanrop, 0 silver, tidigare unpaid=71 →0, besättning kvar. Professionell lön/avdrag/audit/runway och rekryteringskonkurrens verifieras fortsatt i befintliga integrationstester.
- [Bildpaket](index.html): två oberoende färska PG/Redis/server-världar, riktig Firefox1280/390 och installerad-för-provet Keryx. Vanlig registrering/join/grundning120; sedan explicit sparad lokal fixtur. Browser POST tränar spjutmän med0silver; CLI bygger Emporos och debiterar exakt16ceder, silver0. Full källhash före/efter varje lopp i `native*/proof.json`; ingen browser-API-mock eller delad rigg.
- Två riggfel redovisas öppet: första försöket laddade en stängd drawer, andra antog totalsumma där webben visar per-manpris. Sparade i `browser-harness-error.log` och `browser-per-man-assertion-error.log`. Ingendera är ett godkänt användarprov.

## Avgränsning

Ingen migration, uppdatering av befintliga förråd eller köer, merge, push eller deploy. Prayers/escrow och befintliga eventtolkningar är orörda. Bildpaketet avser dynamiska katalograder; ingen produktions-JS/CSS-layout har ändrats. End-to-end handel bevisas här via riktig leveranshandler/DB, inte ett browserhandelslopp. Ny språkorder (“game day”) inväntar separat START.


## Slutkontroller

756 JS och14 Codex gröna (`js-final.log`, `codex.log`); vet grön (`vet.log`); samtliga fyra day_guard gröna (`day-guard-final.log`). Två nativekörningar har910 hashade produktionsfiler vardera, exakt oförändrade även vid slutkontrollen (`final-source-verification.json`). Tidig chattstatus angav746 JS av misstag; faktisk sluträkning är756.

Hela Go-sviten körd **en gång** på nyskapad PG16/migration210: samtliga26 testpaket gröna, world154.482s (`full-go.log`). Därefter endast rapport-/verktygsdokumentation, inga produktions- eller teständringar. Slutcommit hämtas från `git log codex/silverflodet` och agentöverlämningen; grenen utgår från2e2cbcb3 och har inte rebasats under bevisen.
