package capabilities

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/province"
)

// checkContext bundles the inputs every checker needs. settlementID is
// resolved once by the caller (the /actions handler) — uuid.Nil means the
// province has no settlement (e.g. an unowned/outpost tile), in which case
// every settlement-scoped verb reports itself unsatisfied rather than erroring.
type checkContext struct {
	ctx          context.Context
	pool         *pgxpool.Pool
	clk          clock.Clock
	worldID      uuid.UUID
	provinceID   uuid.UUID
	playerID     uuid.UUID
	settlementID uuid.UUID
}

// NewContext builds the same checkContext List uses, for api/handlers callers
// that need to run a single verb's checker as a mutating handler's
// precondition (Fas 3 anti-drift) — the exact inputs guarantee a 422 can
// never disagree with what GET .../actions shows for the same verb.
// settlementID may be uuid.Nil where the check does not need one (e.g. the
// colonize settlement-cap requirement, which only reads worldID/playerID).
func NewContext(ctx context.Context, pool *pgxpool.Pool, clk clock.Clock, worldID, provinceID, playerID, settlementID uuid.UUID) checkContext {
	return checkContext{
		ctx:          ctx,
		pool:         pool,
		clk:          clk,
		worldID:      worldID,
		provinceID:   provinceID,
		playerID:     playerID,
		settlementID: settlementID,
	}
}

// hasSettlement reports whether this province resolved to an owned settlement.
func (cc checkContext) hasSettlement() bool {
	return cc.settlementID != uuid.Nil
}

// hasBuilding mirrors the `SELECT EXISTS(... FROM buildings ...)` check used
// throughout api/handlers (Craft, Recruit, Rite, Build).
// TODO: Fas 3 unify with handler gate.
func (cc checkContext) hasBuilding(buildingType string) bool {
	if !cc.hasSettlement() {
		return false
	}
	var exists bool
	_ = cc.pool.QueryRow(cc.ctx,
		`SELECT EXISTS(SELECT 1 FROM buildings WHERE settlement_id = $1 AND building_type = $2)`,
		cc.settlementID, buildingType,
	).Scan(&exists)
	return exists
}

// goodAmount returns the live (lazily-settled) stock of a good at this
// settlement, mirroring the settled(amount, rate, calc_tick) pattern used by
// every goods-deducting handler.
// TODO: Fas 3 unify with handler gate.
func (cc checkContext) goodAmount(goodKey string) float64 {
	if !cc.hasSettlement() {
		return 0
	}
	var amt float64
	err := cc.pool.QueryRow(cc.ctx,
		`SELECT GREATEST(0, settled(amount, rate, calc_tick))
		   FROM settlement_goods WHERE settlement_id = $1 AND good_key = $2`,
		cc.settlementID, goodKey,
	).Scan(&amt)
	if err != nil {
		return 0
	}
	return amt
}

// population returns the settlement's current population (0 if none/collapsed).
func (cc checkContext) population() int {
	if !cc.hasSettlement() {
		return 0
	}
	var pop int
	_ = cc.pool.QueryRow(cc.ctx,
		`SELECT population FROM settlements WHERE id = $1 AND state != 'collapsed'`,
		cc.settlementID,
	).Scan(&pop)
	return pop
}

// deployableLandUnits counts this settlement's garrisoned land units — the unit
// a colonize/march order can actually use. Status is the truth: 'garrison' means
// finished and deployable, so a battle-worn cohort (size < 100 after losses) still
// counts; only 'forming'/'training' units are excluded.
func (cc checkContext) deployableLandUnits() int {
	if !cc.hasSettlement() {
		return 0
	}
	var n int
	_ = cc.pool.QueryRow(cc.ctx,
		`SELECT count(*) FROM units
		 WHERE settlement_id = $1 AND owner_id = $2
		   AND status = 'garrison' AND category = 'land'`,
		cc.settlementID, cc.playerID,
	).Scan(&n)
	return n
}

// anyUnitsHere counts any of the player's units (garrison or forming — i.e.
// not yet marched off) at this settlement, land or naval.
func (cc checkContext) anyUnitsHere() int {
	if !cc.hasSettlement() {
		return 0
	}
	var n int
	_ = cc.pool.QueryRow(cc.ctx,
		`SELECT count(*) FROM units
		 WHERE settlement_id = $1 AND owner_id = $2 AND status IN ('garrison', 'forming', 'training')`,
		cc.settlementID, cc.playerID,
	).Scan(&n)
	return n
}

// idleNavalUnits counts garrisoned ships at this settlement with no cargo —
// the ones `unit load` can embark a land unit onto.
func (cc checkContext) idleNavalUnits() int {
	if !cc.hasSettlement() {
		return 0
	}
	var n int
	_ = cc.pool.QueryRow(cc.ctx,
		`SELECT count(*) FROM units
		 WHERE settlement_id = $1 AND owner_id = $2
		   AND category = 'naval' AND status = 'garrison' AND cargo_unit_id IS NULL`,
		cc.settlementID, cc.playerID,
	).Scan(&n)
	return n
}

