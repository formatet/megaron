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

// Vision lives in province.LiveRadius (tested there); this covers movement only.
func TestNomadicHost_MovesAtHalfSpeed(t *testing.T) {
	// Guards the direction of the multiplier: it scales HOURS, so the host's
	// number must be ABOVE the baseline. A value below 1.0 would silently make the
	// slowest thing in the game the fastest.
	host, base := MarchHoursFactorFor(TypeNomadicHost), MarchHoursFactorFor(TypeSpearman)
	if host != 2.0 {
		t.Fatalf("host march-hours factor = %v, want 2.0 (double a spearman's hours = half its speed)", host)
	}
	if base != 1.0 {
		t.Fatalf("spearman march-hours factor = %v, want the baseline 1.0", base)
	}
	if host <= base {
		t.Fatalf("host factor %v must EXCEED the baseline %v — it scales hours, not speed", host, base)
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

// Locked decision (Timothy 2026-07-15): the host may not embark in MVP. The
// size<100 forming gate blocked it only by accident (size=1); this is the rule.
func TestNomadicHost_CannotEmbark(t *testing.T) {
	if CanEmbark(TypeNomadicHost) {
		t.Fatal("the nomadic host must never be able to embark")
	}
	for _, ty := range []Type{TypeSpearman, TypeEliteInfantry, TypeWarChariot} {
		if !CanEmbark(ty) {
			t.Fatalf("%s must still be able to embark", ty)
		}
	}
}

func TestNomadicHost_HasCanonicalDisplayName(t *testing.T) {
	if got := DisplayName(string(TypeNomadicHost)); got != "Nomadic Host" {
		t.Fatalf("DisplayName(nomadic_host) = %q, want %q", got, "Nomadic Host")
	}
}
