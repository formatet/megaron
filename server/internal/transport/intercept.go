package transport

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Dice is R5's injectable probability seam (megaron_plan_sjohandel_kraver_
// skepp.md — "en injicerbar seam i InterceptScanHandler. Följ mönstret med
// Dice i UnitArrivalHandler / economy.NewWallDice"). transport may not import
// economy (G1: they sit at the same tier), so this is its own tiny copy of
// the same one-method contract rather than a shared type — any economy.Dice
// value (e.g. economy.NewWallDice()) already satisfies it structurally, so
// production wiring in cmd/server just passes that straight in.
type Dice interface {
	Float64() float64 // [0,1) — math/rand.Float64's contract
}

// wallDice is the production Dice default (InterceptScanHandler.Dice is
// nil-checked and falls back to this) — delegates to the global math/rand
// source, matching economy.wallDice's own behaviour.
type wallDice struct{}

func (wallDice) Float64() float64 { return rand.Float64() }

// NavalSeizureOutcome is R5's three-way naval interception result, rolled
// ONCE in seize() (events store outcomes, never intentions — CLAUDE.md).
type NavalSeizureOutcome string

const (
	// NavalSeizureCaptured: the interceptor takes the cargo AND the ship —
	// strawman 40%.
	NavalSeizureCaptured NavalSeizureOutcome = "captured"
	// NavalSeizureLimped: half the cargo is destroyed outright, the ship
	// limps home with the rest — strawman 40%.
	NavalSeizureLimped NavalSeizureOutcome = "limped"
	// NavalSeizureSunk: the whole cargo and the ship are lost — strawman 20%.
	NavalSeizureSunk NavalSeizureOutcome = "sunk"
)

// rollNavalSeizureOutcome applies R5's strawman odds (captured 40% / limped
// 40% / sunk 20% — "okalibrerat, justeras med prissättningen"). Tests inject
// a Dice that returns a fixed value to force each branch deterministically.
func rollNavalSeizureOutcome(d Dice) NavalSeizureOutcome {
	roll := d.Float64()
	switch {
	case roll < 0.4:
		return NavalSeizureCaptured
	case roll < 0.8:
		return NavalSeizureLimped
	default:
		return NavalSeizureSunk
	}
}

// Interception tuning (calibration — tune freely). radius: an enemy sentry watching
// a hex within this many hexes of a caravan's current position seizes it. interval:
// ticks between scans.
const (
	interceptRadius            = 2
	interceptScanIntervalTicks = 1
)

// Notifier pushes a notification to a player. *notify.Hub satisfies it.
type Notifier interface {
	NotifyPlayer(ctx context.Context, worldID, playerID uuid.UUID, kind string, level int, payload any) error
}

// InterceptScanHandler is the recurring sweep that seizes trade/transfer caravans
// passing within reach of an enemy sentry (movement-motor Slice C / War & Diplomacy
// Fas X). It reads ONLY the transports table — messengers are sacred and never
// scanned, so they can never be intercepted (only the gods may touch them).
type InterceptScanHandler struct {
	pool       *pgxpool.Pool
	scheduler  *events.Scheduler
	eventStore *events.Store
	notifier   Notifier
	clk        clock.Clock
	// Dice is R5's naval-seizure-outcome seam. nil-guarded (falls back to
	// wallDice{}, production behaviour) — tests inject a fixed-value Dice to
	// force captured/limped/sunk deterministically.
	Dice Dice
}

// NewInterceptScanHandler creates an InterceptScanHandler.
func NewInterceptScanHandler(pool *pgxpool.Pool, sched *events.Scheduler, store *events.Store, notifier Notifier, clk clock.Clock) *InterceptScanHandler {
	return &InterceptScanHandler{pool: pool, scheduler: sched, eventStore: store, notifier: notifier, clk: clk, Dice: wallDice{}}
}

func (h *InterceptScanHandler) dice() Dice {
	if h.Dice != nil {
		return h.Dice
	}
	return wallDice{}
}

type inFlightTransport struct {
	id         uuid.UUID
	owner      uuid.UUID
	originID   *uuid.UUID
	originQ    int
	originR    int
	destQ      int
	destR      int
	category   string
	departs    time.Time
	arrives    time.Time
	shipUnitID *uuid.UUID
}

