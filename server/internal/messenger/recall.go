package messenger

// RecallArrival: when a recall-order messenger reaches the unit, this handler
// fires and starts the actual return march / outpost teardown.
//
// Two sub-types share the same event type, distinguished by the Kind field:
//   - "march"   — turn an in-flight army around and march it home
//   - "outpost" — tear down an outpost and march the garrison home
//
// Both are idempotent via an atomic conditional claim (CLAUDE.md "Event handlers"):
// the return work only happens if the unit is still recallable when the messenger arrives.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/timescale"
	"github.com/poleia/server/internal/combat"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/province"
)

// HoursPerHex is the travel speed of a messenger (hours per hex). Shared by diplomatic
// messengers (api/handlers messenger send) and recall messengers so the rate lives in one place.
// A future "blessed messengers" rite would turn this into a multiplier-driven value (see thalassa_todo).
const HoursPerHex = 0.5

// RecallMarchPayload is the RecallArrival payload for an in-flight army recall.
type RecallMarchPayload struct {
	Kind          string    `json:"kind"` // "march"
	WorldID       uuid.UUID `json:"world_id"`
	MessengerID   uuid.UUID `json:"messenger_id"` // the visible recall messenger, marked arrived on fire
	MarchID       uuid.UUID `json:"march_id"`
	Infantry      int       `json:"infantry"`
	Chariot       int       `json:"chariot"`
	Priest        int       `json:"priest"`
	Ship          int       `json:"ship"` // galley
	EliteInfantry int       `json:"elite_infantry"`
	WarGalley     int       `json:"war_galley"`
	Merchantman   int       `json:"merchantman"`
	// The messenger is sent to the army's target province (the simplest fixed-point target).
	// Assumption: for an in-flight army the messenger aims for the destination, not the army's
	// instantaneous mid-march position. The return march is modelled as departing from the target.
	// This avoids moving-target interpolation and is always physically safe (never faster than physics).
	OriginQ  int       `json:"origin_q"`
	OriginR  int       `json:"origin_r"`
	TargetQ  int       `json:"target_q"`
	TargetR  int       `json:"target_r"`
	OriginID uuid.UUID `json:"origin_id"` // province the return march goes back to (home)
	TargetID uuid.UUID `json:"target_id"` // province the return march departs from
}

// RecallOutpostPayload is the RecallArrival payload for an outpost recall.
type RecallOutpostPayload struct {
	Kind           string    `json:"kind"` // "outpost"
	WorldID        uuid.UUID `json:"world_id"`
	MessengerID    uuid.UUID `json:"messenger_id"`
	ProvinceID     uuid.UUID `json:"province_id"` // the outpost province
	HomeID         uuid.UUID `json:"home_id"`     // province the garrison returns to
	Infantry       int       `json:"infantry"`
	Chariot        int       `json:"chariot"`
	Priest         int       `json:"priest"`
	Ship           int       `json:"ship"` // galley
	EliteInfantry  int       `json:"elite_infantry"`
	WarGalley      int       `json:"war_galley"`
	Merchantman    int       `json:"merchantman"`
	OutpostTerrain string    `json:"outpost_terrain"`
	OutpostQ       int       `json:"outpost_q"`
	OutpostR       int       `json:"outpost_r"`
	HomeQ          int       `json:"home_q"`
	HomeR          int       `json:"home_r"`
}

// RecallArrivalHandler processes RecallArrival scheduled events.
// It is registered with events.Worker and must be idempotent.
type RecallArrivalHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
	clk       clock.Clock
}

// NewRecallArrivalHandler creates a RecallArrivalHandler.
func NewRecallArrivalHandler(pool *pgxpool.Pool, sched *events.Scheduler, clk clock.Clock) *RecallArrivalHandler {
	return &RecallArrivalHandler{pool: pool, scheduler: sched, clk: clk}
}

// Handle dispatches to the correct sub-handler based on the Kind field.
func (h *RecallArrivalHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var peek struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(e.Payload, &peek); err != nil {
		return fmt.Errorf("unmarshal recall arrival kind: %w", err)
	}
	switch peek.Kind {
	case "march":
		return h.handleMarch(ctx, e)
	case "outpost":
		return h.handleOutpost(ctx, e)
	default:
		return fmt.Errorf("unknown recall kind: %q", peek.Kind)
	}
}

