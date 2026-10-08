# K — spelartext och inline-resultat, BILD

Tempo-raden försvinner från Notifications; dolda historiska matrapporter får spelarord och räknas i ord. Sju dialogvägar ersätts med inline-resultat. **Abandon behåller ett uttryckligt andra klick**, och Keep settlement avbryter utan POST. Sex Lawagetas-texter blir korta rader + `?`; Locked blir en rad med deduplicerade namn, samtliga serverns saknade krav som tooltip och rätt Codexlänk. Notifications/Gossip hålls separata. Bas **mastera7c77c0d**, gren **codex/forenkling-text**, produktionskod **0c0baf39**; senare commits bara test/rigg/dokumentation. Ingen merge/push/deploy; hash och stopp efter K.

## Kontrakt och baslinje

Kontrakt före kod **1571dc75**: behåll förmågor, bodies, filter, arkiv, skäl och gates; inga server/keryx/CSS/kanonändringar. Economy overview id-koppling bokförd för egen slice efter K, orörd här. Bevisar handlings-/informationsytor kring brist i kedjegrinden; hela kedjan är inte omspelad. Baslinje **409 JS** och sex drawers desktop1280×900/mobil390×844 före kod.

**Provenance:** [baseline/proof.json](baseline/proof.json) actual health **a7c77c0d/mig160**, arkiverade oförändrade assets. [after](after/proof.json), [repeat](repeat/proof.json) och [confirm](confirm/proof.json) actual health **0c0baf39e933e101fbc6d60fd4237c52cd943f38/mig160**, identisk produktionskod/binär, separat tom PG16/Redis per arm, rena env, tick sex sekunder, vanliga register/join/founding. Ingen SQL-fixtur/mutation. Alla egna processer/containrar städade. Recept: `tools/README.md`.

## Prövningar och grindar

| Prövning | Bevis |
|---|---|
| Kalendar/lobby kvar, tempo borta; grupper/rader spelarord | JS; fysiskt tempo/grupp-mutation namngivet röd→återställt grönt; historisk syntetisk replay testar filter/drill/back med riktig renderare |
| Samtliga sju dialogvägar | JS importerar riktiga handlers: cancelBuild, rite lyckad/refuserad, dipCancel/ArrangePassage/CallBack, warAbandon; servertext bokstavlig/accessibel, knappar återaktiveras, samma methods/bodies |
| Abandon explicit bekräftelse; ritesvar efter refresh | Fysiska preconfirm/rite-refresh-mutationer namngivet röd→återställt grönt; cancel=0 och confirm=1 callback även i Chromium-fixtur |
| Riktig webb | Tre färska efter-armar: sex brief-`?` till exakt rätt Codexartikel, Locked-tooltip och Goods-länk, faktisk DELETE404 för saknad kö och ritePOST400 för okänd bön med serverns exakta text inline. Inga dialoger/browserfel |
| Kod | **417 JS**, full `tools/gotest.sh` ny PG16/mig160 på c554f437 alla paket (world92.079s), full ren `go vet ./...` exit0. Server/keryx/CSS/Economy/Gossip-diff tom. [Loggar](logs/) |
| BILD | Före/efter desktop390 sex drawers; [Locked](confirm/locked-mobile.png), [inline-fel](confirm/cancel-inline-mobile.png), [rite](confirm/rite-inline-mobile.png), [syntetisk Abandon](confirm/abandon-fixture-mobile.png) och [historiska grupper](confirm/historic-groups-mobile.png). Granskat 1:1; inga nya stilar/färger, ingen trunkering i ändrade rader. Claude bedömer enligt delegation |
| Semantik/användare/drift | Arkiv/filter/body och alla gates kvar; kort prosa hänvisar till Codex. Oberoende begriplighets-playtest kvar. Ingen egen deploy |

## Avgränsningar och metodisk lärdom

De äldre SitosIntervention/SitosFundLow-kinds emitteras inte längre av aktuell server; deras grupper visas därför med **syntetisk API-replay**, inte med påhittade DB-notiser. Abandon-bilden är en **syntetisk DOM-fixtur** som använder samma helper, utan kolonisering eller abandon-API-anrop. Diplomacy-refuseringarna, lyckad rite och abandon-POST/refusering provas med riktiga handlers och stubbat HTTP i JS, inte med verklig handel/gudssvar/koloniövergivande. Webbens cancel404/rite400 körs genom window-bryggan eftersom en olaglig handling saknar klickbar vanlig knapp; ingen lyckad byggavbokning eller rite hävdas.

Förkastade riggar: öppnade flera drawers utan att stänga tidigare och fick pointer-interception; sökte All notifications globalt och träffade även Codex; bedömde Codextitel innan dess asynkrona laddning var klar. Riggen stänger gamla drawers, scope:ar sökningen och väntar exakt artikel. Första mutationsarmen gick röd men saknade namngiven första assertion; assertionen fick rätt namn och alla fyra armar kördes om röd→grön. J:s separata Economy id-fynd kvarstår och har inte smugit in i K.
