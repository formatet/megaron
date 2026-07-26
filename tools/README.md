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

## Arbetsregler

1. En visuell sak per iteration. Skärmdump före/efter. Behåll bara det som förbättrar.
2. Diffa alltid — pixeldiffen bevisar vad som ändrades.
3. Determinism är krav: två körningar utan kodändring → 0 ändrade pixlar.
4. Enhetsriggen är acceptansgrind.
5. Mät innan du påstår.
