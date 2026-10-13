# Stormscan — varje passerad sjöhex

## Sammanfattning

Problem: den gamla scanen läste bara en hex per helt tick och behandlade två
olika storm-ID över en tickgräns som samma storm. Nu läses varje sparat
RouteEnterMilli-inträde samt den hex ett långsamt skepp håller när vädret driver.
Samma sammanhängande storm-ID ger ett slag; olika stormar ger egna slag.

Bas `cfba184b`, migration179. Ingen migration, ny notis eller grafik.
Fyra transportärenden delar samma scan; ordinarie flottmarscher gör det också.
Stillhetens ägar-/avfärds-/ankomststämpel och frysta rutter ändras inte.
Skada2, fullt skrov5, täthet1/100, väderdrift och sikt är oförändrade.

## Kontrakt och baslinje

[Serverkontrakt](server-contract.md). Kedjegrind: bevisar fysiska sjötransporter.
Färsk combat-baslinje på179 grön, `baseline.log`.
Den oförändrade gamla RÖ1-simuleringen sparades före kodändringen i
`storm-before.json`; uppdaterat verktygs legacy-arm reproducerar den exakt.
Ett första simkommando hade fel relativ loggsökväg från server/ och kördes inte;
omkört med absolut utdata, ingen kodreparation eller röd normalisering.

## Semantik och prov

`stormEncounters` används av verklig scan och simulationens efter-arm.
Tidsintervallet är (t−1,t], vädret läses på t, ursprungshexen lämnas direkt
enligt befintlig rutmodell. Sparad position vid föregående tick avgör fortsatt
storm-ID; en ny klar hex eller ett helt klart stillastående tick avslutar mötet.
Om vädret flyttar samma storm från föregående hex till nästa är det samma möte,
inte en ny skada varje gång stormen kryper framåt.

Riktig DB/handler: mellanhex träffas, samma storm över två hex ger ett slag,
två olika stormar samma tick ger två, Stillhet ger noll, dubbelscan ger ingen ny
debitering. Samma mellanhexprov körs för trade/transfer/gift/trade_return.
Sen scan skriver utfallet på ursprungligt tick och inte på den sena scanens tick.
Frysta marschrutter bevaras. Separata tidsprov täcker långsamt skepp, skilda ID
över tickgräns, fraktionella inträden, landankomst och ingen retroaktiv skada.

Äldre stormfixturer beskrev samma storm men gav varje lane-hex olika ID och
bara en av tre obligatoriska trackslots. Nu har sammanhängande lanegrupper
ett ID och tre sammanhängande slotpositioner; långt möte använder driftande
trehexstorm. De gamla röda proven finns i `targeted-initial.log` och
`targeted-correct-tracks.log`; inga svagare skadekrav eller skips införda.

## Parade mätningar — ingen tuning

[Alla 20-/40-hexresultat](storm-comparison.md), 500 oberoende väderfrön,
20 parade färder/frö, 10 000 färder per fall. Samma väder, startlägen,
frysta rutter och restider i båda armarna. Före: historisk heltickscan.
Efter: verkliga gemensamma encounterfunktionen.

Djuphav galär: förlis20hex **0 →1,11 %**, 40hex **0,01 →6,70 %**.
Djuphav frakt: **0 →1,40 %** och **0,14 →7,30 %**.
Referens från gamla T1-målet:0,3/4,8 % på20/40hex. Dessa är historiska
referenser från den gamla riskmodellen, ingen ny gräns för dagens tvåskrovskada.
Alla stormtal lämnas enligt ordern. Detta är syntetisk sjö, ingen uppmätt
förlisandel i den levande alfavärlden.

Även `tools/ro1_storm_simulation.sh` fungerar fortfarande: det kopierar nu den
gemensamma scan-hjälparen till sin låsta före-RÖ1-baseline, så jämförelsen av
rörelsetabellerna inte bryts av det nya verktygets funktionsanrop.

## Grindar och reproduktion

| Grind | Bevis |
|---|---|
| Kod | Riktad färsk combat179 grön; slutlig full färsk Go179/world92,929s, vet och620JS gröna (egna loggar). |
| Mutation | Tick-only samt upprepad-samma-storm: fysiskt namedRED → identiska bytes → varsin färskGREEN. |
| Semantik | Alla transportärenden, riktiga sparade inträden, exakt ID och utfallstick, samma eventpayload som före. |
| Användare | Ingen ny användaråtgärd/API/layout. Verklig scan och observerbara damage/founder-notiser genom DB/handlerproven; befintliga formatterfixture- och Codexprov. Ingen naturlig väderträff eller browsernylayout påstås. |
| Bild | Ingen BILD enligt ordern; Codex `sea` och `storms` beskriver passerade hexar. |
| Drift | Integrations-/healthkvitto i agentloggen och vaultens normala driftstatus efter deploy. |

```sh
tools/gotest.sh ./internal/combat -run 'TestStormEntered|TestSeaStorm|TestBetaStillness' -v
python3 docs/reviews/stormscan-varje-hex/mutate.py
bash tools/stormscan_simulation.sh
bash tools/ro1_storm_simulation.sh docs/reviews/stormscan-varje-hex/ro1
tools/gotest.sh
```

Skadan och eventet sker atomiskt med befintligt per-voyage progressmärke.
En ny voyage retroträffas inte för ticks före dess första scan; tidigare
weather-retention är oförändrad. Markernas befintliga schema återanvänds.