// Handle scans every in-transit interceptable caravan once, seizing any caught by
// an enemy sentry, then re-enqueues itself.
func (h *InterceptScanHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	now := h.clk.Now()

	// Terrain for the FOW gate below (§4): AnyEyeSees needs the target hex's
	// terrain to size the interceptor's vision per eye-kind. Loaded once for the
	// whole sweep. Sentry owner → their live eyes is memoised the same way — many
	// caravans may be caught by the same Wanax, and eyes are fixed for this `now`.
	graph, err := province.LoadTileGraph(ctx, h.pool, e.WorldID)
	if err != nil {
		return fmt.Errorf("intercept scan: load terrain: %w", err)
	}
	eyesByOwner := map[uuid.UUID][]province.Eye{}

	rows, err := h.pool.Query(ctx,
		`SELECT id, owner_id, origin_id, origin_q, origin_r, dest_q, dest_r, category, departs_at, arrives_at, ship_unit_id
		 FROM transports
		 WHERE world_id = $1 AND status = 'in_transit' AND interceptable = true`,
		e.WorldID,
	)
	if err != nil {
		return fmt.Errorf("intercept scan: load transports: %w", err)
	}
	var fleet []inFlightTransport
	for rows.Next() {
		var t inFlightTransport
		if scanErr := rows.Scan(&t.id, &t.owner, &t.originID, &t.originQ, &t.originR, &t.destQ, &t.destR,
			&t.category, &t.departs, &t.arrives, &t.shipUnitID); scanErr != nil {
			rows.Close()
			return fmt.Errorf("intercept scan: scan transport: %w", scanErr)
		}
		fleet = append(fleet, t)
	}
	rows.Close()

	for _, t := range fleet {
		origin := province.MapPosition{Q: t.originQ, R: t.originR}
		dest := province.MapPosition{Q: t.destQ, R: t.destR}
		pos, ok, posErr := province.InterpolatePosition(ctx, h.pool, e.WorldID,
			origin, dest, t.category, t.departs, t.arrives, now)
		if posErr != nil {
			// A DB error is not the same as an unpathable route — never guess a
			// position off the back of one.
			continue
		}
		if !ok {
			// No traversable route exists for this category (e.g. a land caravan
			// whose origin/dest are split by sea or, post-flod, a river — the
			// A* category graph has no path). The caravan's own travel time is
			// already an abstracted straight hex line, never A* (TradeTicksPerHex,
			// messenger/recall.go) — so falling back to that same straight line for
			// its live position keeps it a real, interceptable object instead of
			// silently making it permanently uninterceptable. Only messengers may
			// be uninterceptable (Timothy 2026-07-30).
			pos = straightLineHexPosition(origin, dest, t.departs, t.arrives, now)
			slog.Warn("intercept scan: no traversable route for category, using straight-line fallback position",
				"transport", t.id, "category", t.category,
				"origin_q", t.originQ, "origin_r", t.originR,
				"dest_q", t.destQ, "dest_r", t.destR,
				"fallback_q", pos.Q, "fallback_r", pos.R)
		}

		// An enemy sentry watching within reach of the caravan's current hex,
		// whose reaction policy actually says "intercept" for foreign units
		// (avsiktslagret §S1/S2 — was implicit/hardcoded before mig 112).
		var sentryID, interceptor uuid.UUID
		if qErr := h.pool.QueryRow(ctx,
			`SELECT id, owner_id FROM units
			 WHERE world_id = $1 AND owner_id <> $2 AND status = 'positioned' AND stance = 'sentry'
			   AND sentry_q IS NOT NULL AND sentry_r IS NOT NULL
			   AND (reaction_policy->>'foreign') = 'intercept'
			   AND (ABS(sentry_q - $3) + ABS(sentry_r - $4) + ABS((sentry_q + sentry_r) - ($3 + $4))) / 2 <= $5
			 ORDER BY size DESC
			 LIMIT 1`,
			e.WorldID, t.owner, pos.Q, pos.R, interceptRadius,
		).Scan(&sentryID, &interceptor); qErr != nil {
			continue // no intercept-policy sentry in reach
		}

		// FOW gate (avsiktslagret §4): a sentry may only seize what its OWNER can
		// actually SEE right now — reuse the SAME gate /foreign-units uses
		// (LoadLiveEyes + AnyEyeSees) against the caravan's live interpolated
		// position, so a sentry is never an all-seeing tripwire ("allvetande
		// snubbeltråd"). A naval sentry within interceptRadius but beyond its own
		// short land horizon no longer grabs a caravan blind. Fail closed: no
		// terrain row for the hex → treat as unseen, never guess.
		terrain, known := graph[[2]int{pos.Q, pos.R}]
		if !known {
			continue
		}
		eyes, cached := eyesByOwner[interceptor]
		if !cached {
			eyes = province.LoadLiveEyes(ctx, h.pool, e.WorldID, interceptor, now)
			eyesByOwner[interceptor] = eyes
		}
		if !province.AnyEyeSees(eyes, pos, terrain) {
			continue // the sentry's owner has never laid eyes on this caravan
		}

		if err := h.seize(ctx, e.WorldID, t, sentryID, interceptor, pos); err != nil {
			slog.Error("intercept scan: seize failed", "transport", t.id, "err", err)
		}
	}

	// Re-enqueue the next sweep.
	return h.scheduler.EnqueueTickRecurring(ctx, e.WorldID, events.ScheduledInterceptScan,
		struct{}{}, e.DueTick, interceptScanIntervalTicks)
}

