# tools — offline visuell rigg

Renderingsprinciperna och acceptansgrinden bor i `megaron_terrangrendering.md` (vault).
Här står bara HUR riggen körs.

## Go-sviten mot färsk DB

`tools/gotest.sh [paket] [go test-flaggor]` startar en tom Postgres i en engångscontainer,
migrerar till senaste versionen (och vägrar köra om `schema_migrations` inte stämmer), kör
`go test -count=1 -p 1` med rensad miljö och river containern. **Det enda utfall som räknas som
bevis** (`megaron_arbetssatt` §3). Baslinje och fix = två körningar = två färska DB:er automatiskt.
`-p 1` är inte valfritt: parallella paket slåss om `one_active_world` och `current_world_tick()`
blir NULL.

## Acceptansriggens härkomst

`tools/acceptance.sh up` bygger med `BUILD_COMMIT` och **avbryter** om containerns `/healthz`
(`commit`, `migration`) inte matchar repot. `status` varnar, `provenance` skriver raden som ska stå
i varje rapport som bär mätdata. Läs aldrig riggens commit ur `git rev-parse` — det är värdens, inte
containerns.

## Reseed av livevärlden

`tools/reseed.sh [--dry-run]` kör hela `megaron_drift.md` §Reseed-runbook i ett kommando —
pre-flight, ny värld, städ av `scheduled_events`-zombies, och (alltid) `systemctl restart poleia`
med verifiering mot en NY `world ready`-rad i journalen. Se skriptets egen header och
`megaron_drift.md` §Reseed-runbook för detaljerna och fällorna (healthz/`data-website-id` ljuger).

För **rent visuellt** arbete i renderaren ska browserrundan mot dev-servern inte vara loopen.
`web/static/showcase-*.html` fyller `State` med en fast fixtur och ritar EN fryst frame ur den
riktiga `render/map.js` — ingen server, ingen auth, ingen fetch, ingen deploy per iteration.

## De tre kommandona

```bash
python3 -m http.server 8099 --directory web/     # riggarna serveras på :8099
python3 tools/shot.py <etikett> [query]          # skärmdump → <etikett>.png
tools/pxdiff.sh före.png efter.png [diff.png]    # antal ändrade pixlar + diffbild
```

Andra argumentet till `shot.py` är `<rigg>`, `<rigg>:<query>`, eller en ren query
(som då går till terrängriggen):

| argument | Sida | Vad |
|---|---|---|
| *(tomt)* / `fixture=plains` / `zoom=1.6` / `world=<sträng>` | `showcase-forest.html` | Terräng |
| `units` | `showcase-units.html` | Enheter — **acceptansgrind** |
| `glyphs` · `glyphs:set=flaggor` | `showcase-glyphs.html` | Sigill/flaggor i flera skalor mot tre terränger |
| `cities` | `showcase-cities.html` | Ledrutnätet: två led × fyra murnivåer |
| `coast` | `showcase-cities.html?scene=coast` | Kustsektionen: sex kustgeometrier × två led × murnivå 0/2 |
| `world` · `world:zoom=0.3` | `showcase-world.html` | Helvyn ur en världsfixtur |

En riggnyckel får bära en egen query (`coast`), och då fogas ett användarargument
på med `&`. Det är så en SCEN i en befintlig rigg kan få sin egen viewport —
scenen är bredare än ledrutnätet, och en scen som inte ryms i sin viewport klipps.

`python3 tools/mapgenstats.py <katalog>` sammanfattar `cmd/mapgen-debug`s JSON-utdata
till en kalibreringstabell över flera seeds (cederandel, beståndsstorlekar,
skogsandel, walkable, flodandel). Det är kartgenereringens mätrigg, inte
renderarens — men samma arbetsregel gäller: **samma seedsvep före och efter.**
⚠ Reseedfrekvensen räknas ur `grep -c reseeding` på stderr, aldrig ur JSON:ens
`attempts` — se verktygets docstring för varför den ljuger.

