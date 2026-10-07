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
