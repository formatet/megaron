# J — City Production, BILD

Production har tre huvudsektioner: Population (folk/lediga/boskap + samma slakt), Catchment & workplaces och en Food-rad. Stängd More behåller devotion, personaldetaljer, reserv/regler/tillstånd, senaste dag, lojalitetslogg, kolonigåvor och dagshistorik. Tal skrivs i ord; berörda City/Economy-rater använder game day. Bas master **61a34cf7**, gren **codex/forenkling-production**. Kod **63293de3**; senare commits bara prov/dokumentation. Ingen merge/push/deploy; Claude granskar BILD enligt nattens delegation.

## Kontrakt och baslinje

Kontrakt före kod **5d3c6436**: sju sektioner → tre, alla förmågor och POST/PUT-bodies bevarade; samma Sitos-regler och servervärden, ingen ny CSS/inline-stil/färg. Matens dagar är `coverage_ticks`: aktuellt grain+fish-lager i staden vid aktuell konsumtion, utan framtida produktion eller separat reserv. Noll lager som växer får ingen falsk svältetikett. Bevisar brist-delen av kedjegrinden; hela geografi→brist→brons→elit är inte omspelad. Baslinje **404 JS** gröna.

**Provenance:** [baseline/proof.json](baseline/proof.json) actual health **61a34cf7/mig160**, oförändrade arkiverade assets; [after/proof.json](after/proof.json) och [repeat/proof.json](repeat/proof.json) actual health **63293de3315f16f6534a7843b0e7f51d2d88cd98/mig160**. Varje arm har egen tom PG16/Redis, rena servervariabler, tick sex sekunder, vanliga register/join/founding och webbhandlingar; ingen SQL-fixtur/mutation eller delad spelvärld. Recept i `tools/README.md`. Repetitionen använder samma byggda binär/kod och en ny DB.

## Experiment och grindar

| Prövning | Resultat |
|---|---|
| Tre primära sektioner, More stängd, sekundära IDs/kolonigåvor kvar | JS grönt; fysisk More-open mutation namngivet röd→återställt grönt |
| Blandmatens coverage, inte grain-net eller reserv; samma låg/hög/state | JS grönt; fysisk coverage-källa och hög-tröskel mutation namngivet röd→återställt grönt |
| Noll-växande/låg/fallande/kan-inte-mätta/saknat värde | JS grönt, fem befintliga state-grenar bevarade |
| Riktig webb före och två efter | Arbetsrutnät −1 DELETE200→+1 POST201 med samma hex/good; historik nåbar; slakt POST200 och exakt +10 folk; noll browserfel |
| Kod | **409 JS**, full `tools/gotest.sh` ny PG16/mig160 alla paket (world93.118s), full ren `go vet ./...` exit0. [Loggar](logs/). Server/keryx/CSS-diff tom |
| BILD | Desktop1280×900 +390×844, 1:1; primärvy, matraden, More och Economy i [baseline](baseline/), [after](after/), [repeat](repeat/). Ingen horisontell overflow. Livestock-knapp fick egen rad efter första bildkontrollen |
| Semantik/användare | Matdagar/reserv/lagersaldo hålls isär; verkliga handlingar ovan rena. Begriplighet i oberoende playtest kvar; BILD lämnas till Claude |
| Drift | Ingen egen deploy; integration/deploy ägs av Claude |

## Avgränsningar och lärdom

Devotion- och Gift-handlers, clamp/temple/koloni-gates och requestbodies är kvar; testet säkrar placeringen under More och kolonigate. Färska världar har ingen temple/koloni: verklig devotion-PUT och gåvans caravanresa hävdas inte. Kontrollerna på Buildings/Garrison och numeriska inmatningar/koordinater rörs inte. Befintliga inline-stilar har behållits; inga nya introducerade.

**Sidofynd till Claude:** Economy overview visar redan före ändringen `no data` och nollor. `loadEconomyGoods` mappar overview settlement-id mot `State.provinceData` province-id. `settlement.go:145` returnerar settlement-id. [Baslinjens Economy](baseline/economy-mobile.png) och efterbild visar samma fel; J ändrar bara text/rater och lagar inte kopplingen. Detta behöver separat order.

Tidigt rigganrop förkastat: kort expected-hash jämfördes med full health-hash; rättad körparameter, sedan faktisk full hash kontrollerad. Arbetsträdskatalogen måste vara explicit även för dokumentationsskript. De tre huvudsektionerna kräver fortfarande vertikal scroll på mobil efter stadsbild/rutnät; [matraden](repeat/production-food-mobile.png) visar Food och stängd More utan trunkering.
