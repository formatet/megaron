package capabilities

import (
	"fmt"
	"time"
)

// canKingdomFound requires an active capital not already bound to a kingdom
// — KingdomHandler.Found's own gate. This is a player-scoped verb (kingdoms
// are per-Wanax, not per-province) surfaced with province context per
// temenos_capabilities.md.
func canKingdomFound(cc checkContext) Verb {
	_, _, kingdomID, hasCapital := cc.capitalSettlement()
	inKingdom := hasCapital && kingdomID != nil
	ok := hasCapital && !inKingdom
	detail := "no active capital in this world"
	if hasCapital {
		if inKingdom {
			detail = "already a member of a kingdom"
		} else {
			detail = "capital is unaligned"
		}
	}
	return verb("kingdom-found", CategoryKingdom,
		"Found a new kingdom, becoming its basileus.",
		[]Requirement{
			req("an active capital not already in a kingdom", ok, detail,
				"settle a capital and leave any existing kingdom before founding a new one"),
		})
}

func canKingdomInvite(cc checkContext) Verb {
	_, role, inKingdom := cc.kingdomRole()
	ok := inKingdom && role == "basileus"
	detail := "not in a kingdom"
	if inKingdom {
		detail = "role: " + role
	}
	return verb("kingdom-invite", CategoryKingdom,
		"Invite another Wanax to your kingdom — basileus only.",
		[]Requirement{
			req("you are basileus of a kingdom", ok, detail,
				"found a kingdom (kingdom-found), or wait to be elected basileus"),
		})
}

func canKingdomJoin(cc checkContext) Verb {
	_, provinceID, _, hasCapital := cc.capitalSettlement()
	ok := false
	detail := "no active capital in this world"
	if hasCapital {
		var n int
		_ = cc.pool.QueryRow(cc.ctx,
			`SELECT count(*) FROM kingdom_invitations
			 WHERE province_id = $1 AND accepted_at IS NULL AND expires_at > now()`,
			provinceID,
		).Scan(&n)
		ok = n > 0
		if n > 0 {
			detail = fmt.Sprintf("%d pending invitation(s)", n)
		} else {
			detail = "no pending invitation"
		}
	}
	return verb("kingdom-join", CategoryKingdom,
		"Join a kingdom you've been invited to.",
		[]Requirement{
			req("a pending kingdom invitation to your capital", ok, detail,
				"ask a basileus to invite you (kingdom-invite), then check `poleia kingdom-invitations`"),
		})
}

func canKingdomVote(cc checkContext) Verb {
	kingdomID, _, inKingdom := cc.kingdomRole()
	ok := false
	detail := "not in a kingdom"
	if inKingdom {
		var n int
		_ = cc.pool.QueryRow(cc.ctx,
			`SELECT count(*) FROM kingdom_elections WHERE kingdom_id = $1 AND resolved_at IS NULL AND closes_at > now()`,
			kingdomID,
		).Scan(&n)
		ok = n > 0
		if n > 0 {
			detail = "an election is open"
		} else {
			detail = "no open election"
		}
	}
	return verb("kingdom-vote", CategoryKingdom,
		"Cast your vote in an open basileus election.",
		[]Requirement{
			req("an open election in your kingdom", ok, detail,
				"call an election (kingdom-election-call) or wait for one to open"),
		})
}

// canKingdomElectionCall mirrors KingdomHandler.CallElection: caller must be
// a member, and elections may only be called on a Sunday (UTC).
func canKingdomElectionCall(cc checkContext) Verb {
	kingdomID, _, inKingdom := cc.kingdomRole()
	sunday := cc.clk.Now().UTC().Weekday() == time.Sunday
	noneOpen := true
	detail := "not in a kingdom"
	if inKingdom {
		var n int
		_ = cc.pool.QueryRow(cc.ctx,
			`SELECT count(*) FROM kingdom_elections WHERE kingdom_id = $1 AND resolved_at IS NULL AND closes_at > now()`,
			kingdomID,
		).Scan(&n)
		noneOpen = n == 0
		if noneOpen {
			detail = "no election currently open"
		} else {
			detail = "an election is already open"
		}
	}
	reqs := []Requirement{
		req("you are a kingdom member", inKingdom, detail,
			"found or join a kingdom before calling an election"),
	}
	if inKingdom {
		reqs = append(reqs, req("no election already open in your kingdom", noneOpen, detail,
			"wait for the current election to close"))
	}
	reqs = append(reqs, req("today is Sunday (UTC — elections only run weekly)", sunday,
		boolDetail(sunday, "today is Sunday", "today is "+cc.clk.Now().UTC().Weekday().String()),
		"wait until Sunday (UTC)"))
	return verb("kingdom-election-call", CategoryKingdom,
		"Call a basileus election for your kingdom (Sundays only).", reqs)
}

