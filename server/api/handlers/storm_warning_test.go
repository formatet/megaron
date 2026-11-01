package handlers

import (
	"strings"
	"testing"

	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/unit"
)

// Nausikaa's galley sailed at hull 1 and sank in its first storm: the warning
// is about the ship's own hull, never about a storm she had not seen.
func TestStormHullWarning(t *testing.T) {
	ship := func(h int) *unit.Unit { return &unit.Unit{Type: unit.TypeGalley, Hull: h} }
	if w := stormHullWarning(ship(combat.StormDamage)); !strings.Contains(w, "will sink it") {
		t.Fatalf("a hull a storm would take must warn: %q", w)
	}
	if w := stormHullWarning(ship(combat.StormDamage + 1)); w != "" {
		t.Fatalf("a ship that survives one storm needs no warning: %q", w)
	}
	if w := stormHullWarning(&unit.Unit{Type: "spearman", Hull: 1}); w != "" {
		t.Fatalf("land units have no hull to warn about: %q", w)
	}
}
