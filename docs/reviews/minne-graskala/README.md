# Minnesrutor i gråskala — BILD

## Kontrakt före kod
Problem: minnesrutor har samma färg som aktuell syn. Spelarsanning: svart = aldrig sett, gråskala = minne, färg = aktuell syn. Bevisar geografidelen av kedjegrinden.
Invariant: endast serverns remembered avfärgas; ljushet och detaljer bevaras, inga server-/tier-/FOV-ändringar.
Scope: kartpass, canvas-hjälpare, JS-prov, webbrigg och BILD. Non-scope: synregler, fog-palett och nya förmågor.
Acceptans: remembered grå och live/fog oförändrade; hård pixelkant; terräng/byggnader grå före UI/enheter; inspect fungerar; riktig scouting/återresa med alla tre tier i desktop/390-bild.
Stopvillkor: informationsmodellkonflikt till Claude, ingen serverutvidgning.
Bevisplan: oförändrad master222f4e3f, tier-test/fysisk mutation, hela JS/färsk Go/vet, två isolerade scoutresor utan SQL-ingrepp och pixelmätning. Claude bedömer BILD.

## Baslinje
400 JS gröna på oförändrad master222f4e3f (logs/baseline-js.log). Ingen separat minimap: hex-canvas är kartan, city-scene är stadsbyggnadsvy.
