package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"formatet/megaron/server/internal/clock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MacroTickInterval is how often the macro handlers (upkeep, loyalty decay,
// welfare, colony penalty, borrowed army, kharis) fire, in ticks. A tick is a
// day in the world, and day-scale consequences (starvation, desertion,
// loyalty drift) land once per day — so this is 1.
//
// It replaces TicksPerDay, which was deleted 2026-08-06 (Timothy): that
// constant conflated two different questions — "how often do macro
// consequences land" and "how many ticks make up a day" — and the second
// question no longer exists now that a tick IS a day, the world's indivisible
// unit of time. TicksPerDay was 24 (a tick was a game-HOUR) until the
// day/tick ambiguity was traced as a root cause: "day" meant both 24 ticks
// and a wall-clock day, and nothing in a name said which. That ambiguity
// already cost one production bug (mig 071, universal starvation) and had
// left temenos_varutaxonomi_sol.md 24× out of phase with the code for weeks —
// both documents writing the same numbers under different clocks.
//
// Production rates are per-tick (production_rules.rate_per_tick, mig 071); the
// per-minute unit has been retired. Real-time pacing is a SEPARATE, unlocked
// axis (TICK_MINUTES in internal/tick — e.g. 2 min/tick ≈ one game-month per
// real day) and does not live here. The broader economy re-balance is tracked
// separately — see temenos_ekonomi.md.
//
// DO NOT reintroduce a ticks-per-day concept — the calendar derives from
// 365 ticks/year (temenos_tid_kalender_plan.md, Låst regel 2), never from this.
//
// NEVER write "day" as a UNIT of game time again — in code, JSON, or prose.
// Only "tick". "Day" unqualified always means wall clock.
const MacroTickInterval = 1

// ScheduledEventType identifies what should happen when the event fires.
type ScheduledEventType string

