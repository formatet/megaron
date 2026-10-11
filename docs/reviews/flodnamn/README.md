# Floder har namn

Temenos härleder nu ett stabilt, unikt namn per flodkedja ur hela den statiska kartan. Floden, dess vad och dess sammanhängande deltaö delar namn; endast kända hexar får `river_name`. Webbpanelen, kartetiketten, Keryx och Codex använder samma namn.

Status: byggd på `codex/flodnamn`, låst bas `f57eb4b0` (A ingår). Inte mergad eller deployad. BILD lämnas till Claude/Timothy. Kedjegrind: bevisar geografisk läsbarhet på befintlig karta.

## Kontrakt

| Fält | Innehåll |
|---|---|
| Problem | Flodkedjor saknar namn på spelarens kartytor. |
| Spelarsanning | Samma flod känns igen vid flod, vad och delta, även på minnesmark. |
| Invariant | Stabilt efter omstart; unikt inom världen; okänd terräng avslöjar inget namn; kartan ändras inte. |
| Scope | World-namnlista/härledning, API map/placement-options, webbpanel/etikett, Keryx map/city/place, Codex. |
| Non-scope | Mapgen-omkörning, migration, reseed, rörelse, worker, balans, verbregister och Umami. |
| Acceptans | Determinism/unikhet/deltaarv; FOW; fyra ytor; mätt etikettkollision; fresh Go/JS/vet och verkliga mutationer. |
| Stopp | Hash och STOPP; BILD granskas före integration. |
| Bevis | Låst baslinje, riktiga HTTP-handlerprov på färsk DB, CLI-HTTP-prov, panelklick, Firefox först, pixelinneslutning. |

## Namn och ägarskap

CSV: [`server/internal/world/river_names.csv`](../../../server/internal/world/river_names.csv). Timothys redigerbara lista, exakt dessa 30 namn ur ordern:

Alpheios · Eurotas · Acheloos · Peneios · Skamandros · Simoeis · Maiandros · Hermos · Kaystros · Halys · Sangarios · Orontes · Axios · Strymon · Nestos · Hebros · Granikos · Asopos · Kephisos · Inachos · Ladon · Pamisos · Euenos · Haliakmon · Iardanos · Leontes · Kydnos · Pyramos · Saros · Xanthos.

Fullständig läsning av flodfamilj/delta ur den levande världen `f7c90afa-4653-46a6-b14c-473a268efa3e` gav **35 hexar och 6 kedjor** ([oförändrad SQL-dump](live-river-tiles.json)). En oberoende sexgranns-BFS räknade river∪river_ford. Repo-fixturerna world-full, world-fow och world-stor230 har **0** flodkedjor: gamla dumpar före flodmekaniken. Största observerade kedjeantal är därför **6**, under poolens 30. SQL var endast SELECT; ingen reseed eller annan skrivning mot dev.

`riverComponents` är gemensam BFS-ägare för befintliga mapgen-mått och namn. Kedjor/hexar sorteras lexikografiskt (q,r). SHA256 av world_id + kanonisk hex väljer start i listan; upptagna namn hoppas över i kanonisk kedjeordning. Efter listans slut används deterministiska nummersuffix för fortsatt unikhet. Ingen processlokal slump eller DB-tabell. Sortering gör SQL-radordning irrelevant. Sammanhängande deltaöar ärver sin angränsande kedja; en hypotetisk delad deltaö tillhör den lexikografiskt första kedjan. Flodslätt (`river_valley`) är omgivande mark och får inget flodnamn.

`/map` härleder ur **hela** kartan före tilldelning till live∪minne. `river_name` är omitempty och tilldelas endast när samma hex är känd. Placement-options har motsvarande befintliga kunskapsgrind. Varken klient eller CLI räknar kedjor ur en ofullständig vy.

## Textformer och ytor

| Val | Flod | Vad | Delta |
|---|---|---|---|
| **Byggd konsekvent form** | River Alpheios | Ford of Alpheios | Delta of Alpheios |
| Alternativ med artikel | River Alpheios | Ford of the Alpheios | Delta of the Alpheios |
| Alternativ med namn först | Alpheios River | Alpheios Ford | Alpheios Delta |

Kartetiketten visar bara **Alpheios**. Den använder kartans befintliga tvåpass-stil: 6px monospace, samma ljusa ton och mörka stroke som andra kartnamn. Inga CSS-färger eller nya terrängregler. Lokalt läsbart zoomregister (≥0,55); en etikett per känt namn i utsnittet, förankrad vid den kända delen. Den flyttar mellan kandidater för att undvika stadsnamn, stadssilhuetter, andra flodetiketter och viewportkant. Saknas plats utelämnas etiketten; panelen har alltid det kända namnet. Ingen placering använder okända hexar.

Webb: terrängpanelens titel/rad, ruralpanelens titel/rad och marchmålens namn. Keryx: `map` text och JSON, `city` hexrader och `place` kvittens via placement-options. Några fristående `hex`/`inspect`-kommandon finns inte i nuvarande Keryx. Historiska notifieringspayloads och generella terrängkrav är kvar som sina befintliga maskintokens; de är inte karthex-vyer. Codex: [`sight.md`](../../../web/static/codex/sight.md), avsnitt Rivers.

## Baslinje, tester och mutationer

