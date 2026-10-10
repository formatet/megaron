package handlers

import (
	"strings"
	"testing"

	"formatet/megaron/server/internal/province"
)

// One harbour rule, told the same everywhere: the forecast note, the refusal
// and the gated building types all come from the province package.
func TestHarbourNote_InlandAndCoastal(t *testing.T) {
	if got := harbourNote(false); got != province.HarbourRuleInland || !strings.Contains(got, "can never build a harbour or ships") {
		t.Fatalf("inland site must say it can never build ships, got %q", got)
	}
	if got := harbourNote(true); got != province.HarbourRuleCoastal {
		t.Fatalf("coastal site: got %q", got)
	}
}

func TestHarbourRule_GatedTypesAndRefusal(t *testing.T) {
	for _, bt := range []province.BuildingType{province.BuildingHarbour, province.BuildingShipyard} {
		if !province.NeedsWaterNeighbour(bt) {
			t.Fatalf("%s must be gated by the harbour rule", bt)
		}
		if msg := province.InlandBuildRefusal(bt); !strings.Contains(msg, "this city is inland; found a colony on the coast to build ships") {
			t.Fatalf("refusal lacks the way out: %q", msg)
		}
	}
	if province.NeedsWaterNeighbour(province.BuildingFarm) {
		t.Fatal("farm is not gated by the harbour rule")
	}
}
