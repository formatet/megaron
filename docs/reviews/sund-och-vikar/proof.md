# Bevispaket — sund och vikar (2026-11-04)

Gren `sonnet/sund-och-vikar`. W = `world.StraitMaxWidth` = 3, vik = `world.BayMinLand` = 4. Migration 239.

## Mått Go mot Python (sund4.py, W=3, minland 1)
28 genererade kartor (12x 64x95, 8x 56x40, 8x 64x64, seeds enligt probe_main.go.txt):
Go och Python ger SAMMA antal sund på alla 28 (inga avvikelser). min 0, median 4, max 11.
Live-världen (e1ed7d0c, 64x64): 3 sund (6, 9, 14 hex) och 23 vikar (alla en hex).

## Vik inom sund (överlapp)
Hexar som är vikhex enligt fiskeregeln OCH sundhex (tillhör sundet): 0-4 per karta, 25 av ~640 vikhexar totalt
(~4 %); 7 av 28 kartor har två eller fler. Enstaka, ingen stopp. Livevärlden: 1 hex ((51,39)).

## Paritet vik
`economy.TestBayParityWithWorld` (internal/economy/bay_parity_test.go), 10 kartor 56x40:
varje hex där `rulesWithLandNeighbors` fördubblar fisket är vik eller sund i `world.Places`; varje vik-hex har regeln;
ingen hex är både vik och sund.

## Migration 239, namn på live (SELECT 2026-11-04, ej körd)
Före:
| kind | anchor | name | claimed |
|---|---|---|---|
| strait | 56,25 | Nausikaa's Passage | ja |
| strait | 51,39 | Lilla sund | ja |

Efter (bay, namn och claim kvar). Matchning mot `Places()` på live-kartan:
- bay 56,25: matchar en vik (en hex). OK.
- bay 51,39: matchar INGEN vik. Hexen ligger nu i sundet med anchor 51,36 (14 hex: 51,36 51,37 51,39 52,36 ...).
  Raden lämnas (inget raderas) och flaggas till Claude/Timothy: flytta namnet till sundet (51,36) eller låt det stå som föräldralöst.
Live efter: strait-ankare (34,41), (51,36), (57,20); 23 viks-ankare.

## Mutanter (detached worktree, `places_shapes_test.go`-rad = dödare)
| mutant | dödad av |
|---|---|
| W+1 -> W | TestPlacesStraitWidthLimit :163 |
| `<=` -> `<` | TestPlacesStraitWidthLimit :163 |
| kravet "olika landmassor" borttaget (land delas per hex) | TestPlacesInletOfOneLandmassIsNotStrait :195, TestPlacesStraitIsTwoLandmassesWithNarrowGap :144, TestPlacesMapEdgeIsNotLand :230 |
| coastal_sea-kravet borttaget på bay | TestPlacesLakeAndDeepSeaAreNotBays :211 |
| sjö (lake) räknas som öppet vatten (och bay utan coastal) | TestPlacesLakeAndDeepSeaAreNotBays :211, TestPlacesLakeBetweenLandmassesIsNotStrait :246 |
En kontroll av sjötyp inne i `straitHexes` var ekvivalent (anroparen filtrerar) och togs bort.
