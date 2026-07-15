package unit

import "testing"

// The host is defined as much by what it cannot do as by what it is: it carries a
// people, it cannot fight, and it may found the one metropolis. These gates are the
// only thing standing between "founder phase" and "an unkillable 4 000-man army".
func TestNomadicHost_CannotFight(t *testing.T) {
	if CombatCapable(TypeNomadicHost) {
		t.Fatal("the nomadic host must never be combat-capable")
	}
	for _, ty := range []Type{TypeSpearman, TypeEliteInfantry, TypeWarChariot, TypeGalley, TypeWarGalley} {
		if !CombatCapable(ty) {
			t.Fatalf("%s lost its combat capability", ty)
		}
	}
}

func TestNomadicHost_AloneMayFoundMetropolis(t *testing.T) {
	if !CanFoundMetropolis(TypeNomadicHost) {
		t.Fatal("the nomadic host must be able to found the metropolis")
	}
	for _, ty := range []Type{TypeSpearman, TypeGalley, TypeMerchantman, TypeEliteInfantry} {
		if CanFoundMetropolis(ty) {
			t.Fatalf("%s must not be able to found a metropolis", ty)
		}
	}
}

func TestNomadicHost_SeesOneHexAndMovesAtHalfSpeed(t *testing.T) {
	if got := FOVFor(TypeNomadicHost); got != 1 {
		t.Fatalf("host FOV = %d, want exactly 1 hex", got)
	}
	if got := FOVFor(TypeSpearman); got != 2 {
		t.Fatalf("spearman FOV = %d, want the land baseline 2", got)
	}
	if got := SpeedFactorFor(TypeNomadicHost); got != 0.5 {
		t.Fatalf("host speed factor = %v, want half a spearman's 0.5", got)
	}
	if got := SpeedFactorFor(TypeSpearman); got != 1.0 {
		t.Fatalf("spearman speed factor = %v, want the baseline 1.0", got)
	}
}

// The host is one movable marker; its 4 000 people live in founder_phase.population.
// If it ever routes through the naval branch it could board a ship — which the
// design forbids outright.
func TestNomadicHost_IsALandUnit(t *testing.T) {
	if got := CategoryOf(TypeNomadicHost); got != CategoryLand {
		t.Fatalf("host category = %s, want land (it may never cross the sea)", got)
	}
	if got := CrewFor(TypeNomadicHost); got != 0 {
		t.Fatalf("host crew = %d, want 0 (it is not a vessel)", got)
	}
}

func TestNomadicHost_HasCanonicalDisplayName(t *testing.T) {
	if got := DisplayName(string(TypeNomadicHost)); got != "Nomadic Host" {
		t.Fatalf("DisplayName(nomadic_host) = %q, want %q", got, "Nomadic Host")
	}
}
