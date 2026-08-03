package economy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Event types. NEW types, not new "kinds" on SitosTransaction/SitosFundRelease —
// those two carry silver meaning and their semantics are frozen forever
// (CLAUDE.md "Events"). Old handlers keep reading the old types; the rows
// already in `events` are untouched and still mean what they meant.
const (
	EventSitosGranaryStored   = "SitosGranaryStored"
	EventSitosGranaryReleased = "SitosGranaryReleased"
)

// NotifySitosGranaryRelease is the player-facing kind for a release. Under the
// fund, "intervention" fired constantly and was filtered as noise
// (noisyNotificationKinds in cmd_notifications.go). A release is the opposite:
// rare, and the moment the city ate its reserve. It gets a kind of its own so
// it is NOT swept up by those filters.
const NotifySitosGranaryRelease = "SitosGranaryRelease"

// SitosGranaryPayload is the outcome of one granary movement (events store
// outcomes, not intentions). Total and PerGood are FOOD UNITS, always ≥ 0;
// the event type says which direction. CoverageDays is the coverage that
// triggered it, recorded so a later reader can see why it fired without having
// to reconstruct the stock.
type SitosGranaryPayload struct {
	Total        float64            `json:"total"`
	PerGood      map[string]float64 `json:"per_good"`
	CoverageDays float64            `json:"coverage_days"`
	GranaryAfter float64            `json:"granary_after"`
	GranaryCap   float64            `json:"granary_cap"`
}

// SitosTickHandler is the self-rescheduling per-world granary pass. It runs
// every tick (cadence +1): a city with more than HighDays of food covered puts
// a tithe of the surplus aside; a city below LowDays eats from what it put
// aside, if anything is there.
//
// It touches NO SILVER (B3). Before migration 106 this was a silver fund that
// taxed every settlement per head, and that head tax produced 2132 desertions
// — all silver_shortage — while stabilising nothing.
//
// Idempotent (CLAUDE.md "Event handlers"): tickSettlement claims (event_id,
// settlement_id) in processed_sitos_ticks (migration 097) inside the SAME
// transaction as its writes, so a worker retry of the same ScheduledSitosTick
// event (crash between commit and markDone, or a dead-letter replay) resumes
// rather than double-storing — settlements already committed for this event
// short-circuit, any not yet reached proceed normally.
type SitosTickHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
	store     *events.Store
	hub       Broadcaster
	cfg       SitosConfig
}

// NewSitosTickHandler creates a SitosTickHandler.
func NewSitosTickHandler(pool *pgxpool.Pool, sched *events.Scheduler, store *events.Store, hub Broadcaster, cfg SitosConfig) *SitosTickHandler {
	return &SitosTickHandler{pool: pool, scheduler: sched, store: store, hub: hub, cfg: cfg}
}

// Handle processes one ScheduledSitosTick event for a world.
func (h *SitosTickHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	rows, err := h.pool.Query(ctx,
		`SELECT id FROM settlements
		 WHERE world_id = $1 AND owner_id IS NOT NULL AND state NOT IN ('sunk', 'collapsed')`,
		e.WorldID,
	)
	if err != nil {
		return fmt.Errorf("sitos tick: query settlements: %w", err)
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
		return err
	}

	for _, id := range ids {
		if err := h.tickSettlement(ctx, id, e.WorldID, e.ID); err != nil {
			slog.Error("sitos tick: settlement failed", "settlement", id, "err", err)
		}
	}

	// Reschedule next tick (cadence +1). Last line, matching the kharis/colony
	// precedent: a reschedule failure is the only thing that retries the pass.
	return h.scheduler.EnqueueTickRecurring(ctx, e.WorldID, events.ScheduledSitosTick,
		struct{}{}, e.DueTick, 1)
}

// foodRow is one subsistence good's state in the city, as the granary sees it.
type foodRow struct {
	good  string
	stock float64 // settled amount, floored at 0
	cap   float64
}