// ladenNavalUnits counts garrisoned ships at this settlement carrying cargo —
// the ones `unit unload` can disembark.
func (cc checkContext) ladenNavalUnits() int {
	if !cc.hasSettlement() {
		return 0
	}
	var n int
	_ = cc.pool.QueryRow(cc.ctx,
		`SELECT count(*) FROM units
		 WHERE settlement_id = $1 AND owner_id = $2
		   AND category = 'naval' AND status = 'garrison' AND cargo_unit_id IS NOT NULL`,
		cc.settlementID, cc.playerID,
	).Scan(&n)
	return n
}

// marchingUnits counts the player's units currently in transit anywhere in
// the world — the pool `recall`/`redirect` can act on.
func (cc checkContext) marchingUnits() int {
	var n int
	_ = cc.pool.QueryRow(cc.ctx,
		`SELECT count(*) FROM units WHERE world_id = $1 AND owner_id = $2 AND status = 'marching'`,
		cc.worldID, cc.playerID,
	).Scan(&n)
	return n
}

// ownSettlements returns the count of the player's active settlements and
// whether at least one of them is a non-capital colony (abandon/transfer need
// a second settlement to target).
func (cc checkContext) ownSettlements() (total, nonCapital int) {
	_ = cc.pool.QueryRow(cc.ctx,
		`SELECT count(*), count(*) FILTER (WHERE NOT is_capital)
		   FROM settlements WHERE world_id = $1 AND owner_id = $2 AND state = 'active'`,
		cc.worldID, cc.playerID,
	).Scan(&total, &nonCapital)
	return total, nonCapital
}

