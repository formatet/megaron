# Economy overview id — BILD

Översikten visade nollor och **no data** trots verkliga stadslager: den sökte ett settlement-id med province-id. En rad i klienten ändras till `overviewByID.get(s.settlement_id)`; province-id behålls för stadslänkar och varuanrop. Servern skickar **rätt id**. Bas **mastercfac469d**, gren **codex/economy-overview-id**, kod **153cbb03**; senare commits bara Codex/prov/rapport. Ingen merge/push/deploy, hash och stopp.

## Kontrakt, reproduktion och provenance

Kontrakt/reproduktion före fix **1ee9a479**: actual loadEconomyDrawer-test med skilda id:n, två lika stadsnamn och omvänd overview-ordning går namngivet rött. Real baseline health **cfac469d/mig160**, arkiverade oförändrade assets, vanlig register/join/founding: population **1000**, citystock **1208 grain**, netgrain **0.6**, coverage **241.6**, men översikten visar zero/no data. [API/DOM](baseline/proof.json), [JS-rött](logs/regression-red.log).

[After](after/proof.json) och [repeat](repeat/proof.json): actual health **153cbb03542c3c7a0bb2b6ee80899e406065656b/mig160**, samma fullständiga binär SHA256 **a65b578c24fabed15213a62bb6465760d7310e940d33ab20fad074d0c4def24d**, separat tom PG16/Redis per arm, ren miljö/tick sex sekunder. `province.id != province.settlement_id == details.id == overview.id == founded.settlement_id`. Positiva lager bevisas med vanlig goods-API, inga SQL-fixturer/mutationer. Alla egna processer/containrar städade. Recept: `tools/README.md`.

## Prövningar och grindar

| Prövning | Resultat |
|---|---|
| Actual drawer-kedja, rätt join/province-länkar/ett overview-anrop | **419 JS** gröna; original regression röd→fix grön; fysisk återinförd `get(s.id)` ger namngivet rött→återställt grönt |
| Missing/refused/wrong-id overview | no data kvar; ingen namn-/province-id-fallback; rätt syskonrad blir inte noll |
| Riktig webb, två rena eftervärldar | Hela översiktsraden jämförs med samma serverdata genom presentationsfunktionen: one thousand, +zero point six each game day, two hundred forty one point six game days, surplus. Ingen browsererror/SQL-mutation/horizontal overflow |
| BILD | [Före](baseline/economy-mobile.png) / [efter](after/economy-mobile.png), desktop1280×900/390×844 och separat repetition i katalogerna. Granskat 1:1; rätta längre ord ryms. Claude bedömer enligt delegation |
| Kod/fyra ytor | Endast join-raden + kommentar och Goods-Codex om översikten; server/keryx/CSS/map-diff tom. **Full tools/gotest.sh ny PG16/mig160 ALLA paket** på e03a692d (world93.536s), full ren **go vet ./... exit0**; [loggar](logs/) |
| Kedjegrind/användare/drift | Bevisar brist-översiktens faktiska data, inte hela geografi→brist→brons→elit. Ingen ny spelarprosa i klienten, oberoende playtest inte körd. Ingen egen deploy |

## Avgränsningar och lärdom

**Found the metropolis-confirm** (`render/map.js:4410`) bokförd separat: annan grundningshandling, rörs inte i denna lilla id-fix. City-länken behåller province-id och laddar rätt City; befintlig openCitySettlement lämnar Economy-drawern öppen ovanpå. Det sker redan i baslinjen; riggen stänger Economy för City-bilden, ingen navigationsfix smygs in. Båda fynden till Claude för nästa order.

Första baseline-riggen förkastad efter lyckad felreproduktion: inner_text uppercasar Cityrubriken; rättad kontroll använder text_content. Första full Go-körningen stoppades av **disk quota exceeded** vid länkning, inte testassertion. Egna avslutade K-binärer (~215MB) städades; full Go kördes om mot ny DB. En repeat-kopia blev trunkerad trots fungerande health/grön resa: SHA skilde sig och armen förkastades. Ren repeat använder symlink till hela efterbinären och matchande SHA. Endast rena armar ovan räknas som slutbevis. Ställ alltid rätt identitet på varje API-gräns; matcha aldrig samma stadsnamn som genväg.