const (
	ScheduledArmyArrival      ScheduledEventType = "ArmyArrival"
	ScheduledBuildComplete    ScheduledEventType = "BuildComplete"
	ScheduledTrainComplete    ScheduledEventType = "TrainComplete"
	ScheduledCollapseCheck    ScheduledEventType = "CollapseCheck"
	ScheduledDivineRoll       ScheduledEventType = "DivineRoll"
	ScheduledLoyaltyDecayTick ScheduledEventType = "LoyaltyDecayTick"
	// L1 — daily loyalty welfare signals (kharis favour, feeding, starvation,
	// diet variety), internal/loyalty/welfare.go.
	ScheduledLoyaltyWelfareTick ScheduledEventType = "LoyaltyWelfareTick"
	ScheduledColonyPenaltyTick  ScheduledEventType = "ColonyPenaltyTick"
	ScheduledBorrowedArmyTick   ScheduledEventType = "BorrowedArmyTick"
	ScheduledMessengerArrival   ScheduledEventType = "MessengerArrival"
	ScheduledMessengerReturn    ScheduledEventType = "MessengerReturn"
	ScheduledKharisTick         ScheduledEventType = "KharisTick"
	ScheduledTradeDelivery      ScheduledEventType = "TradeDelivery"
	ScheduledTradeReturn        ScheduledEventType = "TradeReturn"
	ScheduledRecallArrival      ScheduledEventType = "RecallArrival"
	ScheduledLogisticsArrival   ScheduledEventType = "LogisticsArrival"
	// Physical goods transport (movement-motor transport layer) — a caravan/ship
	// carrying a goods manifest arrives at its destination. Supersedes the abstract
	// LogisticsArrival for movers that have a map position (see internal/transport).
	ScheduledTransportArrival ScheduledEventType = "TransportArrival"
	// Recurring sweep that intercepts in-transit caravans passing an enemy sentry
	// (movement-motor Slice C). Messengers are never scanned — sacred/uninterceptable.
	ScheduledInterceptScan ScheduledEventType = "InterceptScan"
	// ScheduledMarchSightingScan is the recurring sweep that notifies a Wanax the
	// first time a foreign march becomes visible to them (asynkronitetsgrinden).
	ScheduledMarchSightingScan ScheduledEventType = "MarchSightingScan"
	// C1 — unit model; handler registered in C2+.
	ScheduledUnitArrival ScheduledEventType = "UnitArrival"
	// Naval sentry patrol: a ship posted on sentry at a coastal_sea hex auto-returns
	// home when its patrol timer fires (SentryPatrolTicks after arrival). No recall —
	// the timer is the only control (self-terminating sea order).
	ScheduledSentryReturn ScheduledEventType = "SentryReturn"
	// C-collapse — city exhausted to ≤100 pop; warband spawned.
	ScheduledCollapseSettlement ScheduledEventType = "CollapseSettlement"
	// W4e — daily grain+silver upkeep for all active units.
	ScheduledUpkeepTick ScheduledEventType = "UpkeepTick"
	// Escrow expiry: refund buyer silver if a trade offer expires without being accepted.
	ScheduledOfferExpiry ScheduledEventType = "OfferExpiry"
	// Sitos-fonden: self-rescheduling stabilization pass, cadence +1 tick (every
	// tick, not daily) — see internal/economy/sitos_tick.go.
	ScheduledSitosTick ScheduledEventType = "SitosTick"
	// March recall/redirect: a recall or redirect messenger reaching a marching
	// discrete unit (temenos_march_recall.md). Distinct from ScheduledRecallArrival,
	// which turns around legacy marching_armies/outposts.
	ScheduledMarchRecall ScheduledEventType = "MarchRecall"
	// ScheduledOrderDelivery: an order courier reaching its unit — the carried
	// order (march etc.) executes on delivery (temenos_orderlopare_plan.md Fas 2).
	ScheduledOrderDelivery ScheduledEventType = "OrderDelivery"
	// ScheduledUnitInterceptScan is the recurring sweep that resolves combat
	// between a marching unit and a FOW-visible enemy sentry within reach
	// (avsiktslagret §S3, megaron_plan_avsiktslagret.md) — the unit-vs-unit
	// counterpart of ScheduledInterceptScan, which only ever covers caravans.
	// Substrate for KR2 (marscherande härar möts).
	ScheduledUnitInterceptScan ScheduledEventType = "UnitInterceptScan"
	// ScheduledBattleTick advances one active KR3 battle (megaron_plan_kr3_stridssystem.md
	// §2) by one battle-tick: sample participation, resolve battleRoundsPerTick
	// rounds, apply losses, check termination, and — unless the battle just
	// ended — re-enqueue itself for the next tick. One instance of this event
	// exists per ACTIVE battle (unlike the world-wide recurring scans above),
	// created by initiateOrJoinBattle and self-perpetuating via
	// EnqueueTickRecurring until the battle ends.
	ScheduledBattleTick ScheduledEventType = "BattleTick"
	// ScheduledOccupationCheck watches ONE occupied settlement's annex counter
	// (megaron_plan_erovring.md S2, combat/occupation_check.go): fires at the
	// tick the occupation is due to mature, re-reads the live counter (an
	// attack resets it), and either notifies the occupant they may annex or
	// re-enqueues itself for the (possibly pushed-back) new deadline. One
	// instance per occupied settlement, created by occupySettlement/
	// resetOccupationDefense, same self-correcting shape as ScheduledBattleTick.
	ScheduledOccupationCheck ScheduledEventType = "OccupationCheck"
	// ScheduledMarchEncounterScan is the recurring sweep that detects two
	// FIENTLIGA marching units sharing the same hex right now (KR2,
	// megaron_plan_kr2_motet.md — "samma hex" is a GROUP BY on interpolated
	// position, not a proximity radius). Before this, two crossing marches
	// passed straight through each other: combat only ever triggered at
	// ARRIVAL (ScheduledUnitArrival) or against a STATIONARY sentry
	// (ScheduledUnitInterceptScan) — never between two columns both still
	// mid-route. Same substrate as ScheduledUnitInterceptScan (preloaded
	// TileGraph, avsiktslagret reaction_policy), combat/march_encounter.go.
	ScheduledMarchEncounterScan ScheduledEventType = "MarchEncounterScan"
)

