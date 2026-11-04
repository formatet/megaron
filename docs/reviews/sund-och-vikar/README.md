# Sund och vikar — utredning 2026-11-02

Utlöst av Timothy: "Lilla sund" (anchor 51,39) på live-världen är en enda hex. Mättes på live-världen
(`e1ed7d0c`, 64×64) + 28 genererade kartor (12× 64×95, 8× 56×40, 8× 64×64, olika seeds). Skript och
export-program i den här mappen (`sund*.py` kör mot `maps/*.json`; `probe_main.go.txt` skapar kartorna).

## Fynd 1 — nuvarande `strait` hittar vikar, inte sund
`world/places.go shapePlaces`: en öppen sjöhex med land på två motsatta sidor, kedja av sådana, **min 1**.
- Live: 23 sundhexar, **23 av 23 är vikar** enligt fiskeregelns definition (kustvatten med land på ≥4 av 6 sidor,
  `economy/hex_rules.go:144`), 23 kedjor, alla på 1 hex.
- 28 kartor: 90–100 % av sundhexarna är vikhexar; **~95 % av kedjorna är exakt 1 hex**.
- Båda namngivna "sund" i live — Lilla sund (51,39) och Nausikaa's Passage (56,25) — är vikar. (56,25) är en
  insjövik mitt i en ö, ingen passage alls. Båda är claimade av spelare.
- Generatorns golv `minStraitsFor` (`mapgen.go countStraits`) använder samma regel, så "sundgolvet" garanterar
  i praktiken vikar. Testet `debugpng_test.go` dokumenterar redan att måttet är ett randartefakt.

## Fynd 2 — land på två sidor med ETT hexs gap mellan två olika landmassor finns nästan inte
Generatorn håller landmassor isär. Utan vikar och med kravet "två olika landmassor" och gap 1 hex: median 1 per karta, max 4, ofta noll. Riktiga
sund är 2–4 hexar breda. Det som på live-bilden ser ut som ett sund (kanalen mellan fastlandet och tvåhexsholmen
vid (53,37)/(54,36), hexarna runt (51,39)–(53,38)) hittas inte alls av dagens regel som sund.

## Mått som fungerar — avståndssumma
Sjöhex (coastal/deep, inte lake) är **sundhex** om summan av hexavståndet (genom sjö) till de två närmaste
**olika** landmassorna är ≤ W+1. (Gap på W hexar → alla hexar på rak linje mellan dem har summan W+1.)
Landmassa = sammanhängande icke-sjö-komponent (flod/vad räknas som land, som i dag). Sammanhängande sundhexar = ett sund.

| W | sund/karta (min / median / max) | kartor utan sund | kedjelängd median / p90 / max | live |
|---|---|---|---|---|
| ≤1 | 0 / 2 / 5 | ej mätt | 2 / 2 / 3 | 1 |
| ≤2 | 0 / 3 / 6 | 2 av 29 | 6 / 9 / 14 | 1 (8 hex) |
| ≤3 | 0 / 4 / 11 | 2 av 29 | 12 / 19 / 36 | 3 (6, 9, 14 hex) |
| ≤4 | 0 / 5 / 12 | ej mätt | 20 / 35 / 75 | 3 |

Kedjorna sväller med W (mynningarna flänsar ut). Landmassestorlek ≥3 hex tar bort live-sundet och många
verkliga öpassager (var fjärde landmassa har ≤4 hex; 99 av 434 har ≤3) → **inget storlekskrav**.

## Skala
Hexskalan är inte dokumenterad i km. Galär: 3 kusthexar/speldygn (`web/static/codex/marching.md`), dvs.
ungefär 10–20 km per hex om ett kustsegel är 30–60 km/dygn (min uppskattning, osäker). Då är ett gap på
1 hex ≈ Messina/Gibraltar/Bonifacio (3–16 km), 2–3 hex ≈ Kythira–Kreta (≈35 km) till Otranto (≈72 km).
W≤3 ligger därmed vid den övre kanten av vad som kan kallas sund; bredare är en havspassage.

## Generatorgolvet — rör inte
Om `minStraitsFor` räknade riktiga sund klarar bara 8/28 (W≤2) respektive 17/28 (W≤3) kartor dagens golv —
det skulle sätta fart på omseedningen. Golvet är en annan fråga (kartkvalitet) än platsnamn; lämna det.
