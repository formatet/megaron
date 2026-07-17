package capabilities

import (
	"fmt"

	"github.com/poleia/server/internal/province"
)

// canMarch requires a garrisoned unit at this settlement. Status is the gate, not
// size: a 'garrison' unit is finished and can march (a battle-worn cohort under
// 100 men included); 'forming'/'training' units are still maturing and excluded.
func canMarch(cc checkContext) Verb {
	var n int
	if cc.hasSettlement() {
		_ = cc.pool.QueryRow(cc.ctx,
			`SELECT count(*) FROM units
			 WHERE settlement_id = $1 AND owner_id = $2 AND status = 'garrison'`,
			cc.settlementID, cc.playerID,
		).Scan(&n)
	}
	ok := n > 0
	return verb("march", CategoryMilitary,
		"Order a garrisoned unit to march to a hex you have seen (live or remembered); intent \"explore\" may push into unseen land.",
		[]Requirement{
			req("a garrisoned unit here", ok,
				fmt.Sprintf("%d deployable unit(s) here", n),
				"recruit or build a unit here first — only garrisoned units can march"),
		})
}

func canRecall(cc checkContext) Verb {
	n := cc.marchingUnits()
	ok := n > 0
	return verb("recall", CategoryMilitary,
		"Send a recall order (by messenger) to a marching unit, turning it home.",
		[]Requirement{
			req("a unit currently marching", ok,
				fmt.Sprintf("%d unit(s) marching", n),
				"march a unit first — recall only applies to units already in transit"),
		})
}

func canRedirect(cc checkContext) Verb {
	n := cc.marchingUnits()
	ok := n > 0
	return verb("redirect", CategoryMilitary,
		"Send a redirect order (by messenger) to a marching unit, giving it a new destination.",
		[]Requirement{
			req("a unit currently marching", ok,
				fmt.Sprintf("%d unit(s) marching", n),
				"march a unit first — redirect only applies to units already in transit"),
		})
}

func canStance(cc checkContext) Verb {
	n := cc.anyUnitsHere()
	ok := n > 0
	return verb("stance", CategoryMilitary,
		"Set or clear a unit's stance (fortify/storm/sentry).",
		[]Requirement{
			req("a unit garrisoned or forming here", ok,
				fmt.Sprintf("%d unit(s) here", n),
				"recruit a unit in this settlement first"),
		})
}

// canLoad requires an idle (no-cargo) garrisoned ship AND a full-strength
// garrisoned land unit, both at this settlement — unit.go Load's own gate.
// TODO: Fas 3 unify with handler gate.
func canLoad(cc checkContext) Verb {
	ships := cc.idleNavalUnits()
	landUnits := cc.deployableLandUnits()
	shipOK := ships > 0
	landOK := landUnits > 0
	return verb("load", CategoryMilitary,
		"Embark a land unit onto a ship in the same settlement.",
		[]Requirement{
			req("an idle ship garrisoned here (no cargo)", shipOK,
				fmt.Sprintf("%d idle ship(s) here", ships),
				"build/recruit a ship here (requires harbour)"),
			req("a full-strength land unit garrisoned here (>=100 men)", landOK,
				fmt.Sprintf("%d/1 deployable land unit(s) here", landUnits),
				"recruit 100 men of one land type in this settlement"),
		})
}

func canUnload(cc checkContext) Verb {
	n := cc.ladenNavalUnits()
	ok := n > 0
	return verb("unload", CategoryMilitary,
		"Disembark the cargo unit from a ship in this settlement.",
		[]Requirement{
			req("a ship garrisoned here carrying cargo", ok,
				fmt.Sprintf("%d laden ship(s) here", n),
				"load a land unit onto a ship here first"),
		})
}

func canDisband(cc checkContext) Verb {
	n := cc.anyUnitsHere()
	ok := n > 0
	return verb("disband", CategoryMilitary,
		"Release units back to civilian population.",
		[]Requirement{
			req("a unit garrisoned or forming here", ok,
				fmt.Sprintf("%d unit(s) here", n),
				"recruit a unit in this settlement first"),
		})
}

// CanColonize exposes canColonize to api/handlers.UnitHandler.March, whose
// colonize-intent precondition uses it (Fas 3 anti-drift).
func CanColonize(cc checkContext) Verb { return canColonize(cc) }

// SettlementCapRequirement exposes settlementCapRequirement to
// UnitHandler.March. March already validates, per the SPECIFIC unit being
// dispatched, that it is a deployable (>=100 men, garrison-or-positioned)
// land unit — canColonize's "deployable land unit garrisoned here" requirement
// is an AGGREGATE (any unit at the settlement) that would wrongly reject a
// positioned unit (already off any settlement, mid-journey) that is
// perfectly valid to colonize with. Only the settlement-cap piece — which
// depends solely on worldID/playerID, not on which unit is marching — maps
// 1:1 onto March's own check, so it is split out and reused directly rather
// than routing through the whole verb's Available flag.
func SettlementCapRequirement(cc checkContext) Requirement { return cc.settlementCapRequirement() }

// settlementCapRequirement is colonize's "room under the settlement cap" gate.
func (cc checkContext) settlementCapRequirement() Requirement {
	total, _ := cc.ownSettlements()
	capOK := total < province.MaxSettlementsPerWanax
	return req(fmt.Sprintf("under the settlement cap (%d)", province.MaxSettlementsPerWanax), capOK,
		fmt.Sprintf("%d/%d settlements held", total, province.MaxSettlementsPerWanax),
		"abandon or consolidate a settlement before founding another")
}

// canColonize is the keystone example from temenos_capabilities.md: a
// garrisoned (deployable) land unit plus headroom under the per-Wanax
// settlement cap.
func canColonize(cc checkContext) Verb {
	deployable := cc.deployableLandUnits()
	deployableOK := deployable > 0

	return verb("colonize", CategoryMilitary,
		"March a garrisoned land unit to an empty hex with intent=colonize to found a new settlement there.",
		[]Requirement{
			req("a deployable land unit garrisoned here", deployableOK,
				fmt.Sprintf("%d/1 deployable", deployable),
				"recruit a land unit in this settlement, then march it with --intent colonize"),
			cc.settlementCapRequirement(),
		})
}