// ScheduledEvent is a pending game event stored durably in PostgreSQL.
type ScheduledEvent struct {
	ID           int64
	WorldID      uuid.UUID
	EventType    ScheduledEventType
	Payload      json.RawMessage
	ProcessAfter time.Time
	ProcessedAt  *time.Time
	FailedAt     *time.Time
	Attempts     int
	DueTick      int
}

// Scheduler enqueues timed game events.
type Scheduler struct {
	pool  *pgxpool.Pool
	clock clock.Clock
}

// NewScheduler creates a Scheduler backed by the given connection pool.
// clk is used for audit timestamps — pass a TestClock in tests.
func NewScheduler(pool *pgxpool.Pool, clk clock.Clock) *Scheduler {
	return &Scheduler{pool: pool, clock: clk}
}

// Clock returns the scheduler's clock (used by delivery handlers for chained events).
func (s *Scheduler) Clock() clock.Clock { return s.clock }

// EnqueueTick schedules a new event to fire when worlds.current_tick reaches dueTick.
// process_after is set to now() to satisfy the NOT NULL constraint; the scheduler
// fires this event via the due_tick path only.
func (s *Scheduler) EnqueueTick(ctx context.Context, worldID uuid.UUID, eventType ScheduledEventType, payload any, dueTick int) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal scheduled payload: %w", err)
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO scheduled_events (world_id, event_type, payload, process_after, due_tick)
		 VALUES ($1, $2, $3, now(), $4)`,
		worldID, string(eventType), raw, dueTick,
	)
	return err
}

// EnqueueTickRecurring schedules the next firing of a self-rescheduling
// periodic event: nextDue = lastDue + intervalTicks, clamped to never land in
// the past. If the previous firing ran late (catch-up after the queue fell
// behind), a bare lastDue+interval would still be overdue and the handler
// would re-fire every worker poll until it replayed every missed interval —
// the KharisTick treadmill (2026-07-13, six sea_blessing ships in six
// minutes). Missed intervals are skipped, not replayed: rates are lazy and
// continuous, so only the discrete consequence check is periodic.
//
// G2 tick-dedup (megaron_plan_tickdedup.md, mig 127): this line sits as the
// LAST statement of every self-rescheduling handler, so a worker retry of
// that handler (timeout/error, up to DeadLetterAttempts times) calls it
// twice with identical arguments. ON CONFLICT DO NOTHING relies on the
// partial unique index idx_scheduled_recurring_dedup on
// (world_id, event_type, due_tick, payload) WHERE processed_at IS NULL — a
// partial index requires the conflict target AND its WHERE clause to be
// repeated here, or Postgres won't match it. payload is part of both: two
// legitimate concurrent rows (e.g. two active BattleTicks) differ only in
// payload and must both survive.
func (s *Scheduler) EnqueueTickRecurring(ctx context.Context, worldID uuid.UUID, eventType ScheduledEventType, payload any, lastDue, intervalTicks int) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal scheduled payload: %w", err)
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO scheduled_events (world_id, event_type, payload, process_after, due_tick)
		 VALUES ($1, $2, $3, now(),
		         GREATEST($4::int, (SELECT current_tick FROM worlds WHERE id = $1) + $5::int))
		 ON CONFLICT (world_id, event_type, due_tick, payload) WHERE processed_at IS NULL DO NOTHING`,
		worldID, string(eventType), raw, lastDue+intervalTicks, intervalTicks,
	)
	return err
}