// visibleOrigins mirrors api/handlers/world.go's loadVisibleOrigins — the
// KNOWN-set (live settlements ∪ in-flight marches ∪ messenger contacts ∪
// scouted tiles/provinces) used to FOW-gate messenger Send. Duplicated here
// (capabilities sits below api/handlers in G1) rather than imported.
// TODO: Fas 3 unify with handler gate.
func (cc checkContext) visibleOrigins() []province.MapPosition {
	rows, err := cc.pool.Query(cc.ctx,
		`SELECT DISTINCT pos.q, pos.r FROM (
		     SELECT p.map_q AS q, p.map_r AS r
		     FROM provinces p
		     JOIN settlements s ON s.province_id = p.id
		     WHERE p.world_id = $1 AND (
		         s.owner_id = $2
		         OR (s.kingdom_id IS NOT NULL AND s.kingdom_id IN (
		             SELECT km.kingdom_id FROM kingdom_members km WHERE km.player_id = $2
		         ))
		     )
		     UNION ALL
		     SELECT op.map_q, op.map_r
		     FROM marching_armies ma
		     JOIN provinces op ON op.id = ma.origin_id
		     JOIN settlements os ON os.province_id = ma.origin_id
		     WHERE ma.world_id = $1 AND ma.resolved = false AND os.owner_id = $2
		     UNION ALL
		     SELECT tp.map_q, tp.map_r
		     FROM marching_armies ma
		     JOIN provinces tp ON tp.id = ma.target_id
		     JOIN settlements os ON os.province_id = ma.origin_id
		     WHERE ma.world_id = $1 AND ma.resolved = false AND os.owner_id = $2
		       AND ma.intent != 'explore'
		     UNION ALL
		     SELECT dp.map_q, dp.map_r
		     FROM messengers m
		     JOIN settlements ds ON ds.id = m.destination_id
		     JOIN provinces dp ON dp.id = ds.province_id
		     WHERE m.world_id = $1 AND m.sender_id = $2
		       AND m.status IN ('delivered', 'returning', 'arrived')
		     UNION ALL
		     SELECT p.map_q, p.map_r
		     FROM player_scouted_provinces sp
		     JOIN provinces p ON p.id = sp.province_id
		     WHERE sp.world_id = $1 AND sp.player_id = $2
		     UNION ALL
		     SELECT q, r FROM player_scouted_tiles WHERE world_id = $1 AND player_id = $2
		 ) pos`,
		cc.worldID, cc.playerID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var origins []province.MapPosition
	for rows.Next() {
		var pos province.MapPosition
		if err := rows.Scan(&pos.Q, &pos.R); err == nil {
			origins = append(origins, pos)
		}
	}
	return origins
}

// visibleForeignSettlements counts settlements owned by other Wanaxes that
// fall within this player's visible-origins radius (6 hexes) — the FOW gate
// that Send (message/trade-offer/messenger) enforces server-side.
// TODO: Fas 3 unify with handler gate.
func (cc checkContext) visibleForeignSettlements() int {
	origins := cc.visibleOrigins()
	if len(origins) == 0 {
		return 0
	}
	rows, err := cc.pool.Query(cc.ctx,
		`SELECT prov.map_q, prov.map_r
		 FROM settlements s
		 JOIN provinces prov ON prov.id = s.province_id
		 WHERE s.world_id = $1 AND s.owner_id IS NOT NULL AND s.owner_id != $2 AND s.state != 'sunk'`,
		cc.worldID, cc.playerID,
	)
	if err != nil {
		return 0
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var q, r int
		if err := rows.Scan(&q, &r); err != nil {
			continue
		}
		if province.VisibleFrom(province.MapPosition{Q: q, R: r}, origins, 6) {
			count++
		}
	}
	return count
}

// pendingInboundTradeOffer reports whether this settlement has a delivered,
// still-pending trade offer waiting on trade-accept/trade-decline.
func (cc checkContext) pendingInboundTradeOffer() bool {
	if !cc.hasSettlement() {
		return false
	}
	var n int
	_ = cc.pool.QueryRow(cc.ctx,
		`SELECT count(*) FROM messengers
		 WHERE destination_id = $1 AND status = 'delivered'
		   AND trade_offer IS NOT NULL AND trade_offer->>'status' = 'pending'`,
		cc.settlementID,
	).Scan(&n)
	return n > 0
}

// pendingOutboundTradeOffer reports whether this settlement has an
// outstanding trade offer awaiting the recipient (trade-cancel target).
func (cc checkContext) pendingOutboundTradeOffer() bool {
	if !cc.hasSettlement() {
		return false
	}
	var n int
	_ = cc.pool.QueryRow(cc.ctx,
		`SELECT count(*) FROM messengers
		 WHERE origin_id = $1 AND status IN ('outbound', 'delivered')
		   AND trade_offer IS NOT NULL AND trade_offer->>'status' = 'pending'`,
		cc.settlementID,
	).Scan(&n)
	return n > 0
}

// repliableInbox counts delivered messengers at this settlement that are not
// a still-pending trade offer (those route to trade-accept/decline instead —
// see MessengerHandler.Reply's 409).
func (cc checkContext) repliableInbox() int {
	if !cc.hasSettlement() {
		return 0
	}
	var n int
	_ = cc.pool.QueryRow(cc.ctx,
		`SELECT count(*) FROM messengers
		 WHERE destination_id = $1 AND status = 'delivered'
		   AND (trade_offer IS NULL OR trade_offer->>'status' != 'pending')`,
		cc.settlementID,
	).Scan(&n)
	return n
}

// kharisAmount returns the player's live (settled) kharis standing in this world.
func (cc checkContext) kharisAmount() float64 {
	var k float64
	err := cc.pool.QueryRow(cc.ctx,
		`SELECT GREATEST(0, settled(kharis_amount, kharis_rate, kharis_calc_tick))
		   FROM player_world_records WHERE player_id = $1 AND world_id = $2`,
		cc.playerID, cc.worldID,
	).Scan(&k)
	if err != nil {
		return 0
	}
	return k
}

// cultureID returns this settlement's culture (for prayer-catalogue lookups).
func (cc checkContext) cultureID() string {
	if !cc.hasSettlement() {
		return ""
	}
	var culture string
	_ = cc.pool.QueryRow(cc.ctx,
		`SELECT culture_id FROM settlements WHERE id = $1`, cc.settlementID,
	).Scan(&culture)
	return culture
}

// capitalGoodAmount returns the live stock of a good at the player's
// capital, regardless of which province/settlement cc is currently scoped
// to — mirrors goodAmount but always resolves the capital first.
func (cc checkContext) capitalGoodAmount(goodKey string) float64 {
	settlementID, _, _, ok := cc.capitalSettlement()
	if !ok {
		return 0
	}
	var amt float64
	err := cc.pool.QueryRow(cc.ctx,
		`SELECT GREATEST(0, settled(amount, rate, calc_tick))
		   FROM settlement_goods WHERE settlement_id = $1 AND good_key = $2`,
		settlementID, goodKey,
	).Scan(&amt)
	if err != nil {
		return 0
	}
	return amt
}

// capitalSettlement returns the player's active capital settlement ID and
// province ID in this world, if any.
func (cc checkContext) capitalSettlement() (settlementID, provinceID uuid.UUID, kingdomID *uuid.UUID, ok bool) {
	err := cc.pool.QueryRow(cc.ctx,
		`SELECT id, province_id, kingdom_id FROM settlements
		 WHERE world_id = $1 AND owner_id = $2 AND is_capital = true AND state = 'active'`,
		cc.worldID, cc.playerID,
	).Scan(&settlementID, &provinceID, &kingdomID)
	if err != nil {
		if err != pgx.ErrNoRows {
			return uuid.Nil, uuid.Nil, nil, false
		}
		return uuid.Nil, uuid.Nil, nil, false
	}
	return settlementID, provinceID, kingdomID, true
}

// kingdomRole returns the player's role in a kingdom they belong to (any
// kingdom in this world — a Wanax may only ever be in one) and whether they
// are in one at all.
func (cc checkContext) kingdomRole() (kingdomID uuid.UUID, role string, ok bool) {
	err := cc.pool.QueryRow(cc.ctx,
		`SELECT km.kingdom_id, km.role
		   FROM kingdom_members km
		   JOIN kingdoms k ON k.id = km.kingdom_id
		  WHERE k.world_id = $1 AND km.player_id = $2`,
		cc.worldID, cc.playerID,
	).Scan(&kingdomID, &role)
	if err != nil {
		return uuid.Nil, "", false
	}
	return kingdomID, role, true
}