// tickSettlement runs the granary evaluation for one settlement in a single TX.
func (h *SitosTickHandler) tickSettlement(ctx context.Context, settlementID, worldID uuid.UUID, eventID int64) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Exactly-once claim, scoped to (event_id, settlement_id) rather than
	// event_id alone: Handle fans ONE ScheduledSitosTick event out across every
	// settlement, each committing its own transaction here, so a Handle-level
	// claim would either falsely skip settlements never reached before a crash,
	// or falsely mark them done before their writes commit.
	claim, err := tx.Exec(ctx,
		`INSERT INTO processed_sitos_ticks (event_id, settlement_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		eventID, settlementID)
	if err != nil {
		return fmt.Errorf("claim sitos tick: %w", err)
	}
	if claim.RowsAffected() == 0 {
		return nil // already processed this event for this settlement
	}

	var population int
	if err := tx.QueryRow(ctx,
		`SELECT population FROM settlements WHERE id = $1 FOR UPDATE`,
		settlementID,
	).Scan(&population); err != nil {
		return fmt.Errorf("load settlement: %w", err)
	}

	foods, foodStock, err := h.loadFood(ctx, tx, settlementID)
	if err != nil {
		return fmt.Errorf("load food: %w", err)
	}
	granary, granaryTotal, err := loadGranary(ctx, tx, settlementID)
	if err != nil {
		return fmt.Errorf("load granary: %w", err)
	}

	coverage := CoverageDays(foodStock, population)
	action := EvaluateGranaryAction(foodStock, granaryTotal, population, h.cfg)

	var moved map[string]float64
	switch action.Kind {
	case "store":
		moved, err = h.store_(ctx, tx, settlementID, foods, foodStock, action.Quantity)
	case "release":
		moved, err = h.release(ctx, tx, settlementID, foods, granary, granaryTotal, action.Quantity)
	default:
		moved = nil
	}
	if err != nil {
		return fmt.Errorf("%s: %w", action.Kind, err)
	}

	var total float64
	for _, q := range moved {
		total += q
	}
	if total > 0 {
		after := granaryTotal + total
		if action.Kind == "release" {
			after = granaryTotal - total
		}
		eventType := EventSitosGranaryStored
		if action.Kind == "release" {
			eventType = EventSitosGranaryReleased
		}
		_, _ = h.store.Append(ctx, settlementID, events.StreamProvince, eventType,
			SitosGranaryPayload{
				Total: total, PerGood: moved, CoverageDays: coverage,
				GranaryAfter: after, GranaryCap: GranaryCap(population, h.cfg),
			},
			worldID, nil)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// A release is the city eating its reserve — the one moment a Wanax needs
	// to hear about. Storing is routine and stays silent, as the fund's "buy"
	// leg did. Best-effort, outside the tx.
	if h.hub != nil && action.Kind == "release" && total > 0 {
		var ownerID uuid.UUID
		if err := h.pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id = $1`, settlementID).Scan(&ownerID); err == nil {
			_ = h.hub.NotifyPlayer(ctx, worldID, ownerID, NotifySitosGranaryRelease, 2, map[string]any{
				"settlement_id":  settlementID,
				"food_released":  total,
				"coverage_days":  coverage,
				"granary_after":  granaryTotal - total,
				"granary_empty":  granaryTotal-total <= 0,
				"per_good":       moved,
			})
		}
	}
	return nil
}

// loadFood reads the city's subsistence goods and their total settled stock.
// Goods the city does not track (a landlocked city has no fish row) are simply
// absent — the granary can only ever hold what the city could hand it.
func (h *SitosTickHandler) loadFood(ctx context.Context, tx pgx.Tx, settlementID uuid.UUID) ([]foodRow, float64, error) {
	var out []foodRow
	var total float64
	for _, good := range h.cfg.SubsistenceGoods {
		var stock, cap float64
		err := tx.QueryRow(ctx,
			`SELECT GREATEST(0, settled(amount, rate, calc_tick)), COALESCE(cap, 0)
			 FROM settlement_goods WHERE settlement_id = $1 AND good_key = $2`,
			settlementID, good,
		).Scan(&stock, &cap)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, 0, err
		}
		out = append(out, foodRow{good: good, stock: stock, cap: cap})
		total += stock
	}
	return out, total, nil
}

