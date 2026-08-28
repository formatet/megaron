package province

import "testing"

// TestLevelledSpec_Level1IsUnchanged — att bygga en arbetsplats första gången ska
// kosta exakt vad den alltid kostat. Nivåtrappan får inte fördyra grundbygget.
func TestLevelledSpec_Level1IsUnchanged(t *testing.T) {
	spec, ok := LevelledSpec(BuildingFarm, 1)
	if !ok {
		t.Fatal("farm level 1 must resolve")
	}
	base := BuildingSpecs[BuildingFarm]
	if len(spec.Costs) != len(base.Costs) {
		t.Fatalf("level 1 cost set changed: got %v, want %v", spec.Costs, base.Costs)
	}
	if _, hasCedar := spec.Costs["cedar"]; hasCedar {
		t.Error("level 1 must not cost cedar — cedar buys growth, not the first building")
	}
}

// TestLevelledSpec_CedarBuildingsCostCedar — ädelträmekaniken finns kvar, men
// bara för det maritima och monumentala (S2, 2026-08-27). Hamnen och varvet är
// fönstret mot havet; templet är palatsbygget.
func TestLevelledSpec_CedarBuildingsCostCedar(t *testing.T) {
	for bt := range LevelCedarBuildings {
		for level := 2; level <= MaxBuildingLevel; level++ {
			spec, ok := LevelledSpec(bt, level)
			if !ok {
				t.Fatalf("%s level %d must resolve", bt, level)
			}
			if spec.Costs["cedar"] <= 0 {
				t.Errorf("%s level %d must cost cedar, got %.3f", bt, level, spec.Costs["cedar"])
			}
		}
		l2, _ := LevelledSpec(bt, 2)
		l3, _ := LevelledSpec(bt, 3)
		if l3.Costs["cedar"] <= l2.Costs["cedar"] {
			t.Errorf("%s: each level must cost more cedar than the one below it", bt)
		}
	}
}

// TestLevelledSpec_BaseBuildingsNeedNoCedar är S2:s hela poäng
// (megaron_plan_dagsverkesskalan, 2026-08-27).
//
// Mätt 2026-08-27: ceder finns på 31 av världens 2 240 hexar och NOLL av dem
// ligger i någon stads upptagningsområde. När LevelCedarCost gällde varje
// nivåbyggnad var därför hela nivåtrappan låst för alla åtta städer i drift —
// samtliga 70 byggnader stod på nivå 1 sedan 2026-07-23, och det lästes länge
// som spelarslöhet snarare än som en spärr ingen kunde passera.
//
// En jordbruksstad måste kunna intensifiera sin mark med lokala material.
func TestLevelledSpec_BaseBuildingsNeedNoCedar(t *testing.T) {
	for bt := range LevelledBuildings {
		if LevelCedarBuildings[bt] {
			continue
		}
		for level := 2; level <= MaxBuildingLevel; level++ {
			spec, ok := LevelledSpec(bt, level)
			if !ok {
				t.Fatalf("%s level %d must resolve", bt, level)
			}
			if spec.Costs["cedar"] > 0 {
				t.Errorf("%s level %d costs %.3f cedar — base progression must be buildable "+
					"from local materials, or the whole level staircase locks again", bt, level, spec.Costs["cedar"])
			}
		}
	}
}

// TestLevelledSpec_MaterialsRiseWithLevel — utan ceder måste något annat bära
// trappan, annars kostar nivå 2 och 3 exakt samma material som nivå 1 och
// uppgraderingen blir gratis i allt utom tid.
func TestLevelledSpec_MaterialsRiseWithLevel(t *testing.T) {
	l1, _ := LevelledSpec(BuildingLumbermill, 1)
	l2, _ := LevelledSpec(BuildingLumbermill, 2)
	l3, _ := LevelledSpec(BuildingLumbermill, 3)

	for _, good := range []string{"timber", "stone"} {
		if l2.Costs[good] <= l1.Costs[good] {
			t.Errorf("%s: level 2 (%.3f) must cost more than level 1 (%.3f)", good, l2.Costs[good], l1.Costs[good])
		}
		if l3.Costs[good] <= l2.Costs[good] {
			t.Errorf("%s: level 3 (%.3f) must cost more than level 2 (%.3f)", good, l3.Costs[good], l2.Costs[good])
		}
	}
	if l3.DurationTicks <= l2.DurationTicks {
		t.Error("each level must take longer to build than the one below it")
	}
}

// TestLevelledSpec_DoesNotMutateCatalogue — BuildingSpecs är en delad global
// katalog. Om LevelledSpec skrev cedar rakt in i den skulle grundkostnaden för
// varje efterföljande bygge i processen växa. Regressionsvakt.
func TestLevelledSpec_DoesNotMutateCatalogue(t *testing.T) {
	before := len(BuildingSpecs[BuildingFarm].Costs)
	for level := 1; level <= MaxBuildingLevel; level++ {
		_, _ = LevelledSpec(BuildingFarm, level)
	}
	if _, leaked := BuildingSpecs[BuildingFarm].Costs["cedar"]; leaked {
		t.Fatal("LevelledSpec wrote cedar into the shared BuildingSpecs catalogue")
	}
	if after := len(BuildingSpecs[BuildingFarm].Costs); after != before {
		t.Fatalf("shared catalogue cost set grew from %d to %d entries", before, after)
	}
}

// TestLevelledSpec_RejectsOutOfRange — nivåtrappan har ett slut.
func TestLevelledSpec_RejectsOutOfRange(t *testing.T) {
	if _, ok := LevelledSpec(BuildingFarm, MaxBuildingLevel+1); ok {
		t.Error("a level above MaxBuildingLevel must not resolve")
	}
	if _, ok := LevelledSpec(BuildingFarm, 0); ok {
		t.Error("level 0 must not resolve")
	}
}

// TestLevelledBuildings_NonProducingStayFlat — barracks/foundry/wall producerar
// inget och har därför ingen arbetsplatskapacitet att växa. De ska inte gå att
// nivå-bygga (wall har sin egen trappa i WallLevelSpecs).
func TestLevelledBuildings_NonProducingStayFlat(t *testing.T) {
	for _, bt := range []BuildingType{BuildingBarracks, BuildingFoundry, BuildingWall} {
		if LevelledBuildings[bt] {
			t.Errorf("%s produces nothing — it must not be in LevelledBuildings", bt)
		}
		if _, ok := LevelledSpec(bt, 2); ok {
			t.Errorf("%s must not resolve a level-2 spec", bt)
		}
	}
}

// TestLevelledBuildings_TempleIsIncluded — templets nivå styr
// templeDevotionCapacity, men templet gick inte att uppgradera innan detta, så
// den mekaniken var inert. Vakt mot att det glider tillbaka.
func TestLevelledBuildings_TempleIsIncluded(t *testing.T) {
	if !LevelledBuildings[BuildingTemple] {
		t.Error("temple level drives templeDevotionCapacity — it must be upgradeable")
	}
}
