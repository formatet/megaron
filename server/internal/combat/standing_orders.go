package combat

// Standing orders — caravans that run themselves between two of a Wanax's own
// settlements (megaron_plan_staende_leverans.md). PULL, not push: a route
// names a threshold to maintain at the destination ("keep grain at Colony ≥
// 200"), not a fixed quantity/interval, and the recurring sweep below reads
// the live shortfall every tick and reuses the EXISTING internal-transfer
// rail — transport.Dispatch, transport.Manifest (already multi-good),
// transport.ArrivalHandler (already credits a whole manifest, unmodified) —
// no new transport mechanic, per the plan's explicit instruction.
//
// Lives in `combat`, not `economy`, for one reason: G1 (CLAUDE.md) puts
// economy and transport at the SAME tier, so economy may not import transport
// (trade.go's own comment: "economy may not import the transport package").
// combat sits above both and already imports transport (occupation.go) — the
// same reason VoyageProvisions/UnitUpkeep, which this file reuses directly,
// live in this package's upkeep.go rather than in economy. The web/keryx
// SURFACE is still "economy" (an automation tab, per
// megaron_plan_stad_vs_ekonomi.md §1: a flow between two places belongs to
// neither) — that is a UI grouping question, orthogonal to where the Go
// package sits.
//
// Two traps the plan calls out by name, and how this file avoids them:
//
//  1. "The sweep must count a caravan already in flight against the
//     shortfall, or it dispatches a fresh full shipment every tick until the
//     first one lands." Solved by loadLatestLeg/legState.busy(): a route has
//     AT MOST one leg physically moving (or landed-and-awaiting-its-return)
//     at any time — a mutex, not a running-total subtraction. Simpler than
//     partial-credit accounting and trivially satisfies "already-dispatched
//     caravans count against the shortfall" (none can be dispatched while one
//     is out).
//  2. "The sender's own need must be read before anything is sent, or the
//     worst possible bug is an automation that quietly starves the capital."
//     Solved in dispatchOutboundIfNeeded: grain gets a round-trip reserve
//     (population's own consumption for the whole time the gubbe — and, if
//     the source crews the route, its provisions — will be away) subtracted
//     BEFORE any shortfall is clamped to "what's spendable"; every other good
//     is simply never sent past what is actually there (the same guarantee
//     any other transfer already has).
//
// Naval routing (plan §4 point 3, cheaper coastal upkeep — built
// megaron_plan_tva_slices_20260905.md §2): a route whose both ends are
// coastal-or-harboured AND connected by a navigable sea lane
// (province.ResolveTradeRoute — same substrate api/handlers/province.go's
// Trade handler now uses for the single-shot internal transfer) dispatches
// "naval" and prices its crew as the abstracted transport ship's own flat
// hull ration (UpkeepSpecs["merchantman"]) instead of a gubbe — see
// standingOrderRation's naval sibling, standingOrderNavalRation, and the
// category branch in dispatchOutboundIfNeeded. A naval leg draws no gubbe at
// all (idleGubbar/busyOrdersCrewedBy are skipped): the plan is explicit this
// slice requires no owned ship, so there is no population-scale crew to
// reserve, only the flat grain ration to pay.
import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/transport"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// standingOrderRation is what one gubbe (100 men) eats per tick while a land
// caravan is under way — plan §4: "en människa äter 0,5 korn per speldygn,
// och dubbelt i fält... En gubbe på vägen kostar alltså 1 korn per dygn."
// A caravan's gubbe is not a military unit type, so it cannot borrow a row
// from combat.UpkeepSpecs; this reuses the two named building blocks
// (GrainConsumptionPerCitizenPerTick, upkeepFieldGrainFactor — both already
// combined the same way by UnitUpkeep's own land branch, upkeep.go:108-118)
// directly instead of inventing a third number or hijacking an unrelated
// unit type that only coincidentally matches.
func standingOrderRation() float64 {
	return economy.GrainConsumptionPerCitizenPerTick * 100 * upkeepFieldGrainFactor
}

// standingOrderNavalRation is what the abstracted transport ship eats per
// tick on a naval leg — the merchantman's own flat hull ration from
// UpkeepSpecs (megaron_plan_tva_slices_20260905.md §2: "Sjövägen ska kosta
// fartygets egen besättning ... och skrovets platta ranson ur UpkeepSpecs.
// Använd befintliga tal, hitta inte på nya"). UnitUpkeep's naval branch
// never scales by size/crew (upkeep.go:105-107 — "naval: flat, status never
// changes it"), so this is just the table value, unadorned; the plan names
// merchantman specifically (crew 10, vs. galley's 20) as the everyday trade
// hull.
func standingOrderNavalRation() float64 {
	return UpkeepSpecs[string(unit.TypeMerchantman)].Grain
}

