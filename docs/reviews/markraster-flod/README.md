# Markraster: flodmark och mynning (slice A)

Mynningen stannade vid hexkantens mitt medan kusten förvrängdes; flodmarkens två undantag i `markHexAt` gav samtidigt ett rakt hexband. Flodmarken använder nu samma förvrängda uppslagning som annan mark. Fåran bakas efter mark och strand, med eget heltalspixelklipp, och havsgrenens slutpunkt fortsätter längs sin tangent till hav i den förvrängda masken.

Status: byggd för granskning på `codex/markraster-flod`, bas `0d149589`. Inte mergad eller deployad. Detta är en syntetisk renderingsrigg; ingen backend-, databas- eller verkligt spel-verifiering påstås. BILD lämnas till Claude/Timothy enligt arbetsordern. Kedjegrind: bevisar kartans geografiska läsbarhet.

## Slice-kontrakt

| Fält | Kontrakt |
|---|---|
| Problem | Fåran når inte den förvrängda kusten och flodmarkens oförvrängda gräns syns som hexband. |
| Spelarsanning | Samma synliga flodkedja når havet och flodmarken följer landskapet. |
| Invariant | Behåll kedjegraf, fårans bredd, färger, meander, vad och WARP-styrka; ingen fåra över okänd mark. |
| Scope | `render/map.js`, dess regressionsprov, reproducerbara bevisverktyg och rapport. |
| Non-scope | Worker, flodnamn, spelvärldsfixtur, hexöverlägg, ikoner och all annan terrängdesign. |
| Acceptans | Mynning i havsmask; mark förvrängd; Firefox 1:1/närbild; JS grön; verkliga mutationer röda. |
| Stopp | Nytt utseendebeslut utöver felrättningen kräver Timothy; efter hash inväntas START. |
| Bevis | Baslinje, 558 JS-prov, riktade mutationer, frysta Firefox-bilder, determinism och inneslutningsdiff. |

## Baslinje och resultat

- Oförändrad bas: 555/555 JS-prov, utan skips ([logg](baseline-js.log)).
- Slutkod: 558/558, utan skips ([logg](final-js.log)); tre nya regressioner.
- Mynning: 48 kombinationer av sex riktningar, fyra världssalt och river/river_ford når en havspixel.
- Mark: grannmark kan gå in i flodhexen och flodmark kan gå ut; fysisk fåras bläck tillåts över inträngande känd mark, klipps vid hav och dimma.
- Minnesmasken omfattar separat fåras faktiskt ritade bläck, klippt mot dess pixelregion, och hela den förvrängda minnesmarken. Den lägger inte till en grå fysisk hex.
- Mutationer: återinfört exakt flodundantag, återinfört förvrängt flodundantag och ursprunglig slutpunkt ger namngivet rött. Byte-identisk återställning och 18/18 riktade prov gröna. [Exakt mark](mutation-exact-ground.log), [förvrängd mark](mutation-warped-ground.log), [mynning](mutation-mouth.log), [återställt](restored.log).
- Firefox: ingen konsol/page error; två separata efterladdningar pixeldiff 0. World, vanlig forest, relief och cities har pixeldiff 0 mot basens `map.js`, serverad via Playwright-route. Flodfixturen ändrar 6059 skärmpixlar. [Resultat](browser.json).

## Bilder — Firefox först, device scale 1

Primär bedömning vid kamerazoom 1,0 (renderarens ordinarie SCALE=2), ingen extra bildskalning:

| | Före | Efter |
|---|---|---|
| Helvy 1:1 | [Före](before.png) | [Efter](after.png) |
| Mynning, felsökning 3× nearest-neighbour | [Före](before-closeup.png) | [Efter](after-closeup.png) |

[Normal riggzoom 1,6](river-zoom-1.6.png) · [minzoom 0,3](river-zoom-0.3.png). Gula hexöverlägg, Kydonia och Phaistos är den befintliga fixturen och har inte ändrats. Närbilden är diagnostik, inte bedömningsskala. Området omfattar flod/hav, dal/slätt, berg, vad, städer och befintliga överlägg.

## Repro

Från arbetskopian, servern i separat terminal:

```sh
python3 -m http.server 8742 --directory web
python3 tools/markraster_flod.py --host http://localhost:8742
python3 tools/markraster_flod_mutation.py
node --test web/static/js/megaron/**/*.test.mjs
```

Verktygen behöver Python Playwright, Firefox och Pillow. Bildverktyget använder `git show 0d149589:…/map.js` som låst bas; det byter inte arbetskopians produktionsfil. Mutationverktyget återställer i `finally` och kontrollerar byte-identitet. Källa: [hashlista](sources.sha256).

## Grindar och begränsningar

| Grind | Utfall |
|---|---|
| Kod | 558 JS gröna, tre verkliga mutationer, diffcheck. Ingen Go-/SQL-/malländring. |
| Visuell | Firefox fullvy/närbild, spelzoom/minzoom, determinism och fyra oförändrade riggar. BILD väntar granskning. |
| Semantik | Mark följer förvrängningen; fåran följer den kända kedjan och ritar inte på hav/dimma. |
| Användare | Syntetisk showcase, inte ett speltest. Verkliga spelet ligger i separat beställd slice C. |
| Drift | Ingen merge, push eller deploy. |

Ingen ny färg, täthet eller bredd. Oförändrade riggar saknar flodhexar och verifierar därför inneslutning, inte verklig flodgeografi. Första visningens synkrona bakning ligger kvar till worker-slicen. Kanalpixelns semantik i kustmasken är fortsatt baserad på den förvrängda flodregionen; fårans grafiska bläck är separat.