`python3 tools/footing.py [rigg]` mäter **vad staden står på**: blått direkt under
stadsmassans fotlinje, per kolumn, plus en märkt bild. Noll är kravet på
kustscenen. Fotlinjen kommer ur renderarens egen `spriteGround` via riggens
`SHOWCASE.cities()` — måttet räknar inte om siluetten på egen hand.

`SHOT_DIR` styr var PNG:erna hamnar (default: nuvarande katalog). Lägg dem utanför repot.

Beroenden: `python3` + `playwright` (`pip install playwright && playwright install chromium`)
och ImageMagick 7 (`magick`).

## Tre saker som gör riggen användbar och inte får tappas bort

- **Fryst frame:** `requestAnimationFrame` stubbas i ett vanligt `<script>` FÖRE modulimporten,
  så `render()` inte schemalägger sig själv. Havsshimmer och gånganimation hänger på
  `State.animFrame`; utan frysning drunknar varje pixeldiff i brus.
- **Marschgångare i fryst bild:** ge marschen en ankomst som redan passerat → `progress` klampas
  till 1 och gångaren ritas exakt på målhexen. Annars interpoleras positionen mot väggklockan.
- **Viewporten måste rymma `#map-root` helt.** En för smal viewport klipper elementet, och
  eftersom kameran centreras ur `canvas.width` hamnar hela fixturen utanför bild. Symptomet ser
  ut som ett renderingsfel (allt dimma) men är en viewportbugg. Viewporterna bor i `shot.py`.

## ⚠ `compare -metric AE` är inte ett pixelantal

I ImageMagick 7.1.2 Q16-HDRI gav den 7,49×10⁷ för en bild med 702 000 pixlar där det sanna
antalet ändrade pixlar var 10 210. Den duger **bara** för frågan "är det exakt 0?".
Använd `pxdiff.sh` för allt annat — den separerar och maxar kanalerna före tröskling, så en
pixel som ändrats i en enda kanal räknas som en hel pixel och inte som en tredjedel.

## Acceptansvärlden — hela spelarflödet före merge

```bash
tools/acceptance.sh up            # isolerad Megaron på :8097, tick 6 s, karta 30x20
tools/acceptance.sh player Wanax1 # registrera + anslut, skriv ut Bearer-token
tools/acceptance.sh reset         # riv världen och seeda om mellan de två körningarna
tools/acceptance.sh down          # riv stacken och volymerna
```

`python3 tools/acceptance_foreign_units.py <suffix>` kör hela scenariot för främmande
enheter i ett svep: registrerar tre Wanaxes (spawnregeln balanserar hemisfärer, så nr 3
hamnar i samma halva som nr 1 — enda sättet att få två spelare på gångavstånd utan
DB-ingrepp), tar baslinjen, går en spjutbärare i etapper tills ögonen räcker fram och
skriver ut vad var och en ser. ⚠ Marschgrinden kräver KÄND målhex, så en etapp i taget
är enda vägen — och ett svar på 202 är en accepterad marsch, inte ett fel.

`python3 tools/acceptance_incoming_march.py <suffix>` kör användargrinden för
`ForeignMarchSighted` — notisen som startar klockan i asynkronitetsgrinden. Samma
treWanax-uppställning, men bara försvararen grundar sin stad (grundandet ställer
förbanden i garnison utan hexposition, så angriparen måste stå kvar som vandrande folk
för att ha något som kan marschera). Bevisar level 2 med stadsnamn och `arrive_tick`
medan hären fortfarande är i rörelse, noll notiser till åskådaren och till angriparen
själv, och att dedupen håller över flera skanningstick.
⚠ Vänta på att enhetens POSITION ändras mellan etapperna, aldrig på att `status` slutar
säga `marching` — enheten står kvar som `positioned` på sin gamla hex en stund efter
ordern, och läser man av läget då beställer man nästa etapp från en föråldrad position.