// settlementCoastalOrHarboured reports whether a settlement may send/receive
// naval traffic: adjacent to the sea (provinces.coastal) or it has built a
// harbour. Same "coastal OR harbour" gate api/handlers/unit.go's
// embark/disembark checks use (megaron_plan_tva_slices_20260905.md §2 step
// 1: "Använd samma definition — hitta inte på en ny").
func settlementCoastalOrHarboured(ctx context.Context, tx pgx.Tx, settlementID uuid.UUID) (bool, error) {
	var coastal bool
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(p.coastal, false) FROM settlements s JOIN provinces p ON p.id = s.province_id WHERE s.id = $1`,
		settlementID,
	).Scan(&coastal); err != nil {
		return false, err
	}
	if coastal {
		return true, nil
	}
	var hasHarbour bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM buildings WHERE settlement_id = $1 AND building_type = 'harbour')`,
		settlementID,
	).Scan(&hasHarbour); err != nil {
		return false, err
	}
	return hasHarbour, nil
}

// standingOrderRow is one route as read by the sweep.
type standingOrderRow struct {
	id      uuid.UUID
	worldID uuid.UUID
	ownerID uuid.UUID
	fromID  uuid.UUID
	toID    uuid.UUID
	crewID  uuid.UUID
	status  string
}

// legState is the most recent transports row dispatched for a standing order
// — the whole round-trip/mutex state machine lives in this one read.
type legState struct {
	exists bool
	kind   string
	status string
}

// busy reports whether the order currently has its gubbe out on the road:
// either leg physically moving, or the outbound leg landed and the return leg
// hasn't been sent yet. A delivered RETURN leg, no leg at all, or a
// lost/intercepted leg (either direction) means the gubbe is home (or the
// caravan and its cargo are simply gone) and the route is idle again.
func (ls legState) busy() bool {
	if !ls.exists {
		return false
	}
	if ls.status == "in_transit" {
		return true
	}
	return ls.kind == "standing_order_out" && ls.status == "delivered"
}

// StandingOrderTickHandler is the self-rescheduling per-world sweep. One
// instance per world, same shape as economy's SitosTickHandler/
// FoodTickHandler: fans one scheduled event out across every row, per-row
// transaction, per-row idempotency claim (processed_tick_claims, migration
// 098 — same shared table combat/upkeep.go already claims against).
type StandingOrderTickHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
	clk       clock.Clock
	hub       Broadcaster // nil-guarded (tests)
}

// NewStandingOrderTickHandler creates a StandingOrderTickHandler. hub may be nil.
func NewStandingOrderTickHandler(pool *pgxpool.Pool, sched *events.Scheduler, clk clock.Clock, hub Broadcaster) *StandingOrderTickHandler {
	return &StandingOrderTickHandler{pool: pool, scheduler: sched, clk: clk, hub: hub}
}