// loadGranary reads what the granary holds, per good, plus the total.
func loadGranary(ctx context.Context, tx pgx.Tx, settlementID uuid.UUID) (map[string]float64, float64, error) {
	rows, err := tx.Query(ctx,
		`SELECT good_key, amount FROM settlement_granary WHERE settlement_id = $1`,
		settlementID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := map[string]float64{}
	var total float64
	for rows.Next() {
		var good string
		var amount float64
		if err := rows.Scan(&good, &amount); err != nil {
			return nil, 0, err
		}
		out[good] = amount
		total += amount
	}
	return out, total, rows.Err()
}

// store_ moves `quantity` food units from the city into its granary, split
// across the food goods in proportion to what the city holds of each — a city
// living on fish contributes fish. Each good's share is at most its own stock
// (quantity ≤ the surplus ≤ the total stock, by EvaluateGranaryAction), so the
// GREATEST(0, …) below can never clip and food is conserved exactly.
//
// Trailing underscore because `store` is the events.Store field on the handler.
func (h *SitosTickHandler) store_(ctx context.Context, tx pgx.Tx, settlementID uuid.UUID, foods []foodRow, foodStock, quantity float64) (map[string]float64, error) {
	if foodStock <= 0 || quantity <= 0 {
		return nil, nil
	}
	moved := map[string]float64{}
	for _, f := range foods {
		take := quantity * f.stock / foodStock
		if take <= 0 {
			continue
		}
		if _, err := tx.Exec(ctx,
			`UPDATE settlement_goods
			    SET amount = GREATEST(0, settled(amount, rate, calc_tick) - $1),
			        calc_tick = current_world_tick()
			  WHERE settlement_id = $2 AND good_key = $3`,
			take, settlementID, f.good,
		); err != nil {
			return nil, fmt.Errorf("debit %s: %w", f.good, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO settlement_granary (settlement_id, good_key, amount)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (settlement_id, good_key) DO UPDATE SET amount = settlement_granary.amount + $3`,
			settlementID, f.good, take,
		); err != nil {
			return nil, fmt.Errorf("credit granary %s: %w", f.good, err)
		}
		moved[f.good] = take
	}
	return moved, nil
}

// release moves `quantity` food units from the granary back into the city,
// split across what the granary holds. Each good's share is gated by the CITY's
// own cap headroom BEFORE anything moves: crediting past the cap would have the
// LEAST() clamp swallow the difference, and the food would be gone from both
// sides. Whatever headroom refuses stays in the granary and goes out on a later
// tick.
func (h *SitosTickHandler) release(ctx context.Context, tx pgx.Tx, settlementID uuid.UUID, foods []foodRow, granary map[string]float64, granaryTotal, quantity float64) (map[string]float64, error) {
	if granaryTotal <= 0 || quantity <= 0 {
		return nil, nil
	}
	moved := map[string]float64{}
	for _, f := range foods {
		held := granary[f.good]
		if held <= 0 {
			continue
		}
		give := quantity * held / granaryTotal
		if give > held {
			give = held
		}
		if headroom := f.cap - f.stock; give > headroom {
			give = headroom
		}
		if give <= 0 {
			continue
		}
		if _, err := tx.Exec(ctx,
			`UPDATE settlement_goods
			    SET amount = LEAST(cap, settled(amount, rate, calc_tick) + $1),
			        calc_tick = current_world_tick()
			  WHERE settlement_id = $2 AND good_key = $3`,
			give, settlementID, f.good,
		); err != nil {
			return nil, fmt.Errorf("credit %s: %w", f.good, err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE settlement_granary SET amount = GREATEST(0, amount - $1)
			  WHERE settlement_id = $2 AND good_key = $3`,
			give, settlementID, f.good,
		); err != nil {
			return nil, fmt.Errorf("debit granary %s: %w", f.good, err)
		}
		moved[f.good] = give
	}
	return moved, nil
}

// GranaryTotals reads a settlement's granary contents and total for the read
// surfaces (api/handlers). Exported here rather than duplicated as a query in
// the handler so the shown reserve can never drift from the stored one.
func GranaryTotals(ctx context.Context, pool *pgxpool.Pool, settlementID uuid.UUID) (map[string]float64, float64, error) {
	rows, err := pool.Query(ctx,
		`SELECT good_key, amount FROM settlement_granary WHERE settlement_id = $1`,
		settlementID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := map[string]float64{}
	var total float64
	for rows.Next() {
		var good string
		var amount float64
		if err := rows.Scan(&good, &amount); err != nil {
			return nil, 0, err
		}
		out[good] = amount
		total += amount
	}
	return out, total, rows.Err()
}
