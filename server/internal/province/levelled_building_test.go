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

// TestLevelledSpec_HigherLevelsCostCedar — själva mekaniken: att bygga ut en
// arbetsplats kräver ädelträ, alltså handel eller kolonisering.
func TestLevelledSpec_HigherLevelsCostCedar(t *testing.T) {
	for level := 2; level <= MaxBuildingLevel; level++ {
		spec, ok := LevelledSpec(BuildingLumbermill, level)
		if !ok {
			t.Fatalf("lumbermill level %d must resolve", level)
		}
		if spec.Costs["cedar"] <= 0 {
			t.Errorf("level %d must cost cedar, got %.1f", level, spec.Costs["cedar"])
		}
	}
	l2, _ := LevelledSpec(BuildingLumbermill, 2)
	l3, _ := LevelledSpec(BuildingLumbermill, 3)
	if l3.Costs["cedar"] <= l2.Costs["cedar"] {
		t.Error("each level must cost more cedar than the one below it")
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