// Handle processes one ScheduledStandingOrderTick event for a world.
func (h *StandingOrderTickHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	rows, err := h.pool.Query(ctx,
		`SELECT id, world_id, owner_id, from_settlement_id, to_settlement_id, crewed_by_settlement_id, status
		 FROM standing_orders WHERE world_id = $1`,
		e.WorldID,
	)
	if err != nil {
		return fmt.Errorf("standing order tick: query orders: %w", err)
	}
	var orders []standingOrderRow
	for rows.Next() {
		var o standingOrderRow
		if err := rows.Scan(&o.id, &o.worldID, &o.ownerID, &o.fromID, &o.toID, &o.crewID, &o.status); err == nil {
			orders = append(orders, o)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, o := range orders {
		if err := h.tickOrder(ctx, o, e.ID); err != nil {
			slog.Error("standing order tick: order failed", "order", o.id, "err", err)
		}
	}

	return h.scheduler.EnqueueTickRecurring(ctx, e.WorldID, events.ScheduledStandingOrderTick,
		struct{}{}, e.DueTick, events.MacroTickInterval)
}

// tickOrder evaluates one route in its own transaction.
func (h *StandingOrderTickHandler) tickOrder(ctx context.Context, o standingOrderRow, eventID int64) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	claim, err := tx.Exec(ctx,
		`INSERT INTO processed_tick_claims (event_id, scope_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		eventID, o.id)
	if err != nil {
		return fmt.Errorf("claim standing order tick: %w", err)
	}
	if claim.RowsAffected() == 0 {
		return nil // already processed this event for this order
	}

	leg, err := loadLatestLeg(ctx, tx, o.id)
	if err != nil {
		return fmt.Errorf("load latest leg: %w", err)
	}

	// TRAP 1: a leg already in flight blocks a fresh dispatch outright — this
	// is what stops a full shipment going out every tick until the first one
	// lands (megaron_plan_staende_leverans.md §2).
	if leg.exists && leg.kind == "standing_order_out" && leg.status == "delivered" {
		return h.dispatchReturn(ctx, tx, o)
	}
	if leg.busy() {
		return tx.Commit(ctx) // still moving — nothing to do this tick
	}

	if o.status != "active" {
		return tx.Commit(ctx) // paused, idle, nothing in flight — leave it alone
	}

	return h.dispatchOutboundIfNeeded(ctx, tx, o)
}

// dispatchOutboundIfNeeded reads the destination's shortfall and, if the
// source can spare it without dipping below its own reserve (TRAP 2), sends
// the outbound leg.
func (h *StandingOrderTickHandler) dispatchOutboundIfNeeded(ctx context.Context, tx pgx.Tx, o standingOrderRow) error {
	needRows, err := tx.Query(ctx,
		`SELECT good_key, threshold FROM standing_order_outbound_goods WHERE standing_order_id = $1`, o.id)
	if err != nil {
		return fmt.Errorf("load outbound goods: %w", err)
	}
	type need struct {
		good      string
		threshold float64
	}
	var needs []need
	for needRows.Next() {
		var n need
		if scanErr := needRows.Scan(&n.good, &n.threshold); scanErr == nil {
			needs = append(needs, n)
		}
	}
	needRows.Close()
	if err := needRows.Err(); err != nil {
		return err
	}

	shortfall := make(map[string]float64)
	anyNeeded := false
	for _, n := range needs {
		stock, err := settledStock(ctx, tx, o.toID, n.good)
		if err != nil {
			return err
		}
		if stock < n.threshold {
			shortfall[n.good] = n.threshold - stock
			anyNeeded = true
		}
	}
	if !anyNeeded {
		return tx.Commit(ctx) // destination is fully stocked — nothing to do
	}

	fromQ, fromR, err := settlementHex(ctx, tx, o.fromID)
	if err != nil {
		return fmt.Errorf("load source hex: %w", err)
	}
	toQ, toR, err := settlementHex(ctx, tx, o.toID)
	if err != nil {
		return fmt.Errorf("load destination hex: %w", err)
	}

	// Coastal advantage (megaron_plan_tva_slices_20260905.md §2): naval only
	// when both ends are coastal-or-harboured AND a navigable sea lane
	// actually connects them (province.ResolveTradeRoute — the same
	// substrate api/handlers/province.go's Trade handler uses for the
	// single-shot internal transfer). Falls back to "land" with the
	// unchanged straight-line hex distance otherwise.
	fromCoastal, err := settlementCoastalOrHarboured(ctx, tx, o.fromID)
	if err != nil {
		return fmt.Errorf("check source coastal: %w", err)
	}
	toCoastal, err := settlementCoastalOrHarboured(ctx, tx, o.toID)
	if err != nil {
		return fmt.Errorf("check destination coastal: %w", err)
	}
	category, dist, err := province.ResolveTradeRoute(ctx, tx, o.worldID, fromCoastal, toCoastal,
		province.MapPosition{Q: fromQ, R: fromR}, province.MapPosition{Q: toQ, R: toR})
	if err != nil {
		return fmt.Errorf("resolve trade route: %w", err)
	}
	travelMins := 30.0 + float64(dist)*2.0 // same estimate api/handlers/province.go's Trade handler uses
	travelTicks := int(math.Round(travelMins / 60))
	if travelTicks < 1 {
		travelTicks = 1
	}

	// TRAP 2: the source's own need, read BEFORE anything is clamped to "what's
	// spendable". Grain gets a reserve covering the source's own population for
	// the whole round trip (2×travelTicks — the gubbe, and the goods, are gone
	// that long); every other good only ever ships what's actually there.
	fromPop, err := settlementPopulation(ctx, tx, o.fromID)
	if err != nil {
		return fmt.Errorf("load source population: %w", err)
	}
	// Naval pays the abstracted ship's own flat hull ration instead of a
	// gubbe's (plan §2: "Sjövägen ska kosta fartygets egen besättning ...
	// och skrovets platta ranson ur UpkeepSpecs").
	ration := standingOrderRation()
	if category == "naval" {
		ration = standingOrderNavalRation()
	}
	provisions := VoyageProvisions(ration, travelTicks, 0)
	grainReserve := economy.GrainConsumptionPerCitizenPerTick * float64(fromPop) * float64(2*travelTicks)
	if o.crewID == o.fromID {
		// Provisions are drawn from the SAME settlement's SAME grain stock as
		// the manifest below — fold them into one reserve so the two draws can
		// never overdraw it between them (see the deduction order at the
		// bottom of this function).
		grainReserve += provisions
	}

	manifest := transport.Manifest{}
	for good, want := range shortfall {
		available, err := settledStock(ctx, tx, o.fromID, good)
		if err != nil {
			return err
		}
		reserve := 0.0
		if good == economy.GoodGrain {
			reserve = grainReserve
		}
		spendable := available - reserve
		if spendable <= 0 {
			continue
		}
		if send := math.Min(want, spendable); send > 0 {
			manifest[good] = send
		}
	}

	if len(manifest) == 0 {
		return h.pauseOrder(ctx, tx, o,
			"the source has nothing to spare for this route without dipping into its own reserve")
	}

	// Crew check: does the crewing settlement have a gubbe free? Idle = gubbar
	// by population, minus placed gubbar (settlement_placement), minus gubbar
	// already away on this settlement's OTHER standing orders. No schema
	// change to settlement_placement (P4's target_kind CHECK stays hex|
	// building only) — this order's own gubbe is tracked purely via its
	// transport leg state, not a placement row.
	//
	// Naval skips this entirely: the plan is explicit this slice requires no
	// owned ship (an abstracted hull, like the land route needs no owned
	// donkeys), so there is no gubbe-scale (100-citizen) workforce to reserve
	// — only the flat grain ration above, paid regardless of category below.
	if category == "land" {
		idle, err := idleGubbar(ctx, tx, o.crewID)
		if err != nil {
			return fmt.Errorf("load idle gubbar: %w", err)
		}
		busyElsewhere, err := busyOrdersCrewedBy(ctx, tx, o.crewID, o.id)
		if err != nil {
			return fmt.Errorf("count busy orders: %w", err)
		}
		if idle-busyElsewhere < 1 {
			return h.pauseOrder(ctx, tx, o, "no spare workforce at the crewing settlement")
		}
	}

	if o.crewID != o.fromID {
		crewGrain, err := settledStock(ctx, tx, o.crewID, economy.GoodGrain)
		if err != nil {
			return err
		}
		if crewGrain < provisions {
			return h.pauseOrder(ctx, tx, o, "not enough grain at the crewing settlement to provision the caravan")
		}
	}

	// All checks passed — commit the shipment.
	for good, qty := range manifest {
		if err := deductGood(ctx, tx, o.fromID, good, qty); err != nil {
			return fmt.Errorf("deduct %s from source: %w", good, err)
		}
	}
	if err := deductGood(ctx, tx, o.crewID, economy.GoodGrain, provisions); err != nil {
		return fmt.Errorf("deduct provisions: %w", err)
	}

	var currentTick int
	_ = tx.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
	departsAt := h.clk.Now()
	arrivesAt := departsAt.Add(time.Duration(travelMins * float64(time.Minute)))

	orderID := o.id
	if _, err := transport.Dispatch(ctx, tx, h.scheduler, transport.DispatchParams{
		WorldID: o.worldID, OwnerID: o.ownerID, Kind: "standing_order_out",
		OriginID: o.fromID, DestID: o.toID, Category: category,
		OriginQ: fromQ, OriginR: fromR, DestQ: toQ, DestR: toR,
		DepartsAt: departsAt, ArrivesAt: arrivesAt, DueTick: currentTick + travelTicks,
		Manifest: manifest, Interceptable: true, StandingOrderID: &orderID,
	}); err != nil {
		return fmt.Errorf("dispatch outbound leg: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE standing_orders SET last_dispatched_tick = $2 WHERE id = $1`,
		o.id, currentTick,
	); err != nil {
		return fmt.Errorf("update last dispatched: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	h.notifyDispatch(ctx, o, "outbound", manifest, arrivesAt)
	return nil
}

// dispatchReturn runs once the outbound leg has landed: it loads home
// whatever surplus the destination can spare above each return good's floor
// (possibly nothing — an empty caravan going home is quiet and normal,
// plan §4c) and sends the SAME gubbe back the way it came.
func (h *StandingOrderTickHandler) dispatchReturn(ctx context.Context, tx pgx.Tx, o standingOrderRow) error {
	floorRows, err := tx.Query(ctx,
		`SELECT good_key, floor FROM standing_order_return_goods WHERE standing_order_id = $1`, o.id)
	if err != nil {
		return fmt.Errorf("load return goods: %w", err)
	}
	type floor struct {
		good string
		min  float64
	}
	var floors []floor
	for floorRows.Next() {
		var f floor
		if scanErr := floorRows.Scan(&f.good, &f.min); scanErr == nil {
			floors = append(floors, f)
		}
	}
	floorRows.Close()
	if err := floorRows.Err(); err != nil {
		return err
	}

	returnManifest := transport.Manifest{}
	for _, f := range floors {
		stock, err := settledStock(ctx, tx, o.toID, f.good)
		if err != nil {
			return err
		}
		if send := stock - f.min; send > 0 {
			returnManifest[f.good] = send
		}
	}
	for good, qty := range returnManifest {
		if err := deductGood(ctx, tx, o.toID, good, qty); err != nil {
			return fmt.Errorf("deduct %s for return leg: %w", good, err)
		}
	}

	toQ, toR, err := settlementHex(ctx, tx, o.toID)
	if err != nil {
		return fmt.Errorf("load destination hex: %w", err)
	}
	fromQ, fromR, err := settlementHex(ctx, tx, o.fromID)
	if err != nil {
		return fmt.Errorf("load source hex: %w", err)
	}

	// Same lane home as the outbound leg sailed/marched — resolve it the same
	// way (megaron_plan_tva_slices_20260905.md §2) rather than assuming land:
	// a route that went naval out returns naval, priced off the real sea
	// path's own length, not the straight-line hex count.
	toCoastal, err := settlementCoastalOrHarboured(ctx, tx, o.toID)
	if err != nil {
		return fmt.Errorf("check destination coastal: %w", err)
	}
	fromCoastal, err := settlementCoastalOrHarboured(ctx, tx, o.fromID)
	if err != nil {
		return fmt.Errorf("check source coastal: %w", err)
	}
	category, dist, err := province.ResolveTradeRoute(ctx, tx, o.worldID, toCoastal, fromCoastal,
		province.MapPosition{Q: toQ, R: toR}, province.MapPosition{Q: fromQ, R: fromR})
	if err != nil {
		return fmt.Errorf("resolve return trade route: %w", err)
	}
	travelMins := 30.0 + float64(dist)*2.0
	travelTicks := int(math.Round(travelMins / 60))
	if travelTicks < 1 {
		travelTicks = 1
	}

	var currentTick int
	_ = tx.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
	departsAt := h.clk.Now()
	arrivesAt := departsAt.Add(time.Duration(travelMins * float64(time.Minute)))

	orderID := o.id
	if _, err := transport.Dispatch(ctx, tx, h.scheduler, transport.DispatchParams{
		WorldID: o.worldID, OwnerID: o.ownerID, Kind: "standing_order_return",
		OriginID: o.toID, DestID: o.fromID, Category: category,
		OriginQ: toQ, OriginR: toR, DestQ: fromQ, DestR: fromR,
		DepartsAt: departsAt, ArrivesAt: arrivesAt, DueTick: currentTick + travelTicks,
		Manifest: returnManifest, Interceptable: true, StandingOrderID: &orderID,
	}); err != nil {
		return fmt.Errorf("dispatch return leg: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	h.notifyDispatch(ctx, o, "return", returnManifest, arrivesAt)
	return nil
}

func (h *StandingOrderTickHandler) pauseOrder(ctx context.Context, tx pgx.Tx, o standingOrderRow, reason string) error {
	if _, err := tx.Exec(ctx,
		`UPDATE standing_orders SET status = 'paused', pause_reason = $2 WHERE id = $1`,
		o.id, reason,
	); err != nil {
		return fmt.Errorf("pause order: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit pause: %w", err)
	}
	slog.Info("standing order paused", "order", o.id, "reason", reason)
	if h.hub != nil {
		_ = h.hub.NotifyPlayer(ctx, o.worldID, o.ownerID, "StandingOrderPaused", 2, map[string]any{
			"standing_order_id": o.id,
			"reason":            reason,
		})
	}
	return nil
}

func (h *StandingOrderTickHandler) notifyDispatch(ctx context.Context, o standingOrderRow, leg string, manifest transport.Manifest, arrivesAt time.Time) {
	if h.hub == nil {
		return
	}
	goods := make([]map[string]any, 0, len(manifest))
	for good, qty := range manifest {
		goods = append(goods, map[string]any{"good_key": good, "quantity": qty})
	}
	_ = h.hub.NotifyPlayer(ctx, o.worldID, o.ownerID, "StandingOrderDispatched", 3, map[string]any{
		"standing_order_id": o.id,
		"leg":               leg,
		"goods":             goods,
		"arrives_at":        arrivesAt,
	})
}

// ---- shared small helpers --------------------------------------------------

func loadLatestLeg(ctx context.Context, tx pgx.Tx, orderID uuid.UUID) (legState, error) {
	var ls legState
	err := tx.QueryRow(ctx,
		`SELECT kind, status FROM transports
		 WHERE standing_order_id = $1
		 ORDER BY created_at DESC LIMIT 1`,
		orderID,
	).Scan(&ls.kind, &ls.status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return legState{}, nil
		}
		return legState{}, err
	}
	ls.exists = true
	return ls, nil
}

func settledStock(ctx context.Context, tx pgx.Tx, settlementID uuid.UUID, good string) (float64, error) {
	var v float64
	err := tx.QueryRow(ctx,
		`SELECT GREATEST(0, settled(amount, rate, calc_tick))
		 FROM settlement_goods WHERE settlement_id = $1 AND good_key = $2`,
		settlementID, good,
	).Scan(&v)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return v, nil
}

func deductGood(ctx context.Context, tx pgx.Tx, settlementID uuid.UUID, good string, qty float64) error {
	if qty <= 0 {
		return nil
	}
	tag, err := tx.Exec(ctx,
		`UPDATE settlement_goods SET
		     amount    = settled(amount, rate, calc_tick) - $1,
		     calc_tick = current_world_tick()
		 WHERE settlement_id = $2 AND good_key = $3
		   AND settled(amount, rate, calc_tick) >= $1`,
		qty, settlementID, good,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("insufficient %s at settlement %s", good, settlementID)
	}
	return nil
}

func settlementHex(ctx context.Context, tx pgx.Tx, settlementID uuid.UUID) (q, r int, err error) {
	err = tx.QueryRow(ctx,
		`SELECT p.map_q, p.map_r FROM settlements s JOIN provinces p ON p.id = s.province_id WHERE s.id = $1`,
		settlementID,
	).Scan(&q, &r)
	return q, r, err
}

func settlementPopulation(ctx context.Context, tx pgx.Tx, settlementID uuid.UUID) (int, error) {
	var pop int
	err := tx.QueryRow(ctx, `SELECT population FROM settlements WHERE id = $1`, settlementID).Scan(&pop)
	return pop, err
}

// idleGubbar is how many unplaced gubbar (Placements' own "pool_size", P4)
// the settlement has — population/100 minus every settlement_placement row,
// regardless of what good they're working.
func idleGubbar(ctx context.Context, tx pgx.Tx, settlementID uuid.UUID) (int, error) {
	pop, err := settlementPopulation(ctx, tx, settlementID)
	if err != nil {
		return 0, err
	}
	var placed int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM settlement_placement WHERE settlement_id = $1`, settlementID,
	).Scan(&placed); err != nil {
		return 0, err
	}
	return pop/100 - placed, nil
}

// busyOrdersCrewedBy counts how many of the settlement's OTHER standing
// orders currently have a gubbe away — so two routes crewed by the same
// small settlement can never both claim the same last idle gubbe.
func busyOrdersCrewedBy(ctx context.Context, tx pgx.Tx, crewSettlementID, excludeOrderID uuid.UUID) (int, error) {
	rows, err := tx.Query(ctx,
		`SELECT id FROM standing_orders WHERE crewed_by_settlement_id = $1 AND id <> $2`,
		crewSettlementID, excludeOrderID,
	)
	if err != nil {
		return 0, err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	count := 0
	for _, id := range ids {
		leg, err := loadLatestLeg(ctx, tx, id)
		if err != nil {
			return 0, err
		}
		if leg.busy() {
			count++
		}
	}
	return count, nil
}
