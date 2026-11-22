# Kanalen slingrar + uppsamlingsradien i kartvalideringen (2026-11-22)

**BILD efter merge** (arbetssätt: BILD väntar inte). Gäller bara nya världar. Inga befintliga kartor ändras.

## Vad
1. **Kanalens sänka slingrar sig.** Den hårda havskolumnen (`seaChannels`, 33 %/67 %) ligger kvar och skiljer
   fortfarande koppar från tenn. Sänkan runt kolumnen följer nu en mitt som vandrar bort från den
   (`channelMeanderFor`: amplitud `max(2, bredd/20)` kolumner, våglängd `max(10, höjd/6)` rader). Mellan kolumnen och
   mitten är sänkan på full djup, så kolumnen hamnar aldrig som ett en hex brett linjalsnitt genom land.
   Bandets kantbrus har kortare våglängd (8 → 4) och tre oktaver. Fyndet går tillbaka till Timothys ögonkoll
   2026-07-16 ("kanalerna läses som raka lodräta snitt"), som stått öppen sedan dess.
2. **Radiebugg från P1.** `validateMap`:s garanti för koppar i väst och tenn i öst, och testspegeln
   `SpawnOreCatchmentScore`, läste radie 1 (6 grannar). `join.go` och produktionen läser
   `hexgrid.CatchmentRadius` (2). Nu används `spawnCatchmentRadius = 2`, låst mot hexgrid av
   `TestSpawnCatchmentRadiusMatchesHexgrid`. world får inte importera hexgrid enligt G1.
3. **Guldtestet för tennpasset** (`testdata/tin-increase-2be784c2.json`) var oktoberslicens bevis för att inga
   befintliga kartor ändrades. Punkt 1 ändrar kartorna avsiktligt, så invarianten ("tennpasset lägger bara
   till tenn, rör inget annat fält, flyttar inget gammalt tenn") kontrolleras nu inom en och samma körning.

## Mått (samma seeds, master mot gren)
Rader i följd med oförändrat avstånd från kanalkolumnen till närmaste land. Ju längre sträcka, desto rakare kust.

| storlek | längsta raka sträcka, snitt (före → efter) | max | försök per karta |
|---|---|---|---|
| 40×40 (n=200) | 8,6 → 5,5 | 18 → 11 | 3,18 → 3,90 |
| 56×40 (n=200) | 8,3 → 5,2 | 19 → 12 | 3,42 → 3,38 |
| 115×115 (n=30) | 11,6 → 6,6 | 27 → 12 | 2,20 → 2,47 |
| 230×230 (n=12) | 12,4 → 8,0 | 18 → 12 | 1,92 → 3,42 |

Fler omslag vid 230×230 kommer främst från den befintliga vadställesinvarianten (ett vadställe med 3–4 flodgrannar).
Den fanns redan på master och kostar ungefär en sekund per extra försök. Taket är 100 försök; högsta uppmätta är 21 (40×40).

## Bilder
- `fore-efter-230.png`: seed 2000 vid 230×230. Vänster master, höger gren. Grenen slog om till 2009, så det är inte samma geografi.
  Jämför kanalerna: mastern har ett rakt dike nertill vid x≈2/3.
- `fore-40.png` / `efter-40.png`: sex alfakartor (40×40), samma begärda seeds. Några har slagits om.

## Prov
`go test ./internal/world/` grönt. Hela `tools/gotest.sh` på färsk DB före merge.
