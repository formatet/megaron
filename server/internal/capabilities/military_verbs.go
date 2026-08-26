package capabilities

import (
	"fmt"

	"formatet/megaron/server/internal/province"
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
	// Purpose-texten namnger BÅDA vägarna ut. Fram till 2026-08-26 nämnde den
	// bara "explore" — och det var den enda mening i hela actions-ytan som rörde
	// okänd mark, så varje spelare (och varje agent) grep efter den rundresa som
	// vänder hem igen. Mätt över två körningar: ingen spanare kom längre än ~6
	// hexar hemifrån, och noll av fyra spelare hittade en granne.
	return verb("march", CategoryMilitary,
		"Order a garrisoned unit to march to a hex you have seen (live or remembered). "+
			"A plain march leaves the unit standing where it arrives — that is how you gain "+
			"ground you keep seeing; add stance \"sentry\" (or use `keryx unit post`) to post "+
			"it as a forward watch. Intent \"explore\" is the opposite order: it may push into "+
			"unseen land, but the unit turns for home the moment it arrives.",
		[]Requirement{
			req("a garrisoned unit here", ok,
				fmt.Sprintf("%d deployable unit(s) here", n),
				"recruit or build a unit here first — only garrisoned units can march"),
		})
}

// canPost — den framskjutna posten som egen, namngiven yta. Mekaniskt är den
// `march` med stance sentry (keryx unit post är en tunn wrapper, precis som
// unit sentry är över --intent sentry för skepp), och den listas ändå för sig:
// vägen har funnits sedan sentry byggdes och ingenting pekade någonsin på den,
// vilket är hela roten under att utforskning aldrig blev en front.
//
// Kravet är LAND, inte bara "en enhet": ett skepp postas med `unit sentry` och
// har en patrulltimer som tar det hem igen — en helt annan order.
func canPost(cc checkContext) Verb {
	n := cc.deployableLandUnits()
	ok := n > 0
	return verb("post", CategoryMilitary,
		"March a land unit to a hex and hold it there as a forward watch — it stays "+
			"(no patrol timer, no auto-return), extends your fog-of-war from where it stands, "+
			"and intercepts enemy caravans within reach. It eats double grain in the field for "+
			"as long as it holds.",
		[]Requirement{
			req("a garrisoned land unit here", ok,
				fmt.Sprintf("%d deployable land unit(s) here", n),
				"recruit a land unit here first — a ship is posted with `unit sentry` instead, "+
					"which patrols and sails home on its own"),
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
	// Sagt vad de faktiskt GÖR, inte bara att de finns. "sentry" är den
	// framskjutna posten spelet aldrig pekade på.
	return verb("stance", CategoryMilitary,
		"Set or clear a unit's stance: \"fortify\" digs in and refuses to march, "+
			"\"storm\" presses an assault, \"sentry\" stands watch — it spots foreign marches "+
			"passing nearby and intercepts enemy caravans within reach. Note a unit does not "+
			"need sentry to SEE: anything standing on the map already extends your fog-of-war.",
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
				"build/recruit a ship here (requires shipyard)"),
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