Eget compose-projektnamn (`megaron-acc`), egna volymer, eget nätverk, egna portar, egen
JWT-hemlighet. Den kan inte råka röra dev-servern eller live-DB:n — det är hela poängen.
Runbook och det verifierade scenariot: `megaron_drift.md` §Acceptansvärlden.

## Arbetsregler

1. En visuell sak per iteration. Skärmdump före/efter. Behåll bara det som förbättrar.
2. Diffa alltid — pixeldiffen bevisar vad som ändrades.
3. Determinism är krav: två körningar utan kodändring → 0 ändrade pixlar.
4. Enhetsriggen är acceptansgrind.
5. Mät innan du påstår.

### City graphics proof

`city_graphics_capture.py OUT [HOST]` records the existing world showcase and actual City drawer using explicit offline payloads (including geography checks). `city_scene_cases.py OUT [HOST] [BEFORE_CITY_JS]` captures five frozen native-size city scenes; the optional old renderer permits same-fixture comparisons. Both default to localhost:18199.

`city_graphics_acceptance.py OUT EXPECTED_BUILD_COMMIT` verifies the real server, player-founded city, desktop/mobile drawer, and served asset hashes in dedicated local infrastructure. Setup and evidence: `docs/reviews/grafik-stader-20261006/IMPLEMENTATION.md`.

## Sökning av brev och rykten

`python3 tools/search_messages_acceptance.py [output-dir]` kör Chromium mot en egen
lokal HTTP-server med riktiga sök-/drawer-moduler och explicita API-fixturer. Kräver
Python Playwright + Chromium; provar tangentbord, klick, fel, sena svar och mobilbredd.
Ingen spelvärld eller produktionsdata muteras.

### March arrival preview

`python3 tools/march_preview_acceptance.py` runs the actual web menu/Army modules with explicit authenticated API fixtures (per-unit timing, intent changes, cancellation and mobile width). `python3 tools/march_preview_live.py OUT COMMIT` uses freshly built `OUT/temenos` and `OUT/keryx` for register/join/found/preview/march on disposable local PostgreSQL16/Redis7 containers. Requires Docker and Python Playwright/Chromium; allocates dynamic ports and cleans up its own containers/process/config. See `docs/reviews/march-preview/README.md`.

`python3 tools/expedition_acceptance.py [OUT]` verifies the actual map and War expedition controls against explicit authenticated API fixtures: server-supplied duration rules, order summaries, preview GET and order POST ticks, mission row, and desktop/390px screenshots. It does not prove live expedition gameplay.

`python3 tools/expedition_live.py OUT COMMIT web|cli` runs register/join/found → expedition order → legs/turn → home/report with real player APIs, the compiled CLI and browser on disposable PostgreSQL16/Redis7 containers. Put freshly built `temenos` and `keryx` in OUT; the server must embed COMMIT. Uses a clean process environment, two-second game ticks and read-only SQL ground checks. Allocates its own ports and removes its own processes, containers and private CLI config. Run both modes in separate worlds.

`python3 tools/expedition_mutations.py --output-dir OUT` runs fresh-DB baseline, physically removes each of four protections (half-time turn, path sight, transactional report, actual return reservation), requires the named invariant test to fail, restores source after every mutation, then proves restored tests green. Run in an isolated worktree without concurrent edits to expedition.go.

`python3 tools/single_recall_live.py OUT HASH [cli|web]` runs actual single recall and
redirect through Keryx after register/join/found/march on private PG16/Redis and
clean server environment (tick6s). Requires per-unit MarchRecalled/Redirected
audits plus both units in garrison; SQL evidence is read-only. Build temenos and
keryx in OUT, stamping temenos with `-X main.buildCommit=HASH`. Cleans own resources.
The optional web mode clicks the real War Recall/Redirect buttons in Chromium
on a separate fresh world, requires the displayed Runner receipt and records
browser errors; it captures no images and performs no visual judgement.
`python3 tools/single_recall_mutations.py OUT` requires fresh-DB assertion failures
when arrival reuse, failure notice/audit or old messenger claim is removed, and
restores source after every arm.