// seize marks the caravan intercepted and moves its manifest to the interceptor's
// capital. Guarded so a caravan is seized at most once even under concurrent scans.
func (h *InterceptScanHandler) seize(ctx context.Context, worldID uuid.UUID, t inFlightTransport, sentryID, interceptor uuid.UUID, pos province.MapPosition) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx,
		`UPDATE transports SET status = 'intercepted', updated_at = now()
		 WHERE id = $1 AND status = 'in_transit'`, t.id)
	if err != nil {
		return fmt.Errorf("flip intercepted: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil // already delivered/intercepted by a racing sweep
	}

	// The manifest, read inside the seizure TX so the notices name exactly the
	// cargo that changed hands — "your caravan was raided" without saying of
	// what left both Wanaxes guessing (megaron_plan_dispatches.md family rest).
	goods, err := loadManifest(ctx, tx, t.id)
	if err != nil {
		return err
	}

	// R5 (megaron_plan_sjohandel_kraver_skepp.md): a naval transport with a
	// bound ship rolls ONE of three outcomes (events store outcomes, never
	// intentions). A land caravan, or a pre-slice naval transport with no
	// bound ship (R6), keeps today's unconditional "loot goes to capital"
	// behaviour untouched.
	var shipOutcome *NavalSeizureOutcome
	if t.shipUnitID != nil {
		outcome := rollNavalSeizureOutcome(h.dice())
		shipOutcome = &outcome
		switch outcome {
		case NavalSeizureCaptured:
			// Cargo behaves exactly like a land caravan's loot (below); the
			// ship ALSO changes hands, which needs combat's march machinery
			// (transport may not import combat, G1) — cross the boundary via
			// event emission, in the SAME tx so the flip and the event are
			// atomic.
			if err := creditLootToCapital(ctx, tx, worldID, interceptor, t.id); err != nil {
				return err
			}
			if h.scheduler != nil {
				var currentTick int
				_ = tx.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
				if err := h.scheduler.EnqueueTickTx(ctx, tx, worldID, events.ScheduledNavalSeizureOutcome,
					map[string]any{
						"transport_id": t.id, "ship_unit_id": *t.shipUnitID,
						"outcome": string(outcome), "captor_id": interceptor, "q": pos.Q, "r": pos.R,
					}, currentTick); err != nil {
					return fmt.Errorf("enqueue naval seizure outcome: %w", err)
				}
			}
		case NavalSeizureLimped:
			// Half the cargo is destroyed outright (never credited to the
			// interceptor — it simply doesn't exist any more); the rest
			// sails home with the same ship, released on arrival exactly
			// like R3's ship_return leg (kind="damaged_return" is in
			// ArrivalHandler's release set).
			if err := h.dispatchLimpedReturn(ctx, tx, worldID, t, pos, goods); err != nil {
				return fmt.Errorf("dispatch limped return: %w", err)
			}
		case NavalSeizureSunk:
			// Cargo and ship both lost outright — no loot for anyone.
			if _, err := tx.Exec(ctx,
				`UPDATE units SET status = 'disbanded', size = 0, crew = 0, updated_at = now() WHERE id = $1`,
				*t.shipUnitID,
			); err != nil {
				return fmt.Errorf("sink seized ship: %w", err)
			}
		}
	} else {
		if err := creditLootToCapital(ctx, tx, worldID, interceptor, t.id); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// Audit + notify both Wanax: the raider (seized) and the victim (raided). Async
	// play → the victim is likely offline when the raid lands, so the notice matters.
	auditPayload := map[string]any{"transport_id": t.id, "sentry_unit_id": sentryID, "interceptor": interceptor, "q": pos.Q, "r": pos.R}
	seizedPayload := map[string]any{"transport_id": t.id, "q": pos.Q, "r": pos.R, "goods": goods}
	raidedPayload := map[string]any{"transport_id": t.id, "q": pos.Q, "r": pos.R, "goods": goods}
	if shipOutcome != nil {
		auditPayload["ship_outcome"] = string(*shipOutcome)
		auditPayload["ship_unit_id"] = *t.shipUnitID
		seizedPayload["ship_outcome"] = string(*shipOutcome)
		seizedPayload["ship_unit_id"] = *t.shipUnitID
		raidedPayload["ship_outcome"] = string(*shipOutcome)
		raidedPayload["ship_unit_id"] = *t.shipUnitID
	}
	if h.eventStore != nil {
		_, _ = h.eventStore.Append(ctx, t.id, events.StreamProvince, "CaravanIntercepted", auditPayload, worldID, nil)
	}
	if h.notifier != nil {
		_ = h.notifier.NotifyPlayer(ctx, worldID, interceptor, "CaravanSeized", 3, seizedPayload)
		_ = h.notifier.NotifyPlayer(ctx, worldID, t.owner, "CaravanRaided", 3, raidedPayload)
	}
	shipOutcomeLog := "n/a"
	if shipOutcome != nil {
		shipOutcomeLog = string(*shipOutcome)
	}
	slog.Info("caravan intercepted", "transport", t.id, "by", interceptor, "sentry", sentryID, "q", pos.Q, "r", pos.R,
		"ship_outcome", shipOutcomeLog)
	return nil
}

// creditLootToCapital is the pre-R5 seizure behaviour, unchanged: the
// interceptor hauls the cargo home to their capital, or it's lost to the raid
// if they have none (rare).
func creditLootToCapital(ctx context.Context, tx pgx.Tx, worldID, interceptor, transportID uuid.UUID) error {
	var capital *uuid.UUID
	_ = tx.QueryRow(ctx,
		`SELECT id FROM settlements WHERE owner_id = $1 AND world_id = $2 AND is_capital = true LIMIT 1`,
		interceptor, worldID,
	).Scan(&capital)
	if capital == nil {
		return nil
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 SELECT $1, tg.good_key, tg.quantity, 0, 1000000, current_world_tick()
		 FROM transport_goods tg WHERE tg.transport_id = $2
		 ON CONFLICT (settlement_id, good_key) DO UPDATE SET
		     amount = LEAST(
		         settled(settlement_goods.amount, settlement_goods.rate, settlement_goods.calc_tick)
		             + EXCLUDED.amount,
		         settlement_goods.cap),
		     calc_tick = current_world_tick()`,
		*capital, transportID,
	); err != nil {
		return fmt.Errorf("credit loot: %w", err)
	}
	return nil
}

// dispatchLimpedReturn is R5's "limped" outcome: half the manifest is
// destroyed on the spot, the other half sails home with the same ship on a
// fresh "damaged_return" transport from the capture hex — ArrivalHandler
// credits that half and releases the ship when it lands (same mechanism as
// R3's ship_return leg). No avsändarstad left on record (origin_id vanished)
// → falls back to the owner's nearest own port, same as ArrivalHandler's own
// vanished-destination fallback; if the owner has NO settlement left at all,
// there is nowhere to dispatch a leg TO, so the ship is stranded on the spot
// instead (the half-cargo is lost either way — there's no settlement to
// credit it to).
func (h *InterceptScanHandler) dispatchLimpedReturn(ctx context.Context, tx pgx.Tx, worldID uuid.UUID, t inFlightTransport, pos province.MapPosition, goods []manifestLine) error {
	half := Manifest{}
	for _, g := range goods {
		if remaining := g.Quantity / 2; remaining > 0 {
			half[g.GoodKey] = remaining
		}
	}

	destID := t.originID
	if destID == nil {
		if portID, _, _, found, perr := NearestOwnPort(ctx, tx, worldID, t.owner, t.originQ, t.originR); perr == nil && found {
			destID = &portID
		}
	}
	if destID == nil {
		return StrandShip(ctx, tx, *t.shipUnitID, pos.Q, pos.R)
	}

	travelMins := 30.0
	if path, _, ok, err := province.FindPath(ctx, tx, worldID,
		pos, province.MapPosition{Q: t.originQ, R: t.originR}, "naval"); err == nil && ok {
		travelMins = 30.0 + float64(len(path)-1)*2.0
	}
	travelTicks := int(math.Round(travelMins / 60))
	if travelTicks < 1 {
		travelTicks = 1
	}
	var currentTick int
	_ = tx.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
	departsAt := h.clk.Now()
	arrivesAt := departsAt.Add(time.Duration(travelMins * float64(time.Minute)))

	_, err := Dispatch(ctx, tx, h.scheduler, DispatchParams{
		WorldID: worldID, OwnerID: t.owner, Kind: "damaged_return",
		OriginID: *destID, DestID: *destID, Category: "naval",
		OriginQ: pos.Q, OriginR: pos.R, DestQ: t.originQ, DestR: t.originR,
		DepartsAt: departsAt, ArrivesAt: arrivesAt, DueTick: currentTick + travelTicks,
		Manifest: half, Interceptable: true, ShipUnitID: t.shipUnitID,
	})
	return err
}

// manifestLine is one good in a seized caravan's notice payload — same
// {good_key, quantity} shape StandingOrderDispatched carries, so every client
// formats cargo the same way.
type manifestLine struct {
	GoodKey  string  `json:"good_key"`
	Quantity float64 `json:"quantity"`
}

func loadManifest(ctx context.Context, tx pgx.Tx, transportID uuid.UUID) ([]manifestLine, error) {
	rows, err := tx.Query(ctx,
		`SELECT good_key, quantity FROM transport_goods
		 WHERE transport_id = $1 AND quantity > 0 ORDER BY quantity DESC, good_key`, transportID)
	if err != nil {
		return nil, fmt.Errorf("load manifest: %w", err)
	}
	defer rows.Close()
	goods := []manifestLine{}
	for rows.Next() {
		var g manifestLine
		if err := rows.Scan(&g.GoodKey, &g.Quantity); err != nil {
			return nil, fmt.Errorf("scan manifest: %w", err)
		}
		goods = append(goods, g)
	}
	return goods, rows.Err()
}

// straightLineHexPosition returns the caravan's position along a straight cube-
// coordinate line between origin and dest, at the same elapsed-time fraction
// InterpolatePosition uses (clamped the same way at both ends). Only used when
// InterpolatePosition finds no traversable route for the caravan's category —
// see the call site in Handle for why a straight line is the right fallback here
// rather than a permanent skip.
func straightLineHexPosition(origin, dest province.MapPosition, departsAt, arrivesAt, now time.Time) province.MapPosition {
	total := arrivesAt.Sub(departsAt)
	if total <= 0 || !now.Before(arrivesAt) {
		return dest
	}
	if !now.After(departsAt) {
		return origin
	}
	frac := float64(now.Sub(departsAt)) / float64(total)
	return hexLerp(origin, dest, frac)
}

// hexLerp linearly interpolates between two axial hex coordinates in cube space
// and rounds the result back to the nearest hex (Red Blob Games hex-lerp +
// cube-round: https://www.redblobgames.com/grids/hexagons/#line-drawing).
func hexLerp(a, b province.MapPosition, frac float64) province.MapPosition {
	ax, az := float64(a.Q), float64(a.R)
	ay := -ax - az
	bx, bz := float64(b.Q), float64(b.R)
	by := -bx - bz

	x := ax + (bx-ax)*frac
	y := ay + (by-ay)*frac
	z := az + (bz-az)*frac

	rx := math.Round(x)
	ry := math.Round(y)
	rz := math.Round(z)

	dx := math.Abs(rx - x)
	dy := math.Abs(ry - y)
	dz := math.Abs(rz - z)

	switch {
	case dx > dy && dx > dz:
		rx = -ry - rz
	case dy > dz:
		ry = -rx - rz
	default:
		rz = -rx - ry
	}

	return province.MapPosition{Q: int(rx), R: int(rz)}
}