- JS: **558 → 562**, inga skips ([bas](baseline-js.log), [slut](final-js.log)). Nya prov för panelens riktiga inputväg, namnformer, FOW, etikettunikhet, minzoom och kollisionslayout.
- Go-bas i **separat låst arbetskopia** `/tmp/megaron-flodnamn-baseline`, samma bascommit, egen färsk DB/migration171: world, handlers och Keryx gröna ([logg](baseline-go-locked.log)). Första körningen hann läsa pågående ändringar och blev ogiltig; den är bevarad som [baseline-go.log](baseline-go.log), inte klassad som regression eller använd som bevis.
- Slut: full **`go test -count=1 -p 1 ./...`**, 25 testpaket gröna, fresh PG16/migration171 med `env -i` ([logg](full-fresh-go.log)). Vet och Temenos/Keryx-byggen gröna ([vet](vet.log)). Inga nya paket eller G1-undantag.
- `TestRiverNamesDeterministicAcrossProcesses`: omvänd SQL-ordning, upprepade anrop, två nya subprocesser och världssalt. `TestRiverNamesUniqueAndDeltaInheritance`: distinct chains, ford/deltaö-arv, 33 kedjor inklusive poolutarmning, inget namn på plains.
- `TestMapRiverNamesFOW`: verklig auth, production chi-handler och färsk PostgreSQL; live, remembered och fog i samma kedja. Ingen `river_name`-nyckel på fog. [Verklig svarspayload](map-fow.json).
- `TestKeryxRiverNamesMap`: Keryx kommandot mot HTTP-fixtur och faktisk stdout; river/ford/delta. Catchment-JSON och namnformatterare täcks separat.
- Sju **verkliga produktionsmutationer**, alla namngivet röda: [världssalt](mutation-world-salt.log), [unikhet](mutation-unique.log), [delta](mutation-delta.log), [FOW](mutation-fow.log), [Keryx](mutation-keryx.log), [verklig panelkoppling](mutation-panel.log), [etiketter](mutation-labels.log). Varje fil byte-identiskt återställd i finally; full Go/JS därefter gröna. [Körlogg](mutations.log).

## Bilder och etikettmätning

Firefox först; faktisk 1:1, DPR1 och spelzoom1,0. Riktig map.html, CSS, renderer, input och main-modulens handlers. Endast start-IIFE ersätts med explicit fixturbootstrap. HTTP `/map` använder den sparade verkliga FOW-payloaden; omgivande slätt och städer är **uttryckligen syntetiska**. Mycenae/Tiryns och Agamemnon/Nestor finns i fixturen före rendering.

| Firefox | Före | Efter |
|---|---|---|
| Karta 1280 | [Före](firefox-1280-map-before.png) | [Efter](firefox-1280-map.png) |
| Karta 390 | [Före](firefox-390-map-before.png) | [Efter](firefox-390-map.png) |
| Flodpanel 1280/390 | | [1280](firefox-1280-0-panel.png) · [390](firefox-390-0-panel.png) |
| Vadpanel 1280/390 | | [1280](firefox-1280-3-panel.png) · [390](firefox-390-3-panel.png) |
| Deltapanel 1280/390 | | [1280](firefox-1280-16-panel.png) · [390](firefox-390-16-panel.png) |

Chromium och WebKit har samma fyra efterbilder och fyra före per viewport (`<motor>-<bredd>-…png`). Riktiga pointerklick öppnar panelerna med samma servernamn i alla sex kombinationer, utan konsol- eller pageerror. [Browserlogg](browser.log), [faktiskt ritade textrutor](browser.json). En flodetikett per viewport, **noll överlapp** mellan flod/stadsnamn. Måttet inkluderar glyphernas stroke och visar geometrisk separation; det mäter inte mänsklig igenkänning.

Basens map.js serveras via route på samma fixtur och kamera. I alla sex kartpar ligger **varenda ändrad pixel i nya namntextens mätta ruta**; inga pixlar utanför ([inneslutning](containment.json)). Terräng, fåror, dimma och ikoner är därmed pixeloförändrade. Firefox-bilderna granskade vid 1:1; BILD till Timothy går via Claude.

## Repro och grindar

```sh
tools/gotest.sh ./api/handlers/ -run TestMapRiverNamesFOW -args -river-proof-dir=/ABS/WORKTREE/docs/reviews/flodnamn
python3 tools/flodnamn_browser.py
python3 tools/flodnamn_browser.py --baseline
python3 tools/flodnamn_mutation.py
node --test web/static/js/megaron/**/*.test.mjs
tools/gotest.sh
```

`go vet ./...` och `go build ./cmd/server`, `go build ./cmd/keryx` körs från server/. Python-verktygen behöver Playwright (tre motorer); browserverktyget startar egen lokal statisk server och gör inga externa speloperationer. [Källhashar](sources.sha256).

| Grind | Utfall |
|---|---|
| Kod | Full fresh Go171, 562JS, vet, båda byggen, sju mutationer och diffcheck gröna. |
| Visuell | Firefox först + tre motorer desktop/390; paneler och etiketter; inneslutning. BILD väntar. |
| Semantik | Servern äger namnet; live∪minne har namn, fog inget; deltaö ärver. |
| Användare | Verkliga renderer/input/CLI-kedjor på kontrollerade HTTP-fixturer. Ingen komplett login/march-runda mot verkligt spel påstås. |
| Drift | Ingen merge, push, migration, reseed eller deploy. Alfa läst endast. |

Begränsningar: nya framtida namnlisteändringar kan byta härledda namn och kollisionsval. Etikett kan döljas om ingen plats finns. Browserfixturen har en namngiven kedja med avbruten kunskap; JS/Go täcker flera kedjor och poolutarmning. Verkliga spelet och Worker är separata beställningar.