`python3 tools/recall_route_mutations.py OUT` provar recall/redirects riktiga
väg, auktoritativ position och namngivna HTTP-/kuriravslag. Varje arm använder
ny PG16/mig160 med en ledig port, kräver assertion-rött och återställer källan
även vid fel; varje återställd arm måste bli grön. Kör i egen worktree.
Den riktiga spelarresan använder `single_recall_live.py` enligt receptet ovan.

`python3 tools/recall_eta_mutation.py OUT` provar det bevarade K4-ETA-skyddet
på aktuell OrderDelivery för recall och redirect. Varje arm ny PG16/mig160;
väggklockstimmar kräver namngivet assertion-rött, sedan återställning/grönt.
Rivningsrapport: `docs/reviews/riv-marchrecall/README.md`; livekontrollens
SQL körs via `psql -X -v ON_ERROR_STOP=1` i explicit READ ONLY-transaktion.

`python3 tools/verb_parity.py` inventerar registrerade spelar-API-rutter mot
Keryx-anrop, webbens megaron-moduler, Codex-omnämnanden och vaultens verblista.
Ingen DB eller produktionsändring; endast läsning. `--json` ger all källevidens,
`--verblista PATH` väljer annan registerfil, `--root PATH` väljer annan checkout.
Dokumenterade admin/post-MVP/transport-/templateundantag och bounded Codex-alias
bor i `tools/verb_parity_allowlist.json`. Olösta anrop visas separat; frånvaro
av statisk evidens är inte automatiskt frånvaro av förmåga. Fixturer:
`python3 -m unittest discover -s tools -p 'test_verb_parity.py'`.
Rapport och parsermutation: `docs/reviews/verb-parity/`.

`python3 tools/recall_all_live.py OUT COMMIT baseline|web|cli` uses private
fresh PG16/Redis, real register/join/found/march and existing per-unit recall.
Build temenos (stamp COMMIT) and keryx in OUT. Requires per-unit recall audits
and both units in garrison; SQL checks are read-only. Desktop/390px screenshots
in web mode; only own resources/config removed.
`python3 tools/recall_all_mutations.py OUT` physically limits each client loop
to the first unit, requires named JS/HTTP-test assertions red and restores
sources before green checks. No DB required for these client tests.

## Gemensam marschmeny (H)

Från eget worktree: bygg `server/cmd/server` med `-ldflags '-X main.buildCommit=<hash>'`
och `server/cmd/keryx` till `OUT/temenos` respektive `OUT/keryx` med rensad miljö
(`env -i HOME=/home/tk PATH="$PATH" go build …`). Kör sedan
`python3 tools/simple_march_live.py OUT <hash> map|war|land`. Varje körning skapar
egen färsk PG16/Redis, registrerar en spelare, grundar och skickar riktiga order
via webben. Desktop/390px, kvitton, healthz och read-only audit sparas i OUT.
Land väljer känt bart kustland (inte flod/ford); last skall stå på målet och
skeppet återvända. `baseline` tar samma menybilder och resa med gamla assets:
valfri fjärde parameter anger en arkiverad web-katalog.
`python3 tools/simple_march_mutations.py OUT` bryter fysiskt mängd/enhetslås/
kolonilast-body, kräver namngiven assertion och återställer/grönt efter varje arm.

## War-kort (I / återställning P)

