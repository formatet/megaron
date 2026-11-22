# Striden i Monte Carlo (2026-11-22)

Mätning, ingen kodändring. Harnessen `montecarlo_test.go.txt` (döp om till `zz_mc_test.go` i
`server/internal/combat/` för att köra den, och committa den inte) speglar rundloopen i `resolveTick`:
deltagande 0,90, tre rundor per tick, murens budget per tick, storm, flykt vid 0,20 och utplåning.
Den anropar de riktiga `rollSide`, `distributeLosses` och `sideRouts` med `NewSeededDice`.
4000 strider per scenario, 400 per punkt i murtabellen. Grundenheten är `spearman`, som slår ceil(n/2) tärningar.

## Fynd
**Antalet avgör.** 100 mot 100 ger 49/48. 110 mot 100 ger 88 % anfallarseger, 130 mot 100 ger 100 %, 330 mot 300 ger 98 %.
Många T12-tärningar ger nästan inget brus, så striden följer Lanchesters kvadratlag. Lika strider tar ungefär 11 ticks.

**Kharis och välsignelser** verkar som motsvarande andel fler män. Lika styrka: favor (×1,15) ger 80 %, wrath (×0,85) ger 15 %,
Velchanos (×1,25) ger 92 %, hornen (×1,5 i första rundan) ger 58 %. De avgör jämna strider men vänder inte ett underläge på 30 %.

**Muren är en fast sköld.** Den sväljer 20 träffar per nivå och tick. En spjutman ger ungefär 0,11 träffar per tick,
så en nivå väger ungefär 170 anfallare oavsett hur stor garnisonen är. Anfallaren behöver följande överläge för att vinna minst hälften av striderna:

| försvarare | mur 1 | mur 2 | mur 3 |
|---|---|---|---|
| 30 | 6,2:1 | 11,5:1 | >12:1 |
| 100 | 2,7:1 | 4,4:1 | 6,1:1 |
| 300 | 1,6:1 | 2,2:1 | 2,8:1 |
| 600 | 1,3:1 | 1,6:1 | 1,9:1 |

Under tröskeln förlorar försvararna ingen alls: 100 mot 30 bakom mur 1 slutar med 0 döda på murarna, och anfallaren förlorar 82 man.
Över tröskeln märks muren knappt. Storm (halverad budget, anfallarens förluster ×1,5) hjälper bara stora anfallare.
Vägen in är utsvältning (`siege_capitulation.go`).

**Elitstyrkor och vagnar** slår många tärningar men har ett liv per man eller vagn. 100 elitsoldater förlorar mot 300 spjutmän
och vinner mot 100 bakom mur 2 i ungefär hälften av fallen. 20 vagnar utplånas av 100 spjutmän på ungefär ett tick. 40 vagnar vinner.

**Avrundning av modifierare** (`math.Round` i `rollSide`) spelar bara roll för deltagare på 1–2 man. Inget fel.
