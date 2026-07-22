package capabilities

import (
	"fmt"

	"formatet/megaron/server/internal/religion"
)

// canRite checks the temple + kharis gate for this settlement's culture's
// default prayer (battle_frenzy) — the same pair SettlementHandler.Rite
// enforces before the cooldown/offering checks. TODO: Fas 3 unify with handler gate.
func canRite(cc checkContext) Verb {
	hasTemple := cc.hasBuilding("temple")
	culture := cc.cultureID()
	prayerID := religion.DefaultBattleFrenzyFor(culture)
	spec, ok := religion.PrayerSpecs[prayerID]
	minKharis := 0.0
	if ok {
		minKharis = spec.MinKharis
	}
	kharis := cc.kharisAmount()
	kharisOK := kharis >= minKharis

	return verb("rite", CategoryCult,
		"Perform a cultural prayer at your temple (kharis-gated; consumes a material offering).",
		[]Requirement{
			req("temple built", hasTemple,
				boolDetail(hasTemple, "temple built", "no temple"),
				"build a temple (`poleia build --type temple`)"),
			req(fmt.Sprintf("kharis >= %.0f (default prayer: %s)", minKharis, prayerID), kharisOK,
				fmt.Sprintf("kharis %.0f/%.0f", kharis, minKharis),
				"raise divine standing (feed diverse goods, avoid starvation) before casting"),
		})
}
