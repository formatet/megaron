# Oförklarade extra (LIVE 5faac0a, 2026-11-22)

Order: `.agents/order-oforklarade-extra-20261122.md`. BILD efter merge i acceptansvärlden på `5faac0a`, Firefox 1:1, 1280×800. Skript: `bild.py`.

- `a-hexrow.png`: "copper 1 per game day on this hex" (var "total").
- `b-kharis.png`: "Kharis 1.0 · Wrathful (Suspicious at 5)". Nästa band uppåt, tal ur `religion.Mood*`.
- `c-silver.png`: "Silver 120 · pays professional forces · from a silver mine or trade" (samma svans i `keryx goods`).
- Grundningsnotisen "Poseidon grants a Galley (War › Ships)" och "(keryx army)" i keryx är bara testade. Acceptansstaden ligger i inlandet och fick Demeters gård.
- `d1/d2/d3-hammer-*.png`: ⚒ **ej åtgärdad**. Orderns premiss (zoomgrinden) var fel: grinden är 0,4 och standardzoomen 1.
  Den verkliga orsaken: en ny stad är alltid idle, och idle-stämpeln är en prickad ruta på 3×3 px ovanför staden (`d3`, 4× förstoring).
  Fråga till Timothy (BILD): större eller tydligare idle-märke, en etikett, eller ta bort knappen?

Prov: rött före/grönt efter (Sonnet) · mutanter: kharis `>=`→`>` (Sonnet), silversvansen bort (Opus), båda dödade · webb 1052/0 · Go-svit grön.
Bifynd för NU 4: Kult-fliken visar "−0.33" (två decimaler).

## ⚒ åtgärdad (LIVE 75dc435, 2026-11-22)
Timothy: "kör på omdömet, nittiotal". Knappen är borttagen. Egna städer visar alltid en hammare när de bygger och ett spjut när de tränar, ritat i 2×. En stad som står stilla visar inget.
`e1-stamp-building.png` (1:1) och `e2-stamp-building-zoom4x.png`: Hagia Triada bygger ett Granary, och hammaren (10×12 px) står till vänster om flaggan. Skript: `bild_hammare.py`. Helsviten körd av Opus: rc=0.
