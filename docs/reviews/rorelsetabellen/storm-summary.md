# Stormsimulering före/efter RÖ1

| Terrain / färd | Hex | Restid före → efter (ticks) | Förlis före → efter |
|---|---:|---:|---:|
| coastal_sea / galley | 10 | 4 → 3 | 0.00% → 0.00% |
| coastal_sea / galley | 20 | 8 → 7 | 0.04% → 0.01% |
| coastal_sea / galley | 40 | 16 → 13 | 0.68% → 0.35% |
| coastal_sea / war_galley | 10 | 2 → 2 | 0.00% → 0.00% |
| coastal_sea / war_galley | 20 | 5 → 4 | 0.00% → 0.00% |
| coastal_sea / war_galley | 40 | 10 → 8 | 0.13% → 0.01% |
| coastal_sea / cargo | 10 | 6 → 5 | 0.00% → 0.00% |
| coastal_sea / cargo | 20 | 12 → 10 | 0.10% → 0.07% |
| coastal_sea / cargo | 40 | 24 → 20 | 1.80% → 1.40% |
| deep_sea / galley | 10 | 7 → 2 | 0.00% → 0.00% |
| deep_sea / galley | 20 | 14 → 4 | 0.19% → 0.00% |
| deep_sea / galley | 40 | 28 → 8 | 2.14% → 0.01% |
| deep_sea / war_galley | 10 | 4 → 1 | 0.00% → 0.00% |
| deep_sea / war_galley | 20 | 8 → 2 | 0.04% → 0.00% |
| deep_sea / war_galley | 40 | 17 → 5 | 0.75% → 0.01% |
| deep_sea / cargo | 10 | 11 → 3 | 0.07% → 0.00% |
| deep_sea / cargo | 20 | 21 → 6 | 0.50% → 0.00% |
| deep_sea / cargo | 40 | 42 → 12 | 4.21% → 0.14% |

Repro: `tools/ro1_storm_simulation.sh`. Före = b0762ebf, efter = bc6a7139.
500 oberoende väderseeds × 20 parade banor per seed = 10 000 resor per rad.
Varje arm använder sina egna produktionskostnader/planners; samma väder och banor.
80×40 rent syntetiskt hav, 32 stormar (1/100), tre hexar/storm,
skrov 5, skada 2, full besättning. Avfärd i tick 0–20, rak bana; väder simuleras till tick 70.
Seedserie för väder 2026101200–2026101699; banor 100000–100499.
Galär/krigsgalär läser verklig `legTravelTicks` och `BuildRoute`; frakt läser
`PlanTradeJourney` och `journeyRoute`. Verklig `seedShape`, `StepStorm` och
`RouteEnterMilli`. Den rena analysen speglar scanens `holds/inStorm` och dess
heltick-loop: skada vid stormträff om föregående tick inte var stormträff,
inklusive ankomsttick. Ingen SQL-state eller ändring av stormregler.

**Tolkning:** snabbare skepp samplas färre gånger. Nuvarande produktion samplar
position vid hela ticks; en stormhex som passerats helt mellan två ticks blir
inte ett exponeringstillfälle. RÖ1 gör detta särskilt tydligt i djuphavet.
500 vädervärldar är oberoende, de 20 resorna inom en värld är korrelerade;
procenten är observerad simulering, ingen exakt populationsrisk. Noll observerade
förlisningar betyder inte att risken är noll. Kartans naturliga kustgeometri,
blandterräng, insikt/orderfördröjning och spelarnas kursval ingår inte.

Planens historiska T1-mål var 0,3% på 20 sjöhex och 4,8% på 40.
Under nuvarande täthet 1/100 låg redan denna basgalärsbaslinje under det
(0,19% / 2,14% i djuphav); RÖ1 blir ännu lägre (0% / 0,01%).
Planens andra siffror med 1/36 och den gamla −1-skadan beskriver andra armar.
**Ingen tuning av stormarnas täthet, rörelse, skada eller scan gjordes.**
Claude har fått resultaten och avgör om uppföljande balansmätning ska beställas.