// handleMarch turns a recalled in-flight army around once the recall messenger reaches it.
// Idempotent + "command isn't instant": the army keeps marching until this fires. We claim the
// outbound march with an atomic conditional UPDATE; if it was already resolved (the army arrived
// and fought first, or this event already fired), the recall is a harmless no-op.
func (h *RecallArrivalHandler) handleMarch(ctx context.Context, e events.ScheduledEvent) error {
	var p RecallMarchPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal recall march payload: %w", err)
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// The messenger has physically reached the army — mark it arrived so it stops rendering,
	// whether or not the recall still catches the army.
	if p.MessengerID != uuid.Nil {
		_, _ = tx.Exec(ctx, `UPDATE messengers SET status='arrived' WHERE id=$1`, p.MessengerID)
	}

	// Atomic claim: only turn the army around if its outbound march is still unresolved.
	ct, err := tx.Exec(ctx,
		`UPDATE marching_armies SET resolved=true WHERE id=$1 AND resolved=false`, p.MarchID)
	if err != nil {
		return fmt.Errorf("claim outbound march: %w", err)
	}
	if ct.RowsAffected() == 0 {
		slog.Info("recall messenger arrived but army already resolved — recall missed", "march_id", p.MarchID)
		return tx.Commit(ctx)
	}

	// Return trip = full distance origin↔target × terrain cost (army turns around at the target).
	var targetTerrain string
	if err := tx.QueryRow(ctx, `SELECT terrain_type FROM provinces WHERE id=$1`, p.TargetID).Scan(&targetTerrain); err != nil || targetTerrain == "" {
		targetTerrain = "plains"
	}
	dist := province.HexDistance(
		province.MapPosition{Q: p.OriginQ, R: p.OriginR},
		province.MapPosition{Q: p.TargetQ, R: p.TargetR},
	)
	now := h.clk.Now()
	returnsAt := now.Add(returnDuration(dist, targetTerrain))

	var returnMarchID uuid.UUID
	if err := tx.QueryRow(ctx,
		`INSERT INTO marching_armies
		 (world_id, origin_id, target_id, infantry, chariot, priest, ship, elite_infantry,
		  war_galley, merchantman, intent, departs_at, arrives_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'return',$11,$12)
		 RETURNING id`,
		p.WorldID, p.TargetID, p.OriginID,
		p.Infantry, p.Chariot, p.Priest, p.Ship, p.EliteInfantry,
		p.WarGalley, p.Merchantman,
		now, returnsAt,
	).Scan(&returnMarchID); err != nil {
		return fmt.Errorf("create return march: %w", err)
	}

	// Atomic with the claim — no orphan return march if we crash before the worker marks done.
	if err := h.scheduler.EnqueueTx(ctx, tx, p.WorldID, events.ScheduledArmyArrival,
		combat.ArmyArrivalPayload{MarchingArmyID: returnMarchID}, returnsAt); err != nil {
		return fmt.Errorf("schedule army arrival: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	slog.Info("recall messenger reached army, return march started",
		"march_id", p.MarchID, "return_march_id", returnMarchID, "returns_at", returnsAt, "dist", dist)
	return nil
}

// handleOutpost tears down the outpost and sends the garrison home once the recall messenger arrives.
// Idempotent: the province is freed with an atomic conditional UPDATE (owner_id IS NOT NULL); a
// duplicate fire (or an outpost already captured/freed) matches 0 rows and does nothing.
func (h *RecallArrivalHandler) handleOutpost(ctx context.Context, e events.ScheduledEvent) error {
	var p RecallOutpostPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal recall outpost payload: %w", err)
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if p.MessengerID != uuid.Nil {
		_, _ = tx.Exec(ctx, `UPDATE messengers SET status='arrived' WHERE id=$1`, p.MessengerID)
	}

	// Atomic claim + free the province. If it is no longer an owned outpost, the recall is a no-op.
	ct, err := tx.Exec(ctx,
		`UPDATE provinces SET territory_state='free', owner_id=NULL, outpost_feeds=NULL, garrison_strength=0
		 WHERE id=$1 AND owner_id IS NOT NULL`,
		p.ProvinceID,
	)
	if err != nil {
		return fmt.Errorf("claim/free outpost province: %w", err)
	}
	if ct.RowsAffected() == 0 {
		slog.Info("recall messenger arrived but outpost already gone — recall missed", "province", p.ProvinceID)
		return tx.Commit(ctx)
	}

	// Settle-then-subtract the ledgered production flows this outpost fed home.
	rows, err := tx.Query(ctx,
		`SELECT settlement_id, good_key, rate FROM outpost_flows WHERE province_id = $1`, p.ProvinceID,
	)
	if err != nil {
		return fmt.Errorf("load outpost flows: %w", err)
	}
	type flow struct {
		settlementID uuid.UUID
		key          string
		rate         float64
	}
	var flows []flow
	for rows.Next() {
		var f flow
		if err := rows.Scan(&f.settlementID, &f.key, &f.rate); err == nil {
			flows = append(flows, f)
		}
	}
	rows.Close()

	for _, f := range flows {
		if _, err := tx.Exec(ctx,
			`UPDATE settlement_goods SET
			     amount  = LEAST(cap, settled(amount, rate, calc_at)),
			     rate    = GREATEST(0, rate - $3),
			     calc_at = now()
			 WHERE settlement_id = $1 AND good_key = $2`,
			f.settlementID, f.key, f.rate,
		); err != nil {
			return fmt.Errorf("subtract outpost flow: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM outpost_flows WHERE province_id = $1`, p.ProvinceID); err != nil {
		return fmt.Errorf("delete outpost flows: %w", err)
	}

	// March the garrison home (if any).
	var returnMarchID uuid.UUID
	var returnsAt time.Time
	if p.Infantry+p.Chariot+p.Priest+p.Ship+p.EliteInfantry+p.WarGalley+p.Merchantman > 0 {
		dist := province.HexDistance(
			province.MapPosition{Q: p.OutpostQ, R: p.OutpostR},
			province.MapPosition{Q: p.HomeQ, R: p.HomeR},
		)
		now := h.clk.Now()
		returnsAt = now.Add(returnDuration(dist, p.OutpostTerrain))
		if err := tx.QueryRow(ctx,
			`INSERT INTO marching_armies
			 (world_id, origin_id, target_id, infantry, chariot, priest, ship, elite_infantry,
			  war_galley, merchantman, intent, departs_at, arrives_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'return',$11,$12)
			 RETURNING id`,
			p.WorldID, p.ProvinceID, p.HomeID,
			p.Infantry, p.Chariot, p.Priest, p.Ship, p.EliteInfantry,
			p.WarGalley, p.Merchantman,
			now, returnsAt,
		).Scan(&returnMarchID); err != nil {
			return fmt.Errorf("create garrison return march: %w", err)
		}
		if err := h.scheduler.EnqueueTx(ctx, tx, p.WorldID, events.ScheduledArmyArrival,
			combat.ArmyArrivalPayload{MarchingArmyID: returnMarchID}, returnsAt); err != nil {
			return fmt.Errorf("schedule garrison arrival: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	if returnMarchID != uuid.Nil {
		slog.Info("recall messenger reached outpost, garrison return march started",
			"province", p.ProvinceID, "return_march_id", returnMarchID, "returns_at", returnsAt)
	} else {
		slog.Info("recall messenger reached outpost, province freed (no garrison to return)", "province", p.ProvinceID)
	}
	return nil
}

// returnDuration is the travel time of a return march over dist hexes of the given terrain,
// with a 6-minute floor. Pure function — unit-tested.
func returnDuration(dist int, terrain string) time.Duration {
	hours := float64(dist) * province.TerrainMoveHours(terrain)
	if hours < 0.1 {
		hours = 0.1 // minimum 6 minutes
	}
	return timescale.Apply(time.Duration(hours * float64(time.Hour)))
}

// MessengerTravelDuration returns the travel time for a recall messenger over dist hexes.
// Pure function — exported for testing and reused by the recall HTTP handlers.
func MessengerTravelDuration(dist int) time.Duration {
	return timescale.Apply(time.Duration(float64(dist) * HoursPerHex * float64(time.Hour)))
}

// TradeHoursPerHex is the travel speed of a trade caravan (the silver/goods legs of a messenger trade).
// Kept as a separate seam from messengers so caravans can later be tuned slower than runners
// without affecting messenger/recall speed.
const TradeHoursPerHex = 0.5

// TradeTravelDuration returns the travel time for a trade caravan over dist hexes. Pure function.
func TradeTravelDuration(dist int) time.Duration {
	return timescale.Apply(time.Duration(float64(dist) * TradeHoursPerHex * float64(time.Hour)))
}
