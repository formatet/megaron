package handlers

import (
	"fmt"

	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/unit"
)

// stormHullWarning tells a Wanax, before and as a ship sails, what any storm
// would do to it at its present hull. It names the rule and the ship's own
// state only — never a storm the Wanax has not seen (Timothy 2026-11-01).
// "" for land units and for ships that would survive a storm.
func stormHullWarning(u *unit.Unit) string {
	if u == nil || unit.CategoryOf(u.Type) != unit.CategoryNaval || u.Hull > combat.StormDamage {
		return ""
	}
	return fmt.Sprintf("Hull %d of %d: any storm this ship meets will sink it (a storm takes %d hull). Unseen storms may lie on the way; repair at a shipyard first.", u.Hull, combat.HullCap(u.HullBonus), combat.StormDamage)
}

// stormShipWarning is stormHullWarning for a ship known only by type and hull
// (a free ship picked for a transfer, a ship named in a passage order); bonus is its units.hull_bonus.
func stormShipWarning(shipType string, hull, bonus int) string {
	return stormHullWarning(&unit.Unit{Type: unit.Type(shipType), Hull: hull, HullBonus: bonus})
}