Bygg `server/cmd/server` med rensad miljö till `OUT/temenos` och
`-ldflags '-X main.buildCommit=<hash>'`, från eget worktree.
`python3 tools/war_cards_live.py OUT <hash> after` skapar färsk PG16/Redis,
registrerar/grundar en verklig Wanax, öppnar More, ger sentry/fortify/clear genom
webben och skickar March/Recall. Förvald menuenhet och port-/landregler bevaras.
Desktop/390px-bilder av stängt/öppet More, anrop/kvitton, healthz, audit och
slutenhet sparas i OUT. Typad redirect öppnas men dispatchas inte av scenariot. Läget `naval` väljer
en kustgrundning genom vanliga joins och provar More→Load/Unload samt kontroll
av samma cargo tillbaka i garnison; det är en separat färsk arm.
Baslinje: samma kommando med `baseline` och fjärde argumentet en oförändrad
arkiverad web-katalog. Varje arm får egen tom DB och eget OUT.
`python3 tools/war_cards_mutations.py OUT` bryter ankomstvakten, More-placeringen
och navalrecall-vakten fysiskt; kräver namngivet assertionrött och restoredgrönt.
P använder `before-restoration` med arkiverade web-assets som bas, sedan
`restored` / `restored-naval`: kontroller ska synas direkt utan More.
Landarmarna före/efter P väljer en känd, nåbar destination via läsande
march-preview och marscherar utan expedition (Recall på senare Explore-ben
har ett separat, befintligt fel; se P-rapporten).
Korta expeditioners första ben kan avvisa Recall med korrekt 422 (ingen Runner
kan hinna ikapp); riggen väntar på nästa verkliga ben och sparar alla svar.


### Minnesgråskala (BILD)

`python3 tools/memory_gray_live.py OUT EXPECTED_COMMIT after` använder ny PG16/Redis,
rena servervariabler, vanliga register/join/founding/Explore-API:er och riktig webbsida.
OUT ska innehålla färskt byggd `temenos` med `-ldflags '-X main.buildCommit=<hash>'`.
Varje körning skapar egna containrar och städar dem; ingen SQL-fixtur.
Desktop/390px/minzoom, faktisk hover/inspect, grå/live/fog-centra, determinism,
health/kvitto/hemkommen enhet sparas. Baslinje: `baseline` som tredje argument,
fjärde argument en oförändrad arkiverad web-katalog; separat OUT/färsk DB för varje arm.

`python3 tools/memory_gray_replay.py PROOF_DIR BASELINE_WEB OUT` spelar upp samma
verkliga kartpayload med gammal/ny produktion-renderare och mäter containment,
full avmättnad och ljushet på opaka pixlar. Valfritt sista argument `collision`
är en uttryckligen syntetisk renderer-matris med terräng, städer och egen enhet.
UI-namn utelämnas där för att mäta enbart målade byggnader, inte överlagrade texter.
`python3 tools/memory_gray_pixels.py OUT` mäter Chromium-canvasens faktiska färgpixlar.
`python3 tools/memory_gray_mutations.py OUT [PROOF_DIR BASELINE_WEB]` bryter tier,
saturation och (med valfria argument) det faktiska kartpasset, kräver namngivet rött
följt av återställt grönt. Produktion återställs även vid fel.

## Production (J, BILD)

Bygg `server/cmd/server` med ren miljö och `-ldflags '-X main.buildCommit=<hash>'`
till `OUT/temenos`. `python3 tools/production_live.py OUT <hash> after` skapar
färsk PG16/Redis, registrerar/grundar och provar arbetsrutnätets −1/+1, More,
dagshistorik och slakt genom webben. Desktop/390px inklusive matrad, More och
Economy, health/kvitton och ingen SQL-mutation sparas. `baseline` och fjärde
argumentet arkiverad oförändrad web-katalog gör samma handlingar före ändringen.
`python3 tools/production_mutations.py OUT` bryter stängd More, coverage-källa
och reservtröskel; kräver namngiven röd assertion och återställt grönt per arm.

## Spelartext och inline-dialoger (K, BILD)

`python3 tools/text_live.py OUT EXPECTED_COMMIT after` kör ny PG16/Redis, rena
servervariabler och vanliga register/join/founding. OUT ska ha byggd `temenos`
med `-ldflags '-X main.buildCommit=<hash>'`. Sex drawers desktop/390, sex
brief-help till exakt Codexartikel, Locked-tooltip/help och verkliga cancel404/
rite400 visas inline; inga browserdialoger/fel. Historiska Sitos-grupper är
uttryckligen syntetisk API-replay, och Abandon-bilden är en syntetisk
DOM-fixtur med samma produktionshelper, ingen stad/DB/abandonorder fabriceras.
`baseline` + fjärde argument arkiverad web-katalog tar oförändrade förebilder.
`python3 tools/text_mutations.py OUT` kräver namngivet rött→återställt grönt
för rå grupptext, tempo, Abandon före bekräftelse och ritesvar före refresh.

