# Lite fler tennkällor

| Fält | Kontrakt |
|---|---|
| Problem | Tennets plana källtak motverkar Timothys nya beslut om lite fler källor och skalning med kartan. |
| Spelarsanning | Nya världar får lite mer tenn, med samma geografi och bronsberoende över vatten. |
| Invariant | Befintliga världar orörda; alla MapTile-fält utom TinDeposit bevarade per effektiv seed; koppar och tenn på skilda landmassor, samma terräng/reachability/separation/klusterstorlek. |
| Scope | world/mapgen, riktade prov, reproducerbar 20-seed-mätning på 64² och 128², berörda vaultpåståenden. |
| Acceptans | Minsta uppmätta ökning omkring +50% tennhex på 64²; växer med kartan; geografin/övriga deposits identiska; validateMap och full färsk Go/vet gröna; fysisk mutation gammalt tal rött→återställt grönt. |
| Stopvillkor | Kanonkonflikt parkeras i BESLUT; oberoende beställt arbete fortsätter. |
| Bevisplan | Oförändrad baseline + 3 kandidater i separata källkopior; fullfältshash utom TinDeposit och effektiva seeds; regression/mutation; READ ONLY före/efter deploy. |

Kedjegrind: **blockerar** beställd justering av resursknappheten före test. Ingen omfördelning eller närhetsgaranti.

## Mätning före implementation

Bas `2be784c2`; verklig GenerateMap, seeds1009×1..20 på64² och128², inga DB-ingrepp. Kluster = sammanhängande tennkomponenter, hex = faktiska TinDeposit-fält. Källbudgeten är antal fröförsök; kluster kan växa ihop eller kandidater ta slut.

| Variant | Storlek | Tennhex/snitt | Kluster/snitt | Ökning hex | Samma effektiva seeds | Samtliga non-Tin fält bevarade |
|---|---:|---:|---:|---:|---:|---:|
| baseline | 64² | 4.45 | 2.60 | +0.0% | 20/20 | 20/20 |
| baseline | 128² | 4.15 | 2.80 | +0.0% | 20/20 | 20/20 |
| floor3-cap6 | 64² | 6.20 | 2.95 | +39.3% | 20/20 | 2/20 |
| floor3-cap6 | 128² | 6.15 | 3.50 | +48.2% | 20/20 | 2/20 |
| floor4-cap8 | 64² | 7.55 | 3.30 | +69.7% | 20/20 | 1/20 |
| floor4-cap8 | 128² | 7.70 | 4.05 | +85.5% | 20/20 | 1/20 |
| area3 | 64² | 6.00 | 2.75 | +34.8% | 20/20 | 2/20 |
| area3 | 128² | 18.60 | 5.80 | +348.2% | 20/20 | 1/20 |
| post1 | 64² | 5.95 | 2.85 | +33.7% | 20/20 | 20/20 |
| post1 | 128² | 10.70 | 4.90 | +157.8% | 20/20 | 20/20 |
| post2 | 64² | 7.40 | 3.10 | +66.3% | 20/20 | 20/20 |
| post2 | 128² | 15.95 | 5.85 | +284.3% | 20/20 | 20/20 |
| post3 | 64² | 8.60 | 3.25 | +93.3% | 20/20 | 20/20 |
| post3 | 128² | 20.55 | 6.10 | +395.2% | 20/20 | 20/20 |

**Val: post2**, minsta prövade hela tilläggsbudget som når minst cirka+50% på64².
Post1 når bara+33,7%; post3 är större än behövligt. Direkta byten av gamla golv/tak förkastas,
eftersom de flyttar silver via gemensam RNG och ändrad silverkandidatlista. Area3 i primärpasset
har samma fel. Post2 behåller originalkartan och gamla tennhex och tillför 2 klusterförsök på64²,
8 på128²: `max(1, round(2×area/4096))`. Ingen närhets-/täckningsgaranti eller ny separation.
Tenn växer i östra regionens befintliga nåbara kalkberg; redan tagna metallhex används inte.
Den etablerade koppar/tenn-sjödistansen kontrolleras före tillägg, med samma funktion som validateMap.
Det gamla fyrtaket finns kvar endast i originalpassens kompatibilitet, för att inte konvertera
annan terräng eller ta bort silver i remote-isle-steget; det är inte längre den totala budgeten.

## Implementation och bevis

- `GenerateMap` validerar originalkartan före tillägg; samma effektiva seed används. Den extra passens
  RNG `seed ^ 0x54494e` förbrukar ingen slump från den gamla sekvensen. Oförändrad `placeDepositClusters`,
  1–3 hex per fröförsök och samma minSpacing0. Inga gamla TinDeposit-flaggor raderas.
- `compare.py` kopierar verklig produktionskod till isolerad modul och jämför med baseline och vald
  kandidatmätning: 40/40 byteidentiska JSON-resultat mot den förmätta post2, 40/40 non-Tin-hash/effektiva seeds
  och gamla tennpunkter bevarade. Faktisk slutlig `validateMap` grön på40/40.
- `TestGenerateMap_TinIncreasePreservesGeography` använder baseline-golden från oförändrad master, inte
  en andra genomkörning av den nya hjälparen. Exakta tennhex/kluster per seed från förmätningen kontrolleras.
- Fysisk mutation tar bort extra passens anrop: verklig gammal tenn-generering → named RED, originalbytes
  återställda exakt → nytt grönt prov på separat färsk PG16. `mutation.log` och båda armloggarna.
- Riktad oförändrad masterbaseline på egen tom PG16/mig179: TinCapAndSpread + Reachability gröna (28,081s).
  Ny riktad 40-kartorsregression + budgetskalning gröna (7,447s), på en annan tom DB.

## Grindar

| Grind | Status |
|---|---|
| Kod | Full färsk Go/mig179 grön (world150,163s), `go vet ./...` grön. |
| Semantik | Gamla och nya världar åtskilda; inga gamla kartor skrivs om. Samma bronsgeografi, ingen närhetsgaranti. |
| Funktion/användare | Verklig GenerateMap över40 kartor, samma generator som world.NewWorld; detta är inte ett spelat bronsscenario. |
| Visuell | Ingen renderingsändring, ingen BILD-grind. |
| Drift | Före-snapshot av alla befintliga map_tiles + historiska events + Hostsilver sparad READ ONLY; efterkvitto tillfogas. |

## Reproduktion

```sh
python3 docs/reviews/tenn-lite-fler/measure.py
python3 docs/reviews/tenn-lite-fler/compare.py
python3 docs/reviews/tenn-lite-fler/mutate.py
tools/gotest.sh
env -i HOME="$HOME" PATH="$PATH" go -C server vet ./...
```

Mätningen modifierar endast temporära källkopior från bascommiten. Ingen produktionskod eller DB
skrivs av analysskripten. Sammanfattning i `summary.json`, per-seed-resultat i `*.jsonl`.

## Kända avgränsningar

+66,3% är minsta **prövade hela klusterförsöksbudget** som når minst ungefär+50%; inte en exakt50%-garanti
på varje seed. På128² ökar snitthex+284,3% från den gamla plana nivån. Befintlig tenngeografi på dev
är orörd; ingen täckningsgaranti och inget bevis om att främlingar klarar bronskedjan.
