# tools — offline visuell rigg

Renderingsprinciperna och acceptansgrinden bor i `megaron_terrangrendering.md` (vault).
Här står bara HUR riggen körs.

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

Eget compose-projektnamn (`megaron-acc`), egna volymer, eget nätverk, egna portar, egen
JWT-hemlighet. Den kan inte råka röra dev-servern eller live-DB:n — det är hela poängen.
Runbook och det verifierade scenariot: `megaron_drift.md` §Acceptansvärlden.

## Arbetsregler

1. En visuell sak per iteration. Skärmdump före/efter. Behåll bara det som förbättrar.
2. Diffa alltid — pixeldiffen bevisar vad som ändrades.
3. Determinism är krav: två körningar utan kodändring → 0 ändrade pixlar.
4. Enhetsriggen är acceptansgrind.
5. Mät innan du påstår.