## Economy overview id (BILD)

`python3 tools/economy_id_live.py OUT EXPECTED_COMMIT baseline|after [WEB_DIR]`
skapar egen ny PG16/Redis och använder register/join/founding. OUT ska ha
`temenos` byggd med ren miljö och `-ldflags '-X main.buildCommit=<hash>'`.
Baslinjen använder arkiverade oförändrade assets och kräver `no data` trots
positiv population/foodstocks/coverage; efterarmen jämför hela översiktsraden
med serverns settlement-data och kontrollerar province-id i City-länken.
Desktop390, health/API/DOM och inga SQL-mutationer sparas.
`python3 tools/economy_id_mutation.py OUT` återinför det gamla uppslaget fysiskt
och kräver namngivet rött i faktiska drawer-kedjan, sedan återställt grönt.

## Economy→City och grundning inline (BILD)

`python3 tools/drawer_founding_live.py OUT EXPECTED_COMMIT baseline|after [WEB_DIR]`
kör ny PG16/Redis med ren miljö, register/join och verkligt kartklick på Host.
OUT ska ha renbyggd `temenos` med `-ldflags '-X main.buildCommit=<hash>'`;
baslinjen får arkiverade oförändrade web-assets. Avbryt skickar ingen order;
bekräftelsen skickar en POST med oförändrad body, grundar och reloadar.
Economy→City kontrolleras utan manuell stängning efter fix. Desktop390,
health/API/DOM/bilder sparas. Alla requests når riktiga servern; browserns
HTTP-cache är avstängd för omladdningsprovet. Chromium använder `/dev/shm`
(Playwrights disable-dev-shm-usage tas bort), Host/mapdata och två frames
väntas in före koordinatklick. Inga SQL-fixturer.
`python3 tools/drawer_founding_mutation.py OUT` muterar close, browserdialog,
pending-vakt och gemensam dubbelskicksvakt fysiskt: named röd→återställd grön.

## Inspect i spelarord (L, BILD)

`python3 tools/inspect_live.py OUT EXPECTED_COMMIT baseline|after [WEB_DIR]`
kör fresh PG16/Redis/ren miljö och vanliga register/join. OUT ska ha
renbyggd `temenos` med `-ldflags '-X main.buildCommit=<hash>'`; baseline
får arkiverade oförändrade web-assets. Väljer två vanliga spawns på samma
landmassa via **skrivskyddad SELECT** i egna DB:n; ingen fixtur eller ändring
av speldata/positioner. Host vandrar sedan genom kända hexer med verkliga
march-preview/march-API:er tills staden syns enligt normal FOW. Webb: Host,
främmande stad, Send Messenger201, March here→samma dest och riktig fog
utan ägare/försvar. Desktop390/bilder/health/API/DB-läsmetadata sparas.
Chromium använder `/dev/shm`, CDP stänger cache; kamera centreras för klick
men inget kart-/unit-/markerdata injiceras.
`python3 tools/inspect_mutation.py OUT` ger fyra namngivna röda→återställda
gröna: återinförd /24, dold försvararfrånvaro, sent svar i fog och Culture-block.

### Diplomacy M — två flikar, bevarade brev/erbjudanden