// EnqueueTickTx schedules a tick-gated event within an existing transaction.
func (s *Scheduler) EnqueueTickTx(ctx context.Context, tx pgx.Tx, worldID uuid.UUID, eventType ScheduledEventType, payload any, dueTick int) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal scheduled payload: %w", err)
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO scheduled_events (world_id, event_type, payload, process_after, due_tick)
		 VALUES ($1, $2, $3, now(), $4)`,
		worldID, string(eventType), raw, dueTick,
	)
	return err
}

// Handler processes a single scheduled event. It must propagate ctx to all DB calls.
type Handler func(ctx context.Context, e ScheduledEvent) error

// DefaultHandlerTimeout is applied to every handler unless overridden via WithHandlerTimeout.
const DefaultHandlerTimeout = 5 * time.Second

// DeadLetterAttempts is the number of consecutive failures before an event is dead-lettered.
const DeadLetterAttempts = 3

// Worker polls for due events every 10 seconds and dispatches them to registered handlers.
// Each handler is called with a per-handler timeout context (G2). Three consecutive
// timeouts or errors mark the event as dead-lettered for manual inspection.
type Worker struct {
	pool            *pgxpool.Pool
	clock           clock.Clock
	handlers        map[ScheduledEventType]Handler
	timeouts        map[ScheduledEventType]time.Duration
	deadLetterHooks map[ScheduledEventType]Handler
	pollInterval    time.Duration
}

// defaultWorkerPoll is the batch poll cadence at the production tick rate. On a
// sub-minute dev cadence it is tightened via SetPollInterval so due events don't
// wait longer than a tick.
const defaultWorkerPoll = 10 * time.Second

// NewWorker creates a Worker. clk must be the same Clock used by Scheduler.
func NewWorker(pool *pgxpool.Pool, clk clock.Clock) *Worker {
	return &Worker{
		pool:            pool,
		clock:           clk,
		handlers:        make(map[ScheduledEventType]Handler),
		timeouts:        make(map[ScheduledEventType]time.Duration),
		deadLetterHooks: make(map[ScheduledEventType]Handler),
		pollInterval:    defaultWorkerPoll,
	}
}

// SetPollInterval tightens (or relaxes) the batch poll cadence. main.go scales it
// to the tick rate — a 10 s poll leaves events up to 10 s (>1 tick) late on the
// sub-minute TICK_SECONDS=6 dev cadence. Kept out of internal/events itself
// because reading the cadence means importing internal/tick, which imports
// internal/events (G1 cycle) — the caller that owns both wires it. No-op for
// non-positive d.
func (w *Worker) SetPollInterval(d time.Duration) {
	if d > 0 {
		w.pollInterval = d
	}
}

// Register binds a handler to a scheduled event type using the default timeout.
func (w *Worker) Register(t ScheduledEventType, h Handler) {
	w.handlers[t] = h
}

// RegisterWithTimeout binds a handler and overrides the per-call context timeout.
func (w *Worker) RegisterWithTimeout(t ScheduledEventType, h Handler, timeout time.Duration) {
	w.handlers[t] = h
	w.timeouts[t] = timeout
}

// RegisterDeadLetterHook binds a best-effort callback invoked when an event of
// type t is about to be dead-lettered (DeadLetterAttempts consecutive
// failures). P3 soak fix (2026-07-19): before this hook existed, a scheduled
// event that kept failing under DB/load pressure (e.g. a march's
// ScheduledUnitArrival or ScheduledOrderDelivery) was marked failed_at with
// only a server-side ERROR log — the affected Wanax was never told. From the
// player's side that reads as "tyst framgång": the CLI's original 202 said the
// unit was departing/a courier was dispatched, and nothing ever contradicted
// it, so they believe the order went through while the unit never actually
// moves. The hook receives the SAME event the handler failed on (including its
// payload) so it can look up the owner and emit a player-facing notification.
// A hook error is logged but never blocks dead-lettering itself.
func (w *Worker) RegisterDeadLetterHook(t ScheduledEventType, h Handler) {
	w.deadLetterHooks[t] = h
}

// Run polls for due events until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	slog.Info("scheduled event worker started", "poll", w.pollInterval)
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("scheduled event worker stopped")
			return
		case <-ticker.C:
			if err := w.processBatch(ctx); err != nil {
				slog.Error("event worker batch failed", "err", err)
			}
		}
	}
}

func (w *Worker) processBatch(ctx context.Context) error {
	rows, err := w.pool.Query(ctx,
		`UPDATE scheduled_events
		 SET attempts = attempts + 1
		 WHERE id IN (
		     SELECT id FROM scheduled_events
		     WHERE processed_at IS NULL
		       AND failed_at IS NULL
		       AND world_id IN (SELECT id FROM worlds WHERE status = 'active')
		       AND due_tick IS NOT NULL
		       AND due_tick <= (SELECT current_tick FROM worlds w2 WHERE w2.id = scheduled_events.world_id)
		     ORDER BY due_tick, id
		     LIMIT 20
		     FOR UPDATE SKIP LOCKED
		 )
		 RETURNING id, world_id, event_type, payload, process_after, processed_at, failed_at, attempts, due_tick`,
	)
	if err != nil {
		return fmt.Errorf("claim events: %w", err)
	}
	defer rows.Close()

	var batch []ScheduledEvent
	for rows.Next() {
		var e ScheduledEvent
		if err := rows.Scan(&e.ID, &e.WorldID, &e.EventType, &e.Payload, &e.ProcessAfter, &e.ProcessedAt, &e.FailedAt, &e.Attempts, &e.DueTick); err != nil {
			return fmt.Errorf("scan scheduled event: %w", err)
		}
		batch = append(batch, e)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, e := range batch {
		w.dispatch(ctx, e)
	}
	return nil
}

func (w *Worker) dispatch(ctx context.Context, e ScheduledEvent) {
	h, ok := w.handlers[e.EventType]
	if !ok {
		slog.Warn("no handler for scheduled event type", "type", e.EventType)
		w.markFailed(ctx, e.ID)
		return
	}

	timeout := DefaultHandlerTimeout
	if t, ok := w.timeouts[e.EventType]; ok {
		timeout = t
	}

	hCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := h(hCtx, e); err != nil {
		slog.Error("scheduled event handler failed",
			"type", e.EventType, "id", e.ID, "attempt", e.Attempts, "err", err)
		if e.Attempts >= DeadLetterAttempts {
			slog.Error("dead-lettering event after repeated failures",
				"type", e.EventType, "id", e.ID)
			if hook, ok := w.deadLetterHooks[e.EventType]; ok {
				// Best-effort, generous timeout of its own — this is the last chance
				// to tell the player anything, so give it room independent of the
				// handler's own (possibly just-expired) timeout budget.
				hookCtx, hookCancel := context.WithTimeout(ctx, DefaultHandlerTimeout)
				if hookErr := hook(hookCtx, e); hookErr != nil {
					slog.Error("dead-letter notification hook failed",
						"type", e.EventType, "id", e.ID, "err", hookErr)
				}
				hookCancel()
			}
			w.markFailed(ctx, e.ID)
		}
		return
	}
	w.markDone(ctx, e.ID)
}

func (w *Worker) markDone(ctx context.Context, id int64) {
	if _, err := w.pool.Exec(ctx,
		`UPDATE scheduled_events SET processed_at = now() WHERE id = $1`, id,
	); err != nil {
		slog.Error("mark event done failed", "id", id, "err", err)
	}
}

func (w *Worker) markFailed(ctx context.Context, id int64) {
	if _, err := w.pool.Exec(ctx,
		`UPDATE scheduled_events SET failed_at = now() WHERE id = $1`, id,
	); err != nil {
		slog.Error("mark event failed", "id", id, "err", err)
	}
}