func canKingdomTreasuryDeposit(cc checkContext) Verb {
	_, _, _, hasCapital := cc.capitalSettlement()
	_, _, inKingdom := cc.kingdomRole()
	silver := 0.0
	if hasCapital {
		silver = cc.capitalSilver()
	}
	silverOK := silver > 0
	return verb("kingdom-treasury-deposit", CategoryKingdom,
		"Send silver from your capital to the kingdom treasury.",
		[]Requirement{
			req("you are a kingdom member", inKingdom,
				boolDetail(inKingdom, "member of a kingdom", "not in a kingdom"),
				"found or join a kingdom first"),
			req("silver at your capital to deposit", silverOK,
				fmt.Sprintf("silver %.0f", silver),
				"sell goods or tax production for silver first"),
		})
}

func canKingdomBorrowArmy(cc checkContext) Verb {
	kingdomID, role, inKingdom := cc.kingdomRole()
	isBasileus := inKingdom && role == "basileus"
	otherMembers := 0
	if isBasileus {
		_ = cc.pool.QueryRow(cc.ctx,
			`SELECT count(*) FROM kingdom_members WHERE kingdom_id = $1 AND player_id != $2`,
			kingdomID, cc.playerID,
		).Scan(&otherMembers)
	}
	lenderOK := otherMembers > 0
	return verb("kingdom-borrow-army", CategoryKingdom,
		"Borrow units from a kingdom member's settlement — basileus only.",
		[]Requirement{
			req("you are basileus of a kingdom", isBasileus,
				boolDetail(isBasileus, "role: basileus", "not basileus"),
				"found a kingdom or be elected basileus"),
			req("another kingdom member to borrow from", lenderOK,
				fmt.Sprintf("%d other member(s)", otherMembers),
				"invite another Wanax to your kingdom"),
		})
}

// canKingdomCouncil mirrors KingdomHandler.AssignRole (PATCH .../council/{role})
// — no CLI wrapper exists yet, but the route is registered and mutating, so
// per temenos_capabilities.md's "enumerate ALL from cmd_*.go + registered
// routes" it is surfaced here as `kingdom-council`.
func canKingdomCouncil(cc checkContext) Verb {
	kingdomID, role, inKingdom := cc.kingdomRole()
	isBasileus := inKingdom && role == "basileus"
	otherMembers := 0
	if isBasileus {
		_ = cc.pool.QueryRow(cc.ctx,
			`SELECT count(*) FROM kingdom_members WHERE kingdom_id = $1 AND player_id != $2`,
			kingdomID, cc.playerID,
		).Scan(&otherMembers)
	}
	targetOK := otherMembers > 0
	return verb("kingdom-council", CategoryKingdom,
		"Assign a council role (lochagos/navarchos/member) to a fellow kingdom member — basileus only.",
		[]Requirement{
			req("you are basileus of a kingdom", isBasileus,
				boolDetail(isBasileus, "role: basileus", "not basileus"),
				"found a kingdom or be elected basileus"),
			req("another kingdom member to assign a role to", targetOK,
				fmt.Sprintf("%d other member(s)", otherMembers),
				"invite another Wanax to your kingdom"),
		})
}

// capitalSilver returns the live silver stock at the player's capital.
func (cc checkContext) capitalSilver() float64 {
	settlementID, _, _, ok := cc.capitalSettlement()
	if !ok {
		return 0
	}
	var amt float64
	err := cc.pool.QueryRow(cc.ctx,
		`SELECT GREATEST(0, settled(amount, rate, calc_tick))
		   FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'`,
		settlementID,
	).Scan(&amt)
	if err != nil {
		return 0
	}
	return amt
}