`tools/diplomacy_live.py OUT BUILD_COMMIT baseline|after [WEB_DIR]` använder
färsk PG16/Redis och två vanliga register/join-spelare. Bygg OUT/temenos från
server/ med ren `env -i HOME/PATH` och `-ldflags '-X main.buildCommit=<hash>'`.
Baslinjen kan läsa arkiverad oförändrad web/. Vanlig Host-marsch och grundning,
första brevet via Compose respektive Known→Write, mottagarens Reply, fysiskt
återvänt läsbart svar, buy/sell-offert från tråden och Inspect-brev med 201.
Desktop1280/390 BILD; inga SQL-skrivningar eller injicerad klientkunskap.
`tools/diplomacy_mutation.py` provar fyra fysiska konsumentmutationer och
återställer alltid. `tools/diplomacy_scope.py` verifierar oförändrade befintliga
handels-/svarskonsumenter och server/Inspect/Gossip/CSS. Bevis/avgränsningar:
`docs/reviews/forenkling-diplomacy/README.md`.

## N — Economy varurader

`python3 tools/economy_goods_live.py OUT BUILD_COMMIT baseline|after [WEB_DIR]`
kräver OUT/temenos byggd i ren miljö med angiven `main.buildCommit`. Varje arm
startar egen ny PG16/Redis, migrerar och verifierar `/healthz` (commit/160/ok).
Registrering, grundning och kolonisering använder vanliga spelar-API:er och
FOW-känd terräng; inga SQL-skrivningar eller injicerad klientkunskap. Riktig
webb skapar flervaruorder och vanlig Transfer, sparar POST-body/kvittot och
BILD desktop/mobil. Efterarmen prövar Add/Remove och fältens läsbara palett.
Använd arkiverad bas-web för baseline. Egna processer/containrar rivs i finally.

`python3 tools/economy_goods_mutation.py` gör fyra fysiska källmutationer,
kräver namngivet actual-handler-prov rött och restaurerad svit grön, och
återställer i finally. Kör före livearmen så dess källor är stabila.
`python3 tools/economy_goods_scope.py` verifierar oförändrad server/API/CSS,
Crewed-by, POST-block och befintliga route actions. JS-prov:
`node --test web/static/js/megaron/ui/drawers/economy_rows.test.mjs`.

Riggbinärer och Go-byggtemporärer kan ligga under egen `/home/tk/.cache/`
vid /tmp-kvot; bevis ska kopieras till review-katalogen innan egna riggar städas.

`python3 tools/economy_goods_layout.py` är ett separat browser/CSS-konsumentprov,
inte gameplaybevis: kör den verkliga row-renderern från historiska commits och
aktuell kod med riktiga `megaron.css`. Det reproducerar blek fälttext (8b2) och
nowrap-overflow (dcba) och kräver läsbar/innesluten aktuell rad vid båda drawerbredder.
Ingen källmutation eller injicerad spelkunskap; körs även när en livearm pågår.


## Enkel Host-panel (Q)

`python3 tools/host_simple_live.py OUT FULL_HASH after` kräver en serverbinär
byggd med den fasta hashen som `OUT/temenos`. Ny PG16/Redis, ren miljö,
vanlig register/join → Host via faktiskt kartklick → stängd/öppen Details
→ inlinegrundning avbryt (ingen POST) → confirm (en POST201, body `{}`) →
stad/Host borta. Summary jämförs med verklig colonize-preview, prognosen
arkiveras med status/healthz, bilder desktop390 och kvitton.
Bas: `baseline ARCHIVED_WEB` i egen OUT/ny DB.
`python3 tools/host_simple_mutations.py OUT` bryter net-bedömning, stängd
Details, stone-källa, timber→lumber och late-response-vakt; named assertionrött och grönt
efter återställning krävs. Alla verktyg/cache/worktrees under HOME.

Q väljer för slutarmen en vanlig joined Host vars riktiga GET-prognos har
`goods.timber > 0`; `Timber: yes` måste synas. Actual-handler-testet läser
också arkiverad verklig forecast i `docs/reviews/host-enkel/fixtures/` så
varunyckeln inte kan valideras bara mot en stubb med samma felstavning.


## Umami för varje webbverb

