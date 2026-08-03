package world

import "testing"

// Cederkalibreringens grind, mätt före reseeden 2026-08-03 mot den NYA
// landytan (flodfamiljen föll från ~13 % till ~5 % av landet när
// flodbudgeten blev per landmassa). Rapport:
// megaron_cedermatning_20260803.md.
//
// Testet pinnar två saker och bara två:
//
//   - AREAN. cedarFractionTarget säger 3 % av landet. Bandet nedan är
//     2-4 % kring det, mätt som MEDEL över flera seeds — inte per karta.
//     Per karta är spridningen legitim (2,1-3,3 % uppmätt på 230²); ett
//     per-karta-band hade blivit ett flakigt test som faller på en seed
//     ingen tittat på. Medelvärdet är däremot det tal reseeden vilar på.
//
//   - FORMEN. Formmålet är "regioner man kan segla till och hålla", inte
//     utspridd dekor. Före cedarStandGrowthFloor bar en 230²-karta i snitt
//     3,2 bestånd på ≤9 hexar; efter är minsta uppmätta bestånd över 34
//     stora kartor 11 hexar. Golvet här är 10, alltså med marginal —
//     testet ska falla när golvet slutar verka, inte när en seed råkar
//     ligga nära.
//
// 160×160 och inte 230×230: samma area-skalade kodväg (landArea /
// cedarStandAreaDivisor ger 25 respektive 52 bestånd före taket), en
// tredjedel av kostnaden. Bandet är verifierat på båda storlekarna plus
// 120×120 innan det pinnades här.
func TestGenerateMap_CedarCalibrationBandAndStandShape(t *testing.T) {
	if testing.Short() {
		t.Skip("genererar åtta 160×160-kartor")
	}
	const (
		w, h      = 160, 160
		seeds     = 8
		fracMin   = 0.020
		fracMax   = 0.040
		standMin  = 10
		wantFloor = "cedarStandGrowthFloor"
	)

	sum := 0.0
	for seed := int64(0); seed < seeds; seed++ {
		tiles := genTiles(seed, w, h)
		land, cedar := 0, 0
		for _, t := range tiles {
			if tileIsLand(t.Terrain) {
				land++
			}
			if t.Terrain == TerrainForestCedar {
				cedar++
			}
		}
		if land == 0 {
			t.Fatalf("seed %d: noll landhexar", seed)
		}
		sum += float64(cedar) / float64(land)

		sizes := depositSourceSizes(tiles, func(t MapTile) bool { return t.Terrain == TerrainForestCedar })
		if len(sizes) == 0 {
			t.Fatalf("seed %d: inga cederbestånd alls", seed)
		}
		// Sista elementet är det minsta — depositSourceSizes sorterar fallande.
		if got := sizes[len(sizes)-1]; got < standMin {
			t.Errorf("seed %d: minsta cederbestånd %d hexar (vill ha >= %d) — %s verkar inte längre bita; alla: %v",
				seed, got, standMin, wantFloor, sizes)
		}
	}

	mean := sum / seeds
	if mean < fracMin || mean > fracMax {
		t.Errorf("cederandel av land, medel över %d seeds = %.4f, vill ha %.3f..%.3f (cedarFractionTarget = %.2f)",
			seeds, mean, fracMin, fracMax, cedarFractionTarget)
	}
}