- `python3 tools/umami_verb_mutations.py`: tar bort en verklig katalograd, success-track och API-koppling var för sig; kräver namngivet rött och återställer byte för byte, sedan grön telemetrisvit. Kör i egen arbetskopia utan samtidiga ändringar i API/verb_telemetry.
- `python3 tools/umami_verb_browser.py`: Firefox kör verkliga join-/inspect-kontroller med scriptade HTTP-svar. Verifierar headers/payload, utkast/feltext/redirect och exakt ett godkänt/refused-event; inga spel-DB-skrivningar eller externa analytics-anrop.
- `python3 tools/umami_verb_proof.py --send --output OUT`: skickar ett `{test:1}`-event per nytt namn via vanlig Firefox-UA och bekräftar mottagning med SELECT i Umami CT105. `--send` kräver uttryckligt tillstånd för testevent; utan flaggan inventeras bara. Ingen direkt DB-skrivning eller radering. Bevishem: `docs/reviews/umami-verb/README.md`.

### Day language and BILD proof

`tools/day_guard.py web|keryx|codex|server` checks player time vocabulary against exact, counted infrastructure exceptions. Suite entry points are `ui/day_guard.test.mjs`, `cmd/keryx/day_guard_test.go` and `internal/tick/day_guard_test.go`.

`python3 tools/day_mutations.py` physically changes production literals, requires named red tests, restores byte-identical source and requires green. `python3 tools/day_browser.py` exercises real Economy/War/Host/notification controllers and CSS with explicitly scripted HTTP, human fixture names, Firefox first then Chromium/WebKit desktop/390. It produces both calendar ordinal alternatives; it neither chooses one nor claims live game acceptance. Proof and investigation-row outcomes: `docs/reviews/day/README.md`.

## U — mobilkartan

- `python3 tools/mobilkarta_browser.py`: Firefox först, sedan Chromium/WebKit; desktop och touch 390×844. Produktionskarta/CSS/marschmeny/Codex med namngivna, skrivskyddade HTTP-fixturer. Native mus på desktop och tap på mobil; programmerade pointer-gester i alla motorer, dessutom äkta CDP-drag/nyp/långtryck i Chromium. Browserbegränsningar och BILD finns i `docs/reviews/mobilkarta/README.md`.
- `--baseline --source-root BAS_WORKTREE --output OUT`: samma prov mot oförändrad bas-web; kräver oflyttad kamera och för täta mobilmenynamn. `--browsers firefox` väljer motor.
- `python3 tools/mobilkarta_mutations.py`: fyra fysiska mutationer, namngivet rött, byte-identisk återställning i finally, därefter full JS grön. Kräver egen worktree utan samtidiga browserkörningar mot dess källor. CSS-mutationen gör den explicita scrollproben rullande med äkta Chromium-touch och fångar pointercancel.

U:s toppradstillägg: `python3 tools/mobilkarta_browser.py --topbar-only --output OUT` provar alla tre riktiga knappar med native tap/click, rect/hit-test och alla kalendernamn i tre motorer desktop/390. Original main.js-bryggor med enbart start-IIFE ersatt av State/HTTP-fixturen. `python3 tools/mobilkarta_topbar_mutation.py` tar fysiskt bort mobilens CSS-reservation, kräver namngivet rött rect-prov och återställer byte-identiskt. Ny BILD/proof i `docs/reviews/mobilkarta/topbar-v2/README.md`.

## Webbläsarparitet

`python3 tools/browser_parity.py --out DIR` loggar in i acceptansvärlden (`tools/acceptance.sh`, spelare
`Agamemnon` med grundad stad) och öppnar kartan och varje låda/fönster via sin riktiga knapp i Firefox,
Chromium och WebKit, desktop 1280 och mobil 390. Den mäter i stället för att jämföra bilder: sidled-scroll,
canvas som inte följer sin behållare, klippta/utstickande element, knappar som täcks, JS-fel och
innehållshöjd per motor (>15 % från medianen flaggas). Exit 1 vid fynd; bilderna hamnar i `DIR`.
Första körningen (2026-10-09) fann att WebKit ritade kartan som en remsa på 182 px (9 av 10 laddningar).
