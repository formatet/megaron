package combat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/gossip"
	"formatet/megaron/server/internal/loyalty"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UnitArrivalHandler processes ScheduledUnitArrival events.
//
// When a marching unit arrives at its destination it either:
//   - Joins garrison (empty/own/allied hex)
//   - Triggers deterministic combat (enemy settlement present)
//
// Idempotency: the arriving unit is fetched with FOR UPDATE and the handler
// exits early if status != 'marching'. ON CONFLICT DO NOTHING is used for
// projection inserts. Re-running the handler is therefore safe.
//
// C5 stance effects (implemented):
//   - fortify: defending garrison units in fortify stance get ×1.5 strength.
//   - storm:   arriving unit with storm stance halves the wall-level bonus of the target.
//
// TODO C5-sentry: sentry interception is NOT yet active.
// Design: a periodic scan (e.g. ScheduledSentryScan every ~30 s, or a ticker
// in main.go mirroring kharis seedDailyTicks) iterates all marching units,
// computes their interpolated hex from departs_at/arrives_at on a straight line,
// and for each active sentry unit (status='positioned', stance='sentry') within
// 3 hex of sentry_q/r triggers an UnitIntercepted combat using resolveCombat.
// Guard: intercepted_at column on units (or a separate table) prevents the same
// march from being intercepted twice by the same sentry. Stub wired to
// SetStance (stance can be set to 'sentry', sentry_q/r is persisted), but no
// scan goroutine is started yet.
type UnitArrivalHandler struct {
	expeditionOutcomes *[]expeditionOutcome
	pool               *pgxpool.Pool
	eventStore         *events.Store
	hub                Broadcaster
	scheduler          *events.Scheduler
	clk                clock.Clock
	sitosCfg           economy.SitosConfig
	// Dice is the KR3 battle-seed source (megaron_plan_kr3_stridssystem.md §3):
	// initiateOrJoinBattle draws battles.seed from it exactly once, at battle
	// creation. Exported so tests can override for a deterministic seed —
	// same pattern as economy.DeliveryHandler.Dice. Defaults to
	// economy.NewWallDice() (production behaviour); nil-safe (startBattle
	// falls back to NewWallDice() if left nil).
	Dice economy.Dice
}

// NewUnitArrivalHandler creates a UnitArrivalHandler.
func NewUnitArrivalHandler(pool *pgxpool.Pool, store *events.Store, hub Broadcaster, scheduler *events.Scheduler, clk clock.Clock, sitosCfg economy.SitosConfig) *UnitArrivalHandler {
	return &UnitArrivalHandler{pool: pool, eventStore: store, hub: hub, scheduler: scheduler, clk: clk, sitosCfg: sitosCfg, Dice: economy.NewWallDice()}
}

// Handle processes one ScheduledUnitArrival scheduled event.
func (h *UnitArrivalHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var payload unit.ScheduledUnitArrivalPayload
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal unit arrival payload: %w", err)
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if payload.ArriveTick != nil {
		// A superseded arrival: recall/redirect (and a fresh march after the
		// old one ended) queue a new arrival without cancelling this one. Only
		// the arrival whose tick matches the unit's current march may resolve
		// it — otherwise a recalled unit lands at its new target the instant
		// its old march would have. Locks the row like resolve() does.
		var cur *int
		if err := tx.QueryRow(ctx,
			`SELECT arrive_tick FROM units WHERE id = $1 FOR UPDATE`, payload.UnitID,
		).Scan(&cur); err != nil {
			return fmt.Errorf("load arriving unit's arrive_tick: %w", err)
		}
		if cur == nil || *cur != *payload.ArriveTick {
			return nil
		}
	}

	// Per-invocation buffer: workers may call the shared handler concurrently.
	var outcomes []expeditionOutcome
	local := *h
	local.expeditionOutcomes = &outcomes
	if err := local.resolve(ctx, tx, payload.UnitID, payload.WorldID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	for _, outcome := range outcomes {
		h.eventStore.RecordCommitted(ctx, outcome.event)
		if delivery, ok := h.hub.(CommittedNotificationDelivery); ok {
			delivery.DeliverCommittedNotification(ctx, payload.WorldID, outcome.owner, outcome.id, outcome.kind, outcome.level, outcome.body)
		}
	}
	return nil
}

// NotifyDeadLetter is the events.Worker dead-letter hook for ScheduledUnitArrival
// (P3 soak fix, 2026-07-19): registered in main.go so that if an arrival keeps
// failing under load until it is dead-lettered, the owner is told instead of
// the march simply vanishing with no signal ("tyst framgång" — the dispatch's
// 202 promised an arrival that never came, and nothing ever said otherwise).
// Best-effort: a lookup failure here only means the player misses this one
// notification, never a second failure mode on top of the dead-letter itself.
func (h *UnitArrivalHandler) NotifyDeadLetter(ctx context.Context, e events.ScheduledEvent) error {
	var payload unit.ScheduledUnitArrivalPayload
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		return fmt.Errorf("dead-letter: unmarshal unit arrival payload: %w", err)
	}
	if h.hub == nil {
		return nil
	}
	var ownerID uuid.UUID
	var status, utype string
	var q, r int
	if err := h.pool.QueryRow(ctx,
		`SELECT owner_id, status, type, q, r FROM units WHERE id = $1`, payload.UnitID,
	).Scan(&ownerID, &status, &utype, &q, &r); err != nil {
		return fmt.Errorf("dead-letter: load unit owner: %w", err)
	}
	// Namnger subjektet (megaron_plan_dispatches.md §4) — "your infantry's
	// arrival..." läses som en kategori; med namnet blir det "2nd Spearmen of
	// Knossos's arrival...", vilket Wanaxen faktiskt känner igen.
	name := unit.LoadDisplayName(ctx, h.pool, payload.UnitID)
	subject := "your " + utype
	if name != "" {
		subject = name
	}
	_ = h.hub.NotifyPlayer(ctx, payload.WorldID, ownerID, "MarchStalled", 1, map[string]any{
		"unit_id": payload.UnitID,
		"name":    name,
		"q":       q,
		"r":       r,
		"reason": fmt.Sprintf(
			"%s's arrival could not be processed after repeated attempts (system fault) — check `unit list`/`map`; its status is currently %q and may need a fresh march order",
			subject, status),
	})
	slog.Warn("dead-letter: notified owner of stalled unit arrival", "unit", payload.UnitID, "owner", ownerID)
	return nil
}

func (h *UnitArrivalHandler) resolve(ctx context.Context, tx pgx.Tx, unitID, worldID uuid.UUID) error {
	// Load arriving unit with FOR UPDATE — idempotency guard.
	//
	// q/r are scanned as NULLABLE and only moved into unitRow once the unit is
	// known to still be marching. A unit that has STOPPED marching has NULL
	// coordinates by design — garrisoning nulls them (found_metropolis.go), and
	// so does the host dissolving into the metropolis it founds. Scanning them
	// straight into unitRow's plain ints made the load fail on exactly those
	// rows, three lines above the "already resolved" check written to handle
	// them: the handler errored, the worker retried, and after three attempts
	// the dead-letter told the player their march had suffered a "system fault"
	// (G2). Live 2026-09-03, Polyidos: host ordered to march at tick 1, founded
	// its metropolis at tick 2 — nothing cancels a queued arrival — and the
	// arrival fired at tick 3 against a disbanded row. The founding had
	// succeeded; only the notification claimed otherwise.
	var u unitRow
	var curQ, curR *int
	if err := tx.QueryRow(ctx,
		`SELECT id, owner_id, type, category, size, crew, cargo_unit_id,
		        status, q, r, target_q, target_r, stance, march_intent, colony_name, home_settlement_id, capture_mode,
		        carried_silver, provisions, land_target_q, land_target_r, land_cargo_intent, passage_messenger_id,
		        pickup_unit_id, pickup_wait_ticks
		 FROM units WHERE id = $1 FOR UPDATE`,
		unitID,
	).Scan(&u.id, &u.ownerID, &u.utype, &u.category, &u.size, &u.crew, &u.cargoUnitID,
		&u.status, &curQ, &curR, &u.targetQ, &u.targetR, &u.stance, &u.marchIntent, &u.colonyName, &u.homeSettlementID, &u.captureMode,
		&u.carriedSilver, &u.provisions, &u.landTargetQ, &u.landTargetR, &u.landCargoIntent, &u.passageMessengerID,
		&u.pickupUnitID, &u.pickupWaitTicks); err != nil {
		return fmt.Errorf("load arriving unit: %w", err)
	}

	// Idempotent: already resolved. MUST stay above the coordinate requirement
	// below — see the comment on the scan.
	if u.status != "marching" {
		return nil
	}
	if curQ == nil || curR == nil {
		return fmt.Errorf("unit %s is marching but has no current position", unitID)
	}
	u.q, u.r = *curQ, *curR
	if u.targetQ == nil || u.targetR == nil {
		return fmt.Errorf("unit %s has no target coordinates", unitID)
	}

	destQ, destR := *u.targetQ, *u.targetR

	// Sweep FOW along the actual path walked by this unit. Best-effort: log on
	// error, never abort the arrival. Runs for every arrival (garrison, combat,
	// colonise) so isolated spawns always reveal the terrain they traversed.
	if path, _, pathOK, pathErr := province.FindPath(ctx, tx, worldID,
		province.MapPosition{Q: u.q, R: u.r},
		province.MapPosition{Q: destQ, R: destR},
		u.category,
	); pathErr != nil {
		slog.Warn("pathfind error during arrival FOW sweep", "unit", unitID, "err", pathErr)
	} else if pathOK {
		for _, tile := range path {
			if _, insErr := tx.Exec(ctx,
				`INSERT INTO player_scouted_tiles (world_id, player_id, q, r)
				 VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`,
				worldID, u.ownerID, tile.Q, tile.R,
			); insErr != nil {
				slog.Warn("FOW sweep insert failed", "unit", unitID, "q", tile.Q, "r", tile.R, "err", insErr)
			}
		}
	}

	// Explore intent: the unit reaches its target and immediately turns for
	// home — it never garrisons or fights there (temenos_todo.md "Explore-order
	// auto-retur"). The FOW sweep above already revealed and permanently
	// recorded the route (including this hex), so the only remaining step is
	// to dispatch the return leg.
	if u.marchIntent != nil && *u.marchIntent == "explore" {
		return h.exploreArrived(ctx, tx, u, destQ, destR, worldID)
	}

	// Explore-return intent: the unit is back at its home settlement's
	// departure hex. Re-garrison it directly via the known home_settlement_id —
	// bypassing the normal hex→settlement lookup below, which fails for a
	// naval unit resting at the sea hex adjacent to a coastal settlement (that
	// hex has no settlement row of its own).
	if u.marchIntent != nil && *u.marchIntent == "explore_return" {
		return h.exploreReturned(ctx, tx, u, destQ, destR, worldID)
	}

	// Patrol intent: a ship reaches its patrol hex and HOLDS there (positioned +
	// sentry stance) rather than turning home immediately like explore — a patrol
	// timer (ScheduledSentryReturn) turns it home later. The return leg itself
	// reuses the explore_return machinery. "sentry" is the pre-rename value
	// (megaron_plan_cli_sanning, "sentry" collided with the land stance of the
	// same name) — still matched here so a unit already in flight when this
	// deployed, with march_intent written under the old name, still lands.
	if u.marchIntent != nil && (*u.marchIntent == "patrol" || *u.marchIntent == "sentry") {
		return h.sentryArrived(ctx, tx, u, destQ, destR, worldID)
	}

	// Damaged-return intent (megaron_plan_skeppsreparation.md Slice B point 3):
	// a routed ship's hull-drawn home march, dispatched by
	// combat.BattleTickHandler.sendDamagedShipHome (ship_hull.go) at battle end.
	// Same "bypass the hex→settlement lookup" reason as explore_return above —
	// the target is the sea hex adjacent to home, which has no settlement row.
	// "captured_return" (megaron_plan_sjohandel_kraver_skepp.md R5) is the
	// SAME march, dispatched by combat.NavalSeizureOutcomeHandler for a ship
	// that just changed hands in a naval seizure — it re-garrisons exactly
	// the same way at the (now new-owner's) home_settlement_id.
	if u.marchIntent != nil && (*u.marchIntent == "damaged_return" || *u.marchIntent == "captured_return") {
		return h.damagedShipReturned(ctx, tx, u, destQ, destR, worldID)
	}

	// Assault intent: a laden galley has reached the sea hex next to an enemy
	// coastal settlement. The ship cannot enter land, so its cargo storms the
	// beach — the landing is resolved with the cargo's strength, not the ship's.
	if u.marchIntent != nil && *u.marchIntent == "assault" {
		return h.resolveAmphibiousAssault(ctx, tx, u, destQ, destR, worldID)
	}

	// Land mission (R1, megaron_plan_skeppsuppdrag_landsatt.md): the ship has
	// reached the sea hex next to its chosen land target. Land the cargo there
	// (or on the nearest still-unclaimed neighbour, if the world changed
	// mid-transit) and turn the ship for home — never garrison/fight at the
	// sea waypoint itself.
	if u.marchIntent != nil && *u.marchIntent == "land" {
		return h.landArrived(ctx, tx, u, destQ, destR, worldID)
	}

	// Passage mission (R2/R3, megaron_plan_ordna_passage.md 3b-3): the ship has
	// reached the sea hex next to the disembark hex arranged for its runner.
	// No cargo to land — only the wait-or-go-home decision (passageArrived).
	if u.marchIntent != nil && *u.marchIntent == "passage" {
		return h.passageArrived(ctx, tx, u, destQ, destR, worldID)
	}

	// Pickup mission (R3, megaron_plan_hamta_hem.md, slice 2b): the ship has
	// reached the sea hex next to the shore chosen for the unit it was sent
	// to fetch.
	if u.marchIntent != nil && *u.marchIntent == "pickup" {
		return h.pickupArrived(ctx, tx, u, destQ, destR, worldID)
	}

	// Pickup return leg (R6): the ship — with or without the fetched unit
	// aboard — is back at its home settlement's departure hex.
	if u.marchIntent != nil && *u.marchIntent == "pickup_return" {
		return h.pickupReturned(ctx, tx, u, destQ, destR, worldID)
	}

	// Find settlement at destination (if any). The JOIN condition's karens
	// exclusion mirrors march_start.go's pre-flight (megaron_plan_erovring.md
	// S5): a burned settlement past its recolonizable_after_tick is treated
	// as absent here too, so a colonize intent that slipped past the
	// pre-flight (or was dispatched before the karens elapsed) sees the hex
	// as empty at arrival, same as the pre-flight now does.
	var dest destSettlement
	err := tx.QueryRow(ctx,
		`SELECT s.id, s.owner_id, s.wall_level, p.id, p.terrain_type
		 FROM provinces p
		 LEFT JOIN settlements s ON s.province_id = p.id
		   AND NOT (s.state = 'razed' AND s.recolonizable_after_tick IS NOT NULL
		            AND s.recolonizable_after_tick <= current_world_tick())
		 WHERE p.world_id = $1 AND p.map_q = $2 AND p.map_r = $3`,
		worldID, destQ, destR,
	).Scan(&dest.settlementID, &dest.ownerID, &dest.wallLevel,
		&dest.provinceID, &dest.terrain)

	hasSettlement := err == nil && dest.settlementID != nil

	// Colonize intent on an empty hex → found a colony (the unit disbands into
	// its founding populace — colonists become citizens, not a garrison).
	// If the hex turned out to be settled (race), fall through to the normal paths.
	// Authoritative settlement-cap check (dispatch enforces it too, but the count can
	// change mid-transit): over the cap → the unit just garrisons the empty hex instead.
	if u.marchIntent != nil && *u.marchIntent == "colonize" && !hasSettlement {
		var owned int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM settlements WHERE world_id = $1 AND owner_id = $2 AND state = 'active'`,
			worldID, u.ownerID,
		).Scan(&owned); err == nil && owned >= province.MaxSettlementsPerWanax {
			slog.Info("colonize blocked at arrival: settlement cap reached", "owner", u.ownerID, "owned", owned)
			return h.arriveGarrison(ctx, tx, u, destQ, destR, dest.settlementID, worldID)
		}
		// Authoritative catchment-overlap check (dispatch's march_start.go
		// pre-flight enforces it too, but the world can change mid-transit —
		// another colonist could found first): delad-catchment-grind invariant
		// (Timothy 2026-07-27/28), gated for every owner alike. Same fallback
		// shape as the settlement-cap check above — the unit just garrisons the
		// empty hex instead of founding on top of a neighbour's fields.
		if conflict, cErr := province.SettlementCatchmentOverlap(ctx, tx, worldID, destQ, destR); cErr == nil && conflict != nil {
			slog.Info("colonize blocked at arrival: catchment overlap",
				"owner", u.ownerID, "conflict_settlement", conflict.SettlementID, "q", destQ, "r", destR)
			return h.arriveGarrison(ctx, tx, u, destQ, destR, dest.settlementID, worldID)
		} else if cErr != nil {
			slog.Error("colonize: catchment overlap check failed", "err", cErr, "q", destQ, "r", destR)
		}
		return h.foundColony(ctx, tx, u, dest.provinceID, destQ, destR, worldID)
	}

	// No settlement or uncontested → become garrison — UNLESS a hostile unit
	// already holds this hex without a settlement (P2 fix, 2026-07-18 soak:
	// "Dole mot Eastern Outpost"). Before this fix, marching onto a hex where
	// only an enemy's field-positioned unit sat (no settlements row there —
	// province.owner_id-style outposts were never establishable by any code
	// path — the model was dropped outright in migration 138, 2026-09-02 —
	// so the soak-observed "Outpost" was in fact a hostile unit
	// sitting status='positioned' on an empty hex) fell straight through to
	// arriveGarrison: the arriving unit simply co-located with the enemy — no
	// combat, no capture, no notification. Settlements are still resolved via
	// resolveCombat below; this only covers the settlement-less hex.
	if !hasSettlement {
		defenders, ferr := loadFieldDefenders(ctx, tx, worldID, destQ, destR, u.ownerID)
		if ferr != nil {
			slog.Warn("resolve: load field defenders failed, proceeding as peaceful arrival", "unit", unitID, "err", ferr)
		} else if len(defenders) > 0 {
			return h.resolveFieldCombat(ctx, tx, u, defenders, destQ, destR, worldID)
		}
	}
	if !hasSettlement || dest.ownerID == nil || *dest.ownerID == u.ownerID {
		// R4 (megaron_plan_skeppsuppdrag_landsatt.md): a naval unit's plain
		// march (no intent) was validated at dispatch to end next to a
		// friendly settlement — re-resolve that authoritatively here (the
		// world can change mid-transit), AFTER the field-defenders check
		// above, so a contested hex still fights first — so the ship actually
		// docks instead of sitting "positioned" at the sea hex forever, which
		// is exactly the R3/R6 problem this slice closes. If the settlement
		// is gone by now, fall through to the ordinary peaceful arrival below
		// (the ship ends up positioned, same as any other unclaimed-hex
		// arrival).
		if !hasSettlement && u.category == "naval" && (u.marchIntent == nil || *u.marchIntent == "") {
			if sid, found, fErr := friendlySettlementAdjacent(ctx, tx, worldID, u.ownerID, destQ, destR); fErr == nil && found {
				return h.arriveGarrison(ctx, tx, u, destQ, destR, &sid, worldID)
			}
		}
		return h.arriveGarrison(ctx, tx, u, destQ, destR, dest.settlementID, worldID)
	}

	// Enemy settlement present — fight!
	return h.resolveCombat(ctx, tx, u, dest, destQ, destR, worldID)
}

// arriveGarrison places the unit at the destination as a garrison unit.
// If the unit is a naval vessel with cargo, the cargo land unit's position is
// updated to match the ship's destination (C6: cargo follows the ship).
func (h *UnitArrivalHandler) arriveGarrison(
	ctx context.Context, tx pgx.Tx,
	u unitRow, destQ, destR int, settlementID *uuid.UUID, worldID uuid.UUID,
) error {
	newStatus := "garrison"
	if settlementID == nil {
		newStatus = "positioned" // unit on the map without a settlement
	}

	if _, err := tx.Exec(ctx,
		`UPDATE units SET
		   status        = $2,
		   q             = $3,
		   r             = $4,
		   settlement_id = $5,
		   target_q      = NULL,
		   target_r      = NULL,
		   departs_at    = NULL,
		   arrives_at    = NULL,
		   depart_tick   = NULL,
		   arrive_tick   = NULL,
		   march_intent  = NULL,
		   -- A sentry's hold centre is where it stops (the hex SetStance
		   -- would read for a unit already standing here). Without this a
		   -- unit that marched with — or was sent by Runner — stance sentry
		   -- arrived with a NULL (or stale) centre and never intercepted:
		   -- every interception query needs sentry_q/r.
		   sentry_q      = CASE WHEN stance = 'sentry' THEN $3 ELSE sentry_q END,
		   sentry_r      = CASE WHEN stance = 'sentry' THEN $4 ELSE sentry_r END,
		   updated_at    = now()
		 WHERE id = $1`,
		u.id, newStatus, destQ, destR, settlementID,
	); err != nil {
		return fmt.Errorf("unit arrive garrison: %w", err)
	}

	// An expedition that walks into a settlement hands over whatever purse it is
	// still carrying (mig 107). Recall, a rerouted march, a colonisation refused
	// on arrival — all of them funnel through here, so the silver comes home by
	// one path instead of one per way of turning around. Without this a recalled
	// colonist would leave its city permanently poorer for a colony never
	// founded.
	//
	// A unit destroyed in the field keeps its purse and it goes with the column.
	// That is deliberate: the silver was physically on the road, and the road is
	// interceptable like everything else on the map.
	if settlementID != nil && u.carriedSilver > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE settlement_goods
			    SET amount = LEAST(cap, settled(amount, rate, calc_tick) + $1),
			        calc_tick = current_world_tick()
			  WHERE settlement_id = $2 AND good_key = 'silver'`,
			u.carriedSilver, *settlementID,
		); err != nil {
			return fmt.Errorf("unit arrive: return carried purse: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE units SET carried_silver = 0 WHERE id = $1`, u.id,
		); err != nil {
			return fmt.Errorf("unit arrive: clear carried purse: %w", err)
		}
	}

	// A ship that reaches port unloads whatever provisions the voyage did not eat
	// (megaron_plan_skeppsproviant.md §5). Same shape as the purse above, and for
	// the same reason: the food was physically aboard, and it comes back.
	//
	// Load-bearing, not tidiness. Provisioning deliberately over-estimates the
	// return leg (VoyageProvisions assumes it is symmetric with the outbound one),
	// so without this every voyage would burn a full round trip's rations however
	// short it actually was, and the safety margin would quietly become a tax on
	// sailing at all.
	if settlementID != nil && u.provisions > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE settlement_goods
			    SET amount = LEAST(cap, settled(amount, rate, calc_tick) + $1),
			        calc_tick = current_world_tick()
			  WHERE settlement_id = $2 AND good_key = 'grain'`,
			u.provisions, *settlementID,
		); err != nil {
			return fmt.Errorf("unit arrive: unload provisions: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE units SET provisions = 0 WHERE id = $1`, u.id,
		); err != nil {
			return fmt.Errorf("unit arrive: clear provisions: %w", err)
		}
	}

	// C6: if this ship carried a land unit, move the cargo to the ship's new position.
	// The cargo stays 'embarked' — the Wanax must explicitly /unload to deploy it.
	if u.cargoUnitID != nil {
		if _, err := tx.Exec(ctx,
			`UPDATE units SET
			   q             = $2,
			   r             = $3,
			   settlement_id = $4,
			   updated_at    = now()
			 WHERE id = $1 AND status = 'embarked'`,
			*u.cargoUnitID, destQ, destR, settlementID,
		); err != nil {
			// Non-fatal: log and continue (cargo ghost is bad but arrival must not fail).
			slog.Warn("C6: could not update cargo unit position on ship arrival",
				"ship", u.id, "cargo", *u.cargoUnitID, "err", err)
		} else {
			slog.Info("C6: cargo unit position updated with ship", "ship", u.id, "cargo", *u.cargoUnitID, "q", destQ, "r", destR)
		}
	}

	_, _ = h.eventStore.Append(ctx, u.id, events.StreamType(unit.StreamUnit), unit.EventUnitArrived,
		unit.UnitArrivedPayload{
			UnitID:    u.id,
			Q:         destQ,
			R:         destR,
			NewStatus: newStatus,
		}, worldID, nil)

	// R4 (megaron_plan_hamta_hem.md, slice 2b): a fetched land unit reaching
	// the shore AFTER its pickup ship already got there and started waiting —
	// board it right here, in the same transaction, regardless of WHY it
	// arrived (the bud's own march order, or one the Wanax gave directly).
	// Field combat never reaches this function (resolveFieldCombat is checked
	// in resolve() before arriveGarrison is ever called), so a unit that
	// lands in a fight never boards.
	if newStatus == "positioned" && u.category == "land" {
		if ship, found, sErr := h.pickupWaitingShipFor(ctx, tx, worldID, u.ownerID, u.id, destQ, destR); sErr != nil {
			slog.Warn("pickup: check for waiting ship failed", "unit", u.id, "err", sErr)
		} else if found {
			boarded, bErr := h.boardPickupUnit(ctx, tx, ship.id, u.id, u.ownerID, destQ, destR, ship.q, ship.r, worldID)
			if bErr != nil {
				slog.Warn("pickup: board fetched unit failed", "unit", u.id, "ship", ship.id, "err", bErr)
			} else if boarded {
				ship.cargoUnitID = &u.id
				if rErr := h.dispatchReturnHome(ctx, tx, ship, ship.q, ship.r, worldID, returnReasonPickup); rErr != nil {
					return fmt.Errorf("arriveGarrison: pickup ship return home: %w", rErr)
				}
			}
		}
	}

	// Fas 2h: this was the one arrival path with no player-facing notification —
	// ColonyFounded/ArmyArrival/OutpostEstablished already notify, but a plain
	// peaceful march (no combat, no colonize) only ever wrote the audit event
	// above, so a Wanax had to poll `unit list`/`map` to learn a march landed.
	if h.hub != nil {
		_ = h.hub.NotifyPlayer(ctx, worldID, u.ownerID, "UnitArrived", 4, map[string]any{
			"unit_id": u.id,
			"type":    u.utype,
			"name":    unit.LoadDisplayName(ctx, tx, u.id),
			"q":       destQ,
			"r":       destR,
			"status":  newStatus,
		})
	}

	slog.Info("unit arrived (garrison)", "unit", u.id, "q", destQ, "r", destR, "status", newStatus)
	return nil
}

// landArrived handles a "land" mission ship reaching the sea hex next to its
// chosen land target (R1, megaron_plan_skeppsuppdrag_landsatt.md). It puts
// the cargo ashore, optionally founds a colony there, and always turns the
// ship for home (dispatchReturnHome, reusing the explore_return machinery) —
// a land mission never ends with the ship standing at sea.
func (h *UnitArrivalHandler) landArrived(
	ctx context.Context, tx pgx.Tx,
	u unitRow, destQ, destR int, worldID uuid.UUID,
) error {
	if u.cargoUnitID == nil || u.landTargetQ == nil || u.landTargetR == nil || u.homeSettlementID == nil {
		// Defensive: dispatch always sets these three together for a land
		// mission — never strand the ship over a data inconsistency.
		slog.Warn("land arrival: missing cargo/land target/home settlement, garrisoning ship in place instead", "unit", u.id)
		return h.arriveGarrison(ctx, tx, u, destQ, destR, nil, worldID)
	}
	landQ, landR := *u.landTargetQ, *u.landTargetR
	cargoID := *u.cargoUnitID

	// Re-resolve the landing hex authoritatively — the world can change
	// mid-transit. Still free → land exactly where chosen. Taken by now → the
	// nearest still-unclaimed neighbour (the retired Unload fall (b)'s own
	// fallback). Nowhere left at all → keep the cargo aboard and sail home
	// with it; the Wanax is told why.
	var settled bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM provinces p JOIN settlements s ON s.province_id = p.id
		              WHERE p.world_id = $1 AND p.map_q = $2 AND p.map_r = $3 AND s.state = 'active')`,
		worldID, landQ, landR,
	).Scan(&settled); err != nil {
		slog.Warn("land arrival: could not check landing hex, garrisoning ship instead", "unit", u.id, "err", err)
		return h.arriveGarrison(ctx, tx, u, destQ, destR, nil, worldID)
	}
	if settled {
		if nq, nr, found, nErr := province.NearestUnclaimedLandNeighbor(ctx, tx, worldID, destQ, destR); nErr == nil && found {
			landQ, landR = nq, nr
		} else {
			if h.hub != nil {
				_ = h.hub.NotifyPlayer(ctx, worldID, u.ownerID, "UnitArrived", 2, map[string]any{
					"unit_id": u.id,
					"name":    unit.LoadDisplayName(ctx, tx, u.id),
					"note": fmt.Sprintf(
						"the chosen landing site at (%d,%d) is no longer open ground, and nowhere unclaimed remains nearby — the cargo stays aboard and the ship turns for home",
						*u.landTargetQ, *u.landTargetR),
				})
			}
			return h.dispatchReturnHome(ctx, tx, u, destQ, destR, worldID, returnReasonExplore)
		}
	}

	// Disembark: the cargo becomes a field-positioned unit of its own — the
	// same outcome the retired Unload fall (b) used to produce.
	if _, err := tx.Exec(ctx,
		`UPDATE units SET status = 'positioned', settlement_id = NULL, q = $2, r = $3, updated_at = now()
		 WHERE id = $1`,
		cargoID, landQ, landR,
	); err != nil {
		return fmt.Errorf("land arrival: disembark cargo: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE units SET cargo_unit_id = NULL WHERE id = $1`, u.id); err != nil {
		return fmt.Errorf("land arrival: clear ship cargo: %w", err)
	}
	_, _ = h.eventStore.Append(ctx, u.id, events.StreamType(unit.StreamUnit), unit.EventShipUnloaded,
		unit.ShipUnloadedPayload{ShipUnitID: u.id, CargoUnitID: cargoID, Q: landQ, R: landR}, worldID, nil)

	// Grounding order: found a colony right there, no further order needed.
	// Re-runs the SAME authoritative checks the plain-colonize arrival branch
	// below runs (settlement cap, catchment overlap) — dispatch already ran
	// them once (colonizeDispatchPreflight), but the world can change
	// mid-transit. On failure the cargo still lands (above) — only the
	// colonize half is refused, with an honest reason, matching arriveGarrison's
	// existing "co-locate, don't silently fail" posture for the plain case.
	if u.landCargoIntent != nil && *u.landCargoIntent == "colonize" {
		var owned int
		_ = tx.QueryRow(ctx,
			`SELECT count(*) FROM settlements WHERE world_id = $1 AND owner_id = $2 AND state = 'active'`,
			worldID, u.ownerID,
		).Scan(&owned)
		var conflict *province.CatchmentConflict
		if owned < province.MaxSettlementsPerWanax {
			conflict, _ = province.SettlementCatchmentOverlap(ctx, tx, worldID, landQ, landR)
		}
		switch {
		case owned >= province.MaxSettlementsPerWanax:
			h.notifyLandColonizeFailed(ctx, tx, u, cargoID, landQ, landR, worldID,
				"you already hold as many settlements as you may — the landing party garrisons the beach instead of founding a colony")
		case conflict != nil:
			h.notifyLandColonizeFailed(ctx, tx, u, cargoID, landQ, landR, worldID,
				"this ground is already farmed by a neighbouring settlement's catchment — the landing party garrisons the beach instead of founding a colony")
		default:
			// carried_silver is the colonist purse the port paid at dispatch
			// (march_start.go) — without it the colony would start at 0.
			var cargoSize int
			var cargoPurse float64
			_ = tx.QueryRow(ctx, `SELECT size, carried_silver FROM units WHERE id = $1`, cargoID).Scan(&cargoSize, &cargoPurse)
			cargoRow := unitRow{id: cargoID, ownerID: u.ownerID, colonyName: u.colonyName, size: cargoSize, carriedSilver: cargoPurse}
			if err := h.foundColony(ctx, tx, cargoRow, uuid.Nil, landQ, landR, worldID); err != nil {
				return fmt.Errorf("land arrival: found colony: %w", err)
			}
			return h.dispatchReturnHome(ctx, tx, u, destQ, destR, worldID, returnReasonExplore)
		}
	}

	if h.hub != nil {
		_ = h.hub.NotifyPlayer(ctx, worldID, u.ownerID, "UnitArrived", 4, map[string]any{
			"unit_id": cargoID,
			"name":    unit.LoadDisplayName(ctx, tx, cargoID),
			"q":       landQ,
			"r":       landR,
			"status":  "positioned",
		})
	}

	slog.Info("land mission: cargo landed, ship turning for home", "ship", u.id, "cargo", cargoID, "q", landQ, "r", landR)
	return h.dispatchReturnHome(ctx, tx, u, destQ, destR, worldID, returnReasonExplore)
}

// notifyLandColonizeFailed tells the Wanax why a "land" mission's
// cargo_intent=colonize could not found a colony at arrival — the cargo has
// already landed (positioned) by the time this is called, so this only
// explains why it is NOT a new city.
func (h *UnitArrivalHandler) notifyLandColonizeFailed(ctx context.Context, tx pgx.Tx, u unitRow, cargoID uuid.UUID, q, r int, worldID uuid.UUID, reason string) {
	slog.Info("land mission: colonize declined at arrival", "ship", u.id, "cargo", cargoID, "q", q, "r", r, "reason", reason)
	if h.hub == nil {
		return
	}
	_ = h.hub.NotifyPlayer(ctx, worldID, u.ownerID, "UnitArrived", 3, map[string]any{
		"unit_id": cargoID,
		"name":    unit.LoadDisplayName(ctx, tx, cargoID),
		"q":       q,
		"r":       r,
		"status":  "positioned",
		"note":    reason,
	})
}

// passageArrived handles a "passage" mission ship reaching the sea hex next
// to the disembark hex api/handlers.ArrangePassage chose for its runner (R2/
// R3, megaron_plan_ordna_passage.md 3b-3). Unlike a "land" mission, this ship
// carries no cargo — its whole job is the runner boardShipMissions already
// boarded (outbound) or is waiting to be fetched by (pickup), and this
// function only decides whether the ship should hold for that runner's
// homeward leg or turn straight for home:
//   - the runner still needs a physical ride for some leg of its own journey
//     (kind != "order" and status != "arrived" — covers both an outbound
//     runner not yet on its way home, AND a pickup runner already standing
//     "awaiting_passage" right here) — the ship holds (status='positioned',
//     march_intent='passage_wait') until PassageScanHandler's release phase
//     (R4) sends it home, boarding the runner in the very same scan.
//   - otherwise (an order — one-way, never comes back — or the runner is
//     already fully home/gone) — nothing to wait for; the ship turns for
//     home immediately, exactly like a "land" mission's empty return leg.
func (h *UnitArrivalHandler) passageArrived(
	ctx context.Context, tx pgx.Tx,
	u unitRow, destQ, destR int, worldID uuid.UUID,
) error {
	if u.homeSettlementID == nil {
		// Defensive: dispatch always sets this for a passage mission — never
		// strand the ship over a data inconsistency.
		slog.Warn("passage arrival: missing home settlement, garrisoning ship in place instead", "unit", u.id)
		return h.arriveGarrison(ctx, tx, u, destQ, destR, nil, worldID)
	}

	waiting := false
	if u.passageMessengerID != nil {
		var kind, status string
		if err := tx.QueryRow(ctx,
			`SELECT kind, status FROM messengers WHERE id = $1`, *u.passageMessengerID,
		).Scan(&kind, &status); err == nil {
			waiting = kind != "order" && status != "arrived"
		}
		// A scan error (row gone) leaves waiting=false — nothing left to wait for.
	}

	if !waiting {
		slog.Info("passage: nothing left to wait for, ship turning for home", "unit", u.id, "messenger", u.passageMessengerID)
		return h.dispatchReturnHome(ctx, tx, u, destQ, destR, worldID, returnReasonExplore)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE units SET
		   status       = 'positioned',
		   q            = $2,
		   r            = $3,
		   target_q     = NULL,
		   target_r     = NULL,
		   departs_at   = NULL,
		   arrives_at   = NULL,
		   depart_tick  = NULL,
		   arrive_tick  = NULL,
		   march_intent = 'passage_wait',
		   updated_at   = now()
		 WHERE id = $1`,
		u.id, destQ, destR,
	); err != nil {
		return fmt.Errorf("passageArrived: post passage_wait: %w", err)
	}

	if h.hub != nil {
		_ = h.hub.NotifyPlayer(ctx, worldID, u.ownerID, "UnitArrived", 2, map[string]any{
			"unit_id": u.id,
			"name":    unit.LoadDisplayName(ctx, tx, u.id),
			"q":       destQ,
			"r":       destR,
			"status":  "positioned",
			"note":    "waiting for the runner's return",
		})
	}

	slog.Info("passage: ship waiting for runner's return", "unit", u.id, "messenger", *u.passageMessengerID)
	return nil
}

// ReleasePassageWaitShip is messenger's thin, downward-only call into combat
// (G1: messenger may use combat, never the reverse) for R4's release phase
// (megaron_plan_ordna_passage.md 3b-3): messenger.PassageScanHandler decided
// a passage_wait ship's runner is either ready to travel home with it or no
// longer needs it, and calls this to actually turn the ship around — reusing
// dispatchReturnHome exactly like every other return-leg caller in this file,
// inside the SAME transaction and scan the caller is already running (so the
// ship's depart_tick = the current tick, and boardShipMissions — run right
// after this in the same PassageScanHandler.Handle — boards the runner in
// that very scan). A no-op (nil, nothing written) if the ship has already
// been resolved by a racing pass — same claim-then-guard idiom as every other
// handler in this file.
func (h *UnitArrivalHandler) ReleasePassageWaitShip(ctx context.Context, tx pgx.Tx, shipID, worldID uuid.UUID) error {
	var u unitRow
	var curQ, curR *int
	if err := tx.QueryRow(ctx,
		`SELECT id, owner_id, type, category, size, crew, cargo_unit_id,
		        status, q, r, target_q, target_r, stance, march_intent, colony_name, home_settlement_id, capture_mode,
		        carried_silver, provisions, land_target_q, land_target_r, land_cargo_intent, passage_messenger_id
		 FROM units WHERE id = $1 AND status = 'positioned' AND march_intent = 'passage_wait' FOR UPDATE`,
		shipID,
	).Scan(&u.id, &u.ownerID, &u.utype, &u.category, &u.size, &u.crew, &u.cargoUnitID,
		&u.status, &curQ, &curR, &u.targetQ, &u.targetR, &u.stance, &u.marchIntent, &u.colonyName, &u.homeSettlementID, &u.captureMode,
		&u.carriedSilver, &u.provisions, &u.landTargetQ, &u.landTargetR, &u.landCargoIntent, &u.passageMessengerID); err != nil {
		return nil // already released by a racing pass, or gone
	}
	if curQ == nil || curR == nil || u.homeSettlementID == nil {
		return fmt.Errorf("release passage wait ship: unit %s has no position or home settlement", shipID)
	}
	u.q, u.r = *curQ, *curR

	return h.dispatchReturnHome(ctx, tx, u, u.q, u.r, worldID, returnReasonPassageRelease)
}

// pickupArrived handles a "pickup" mission ship reaching the sea hex next to
// the shore api/handlers' pickup handler chose for the unit it was sent to
// fetch (R3, megaron_plan_hamta_hem.md, slice 2b). If the fetched unit is
// ALREADY standing right there (same owner, same hex, positioned), it boards
// immediately and the ship turns for home with it aboard. Otherwise the ship
// holds off the shore (status='positioned', march_intent='pickup_wait') and a
// ScheduledPickupTimeout, pickup_wait_ticks out, decides what happens if the
// unit never makes it (R6).
func (h *UnitArrivalHandler) pickupArrived(
	ctx context.Context, tx pgx.Tx,
	u unitRow, destQ, destR int, worldID uuid.UUID,
) error {
	if u.homeSettlementID == nil || u.pickupUnitID == nil || u.landTargetQ == nil || u.landTargetR == nil || u.pickupWaitTicks == nil {
		// Defensive: dispatch always sets these together for a pickup mission —
		// never strand the ship over a data inconsistency.
		slog.Warn("pickup arrival: missing home settlement/pickup unit/shore/wait, garrisoning ship in place instead", "unit", u.id)
		return h.arriveGarrison(ctx, tx, u, destQ, destR, nil, worldID)
	}
	shoreQ, shoreR := *u.landTargetQ, *u.landTargetR
	pickupID := *u.pickupUnitID

	var unitStatus string
	var unitOwner uuid.UUID
	var unitQ, unitR *int
	alreadyThere := false
	err := tx.QueryRow(ctx,
		`SELECT status, owner_id, q, r FROM units WHERE id = $1`, pickupID,
	).Scan(&unitStatus, &unitOwner, &unitQ, &unitR)
	if err != nil && err != pgx.ErrNoRows {
		return fmt.Errorf("pickup arrival: load fetched unit: %w", err)
	}
	if err == nil {
		alreadyThere = unitStatus == "positioned" && unitOwner == u.ownerID &&
			unitQ != nil && unitR != nil && *unitQ == shoreQ && *unitR == shoreR
	}

	if alreadyThere {
		boarded, bErr := h.boardPickupUnit(ctx, tx, u.id, pickupID, u.ownerID, shoreQ, shoreR, destQ, destR, worldID)
		if bErr != nil {
			return fmt.Errorf("pickup arrival: board unit: %w", bErr)
		}
		if boarded {
			u.cargoUnitID = &pickupID
			return h.dispatchReturnHome(ctx, tx, u, destQ, destR, worldID, returnReasonPickup)
		}
		// Fell through: the unit moved on/changed hands between the SELECT
		// above and boardPickupUnit's own re-check under FOR UPDATE. Wait for
		// it exactly as if it had never been there — the timeout is the only
		// backstop either way.
	}

	if _, err := tx.Exec(ctx,
		`UPDATE units SET
		   status       = 'positioned',
		   q            = $2,
		   r            = $3,
		   target_q     = NULL,
		   target_r     = NULL,
		   departs_at   = NULL,
		   arrives_at   = NULL,
		   depart_tick  = NULL,
		   arrive_tick  = NULL,
		   march_intent = 'pickup_wait',
		   updated_at   = now()
		 WHERE id = $1`,
		u.id, destQ, destR,
	); err != nil {
		return fmt.Errorf("pickupArrived: post pickup_wait: %w", err)
	}

	if h.scheduler == nil {
		return fmt.Errorf("pickupArrived: no scheduler configured, cannot arm pickup timeout")
	}
	var currentTick int
	_ = tx.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
	dueTick := currentTick + *u.pickupWaitTicks
	payload := unit.ScheduledUnitArrivalPayload{UnitID: u.id, WorldID: worldID}
	if err := h.scheduler.EnqueueTickTx(ctx, tx, worldID, events.ScheduledPickupTimeout, payload, dueTick); err != nil {
		return fmt.Errorf("pickupArrived: arm pickup timeout: %w", err)
	}

	if h.hub != nil {
		shipName := unit.LoadDisplayName(ctx, tx, u.id)
		pickupName := unit.LoadDisplayName(ctx, tx, pickupID)
		_ = h.hub.NotifyPlayer(ctx, worldID, u.ownerID, "PickupWaiting", 3, map[string]any{
			"unit_id":            u.id,
			"pickup_unit_id":     pickupID,
			"q":                  destQ,
			"r":                  destR,
			"waiting_until_tick": dueTick,
			"note":               fmt.Sprintf("%s waits off (%d,%d) for %s until tick %d", shipName, destQ, destR, pickupName, dueTick),
		})
	}
	slog.Info("pickup: ship waiting off shore for fetched unit", "unit", u.id, "pickup_unit", pickupID, "until_tick", dueTick)
	return nil
}

// boardPickupUnit is R5 (megaron_plan_hamta_hem.md): embarks the fetched land
// unit onto its pickup ship — the ONE function both R3 (pickupArrived, the
// ship reaches the shore and the unit is already standing there) and R4
// (arriveGarrison, the unit reaches the shore after the ship already does)
// call. Guard: the unit is still positioned, owned by the ship's owner, at
// exactly (shipQ,shipR), and the ship carries no other cargo — never boards a
// unit that has moved on, changed hands, or already been picked up by a
// racing pass. Same effect as Load (api/handlers/unit.go): unit -> embarked,
// settlement_id=NULL, q/r = the ship's hex (R10's tolerated state-change
// write); ship.cargo_unit_id = unit. Event ShipLoaded (existing type,
// existing meaning). Returns boarded=false, nil error when there was nothing
// to board — not an error, the caller falls back to waiting.
func (h *UnitArrivalHandler) boardPickupUnit(
	ctx context.Context, tx pgx.Tx,
	shipID, pickupUnitID, ownerID uuid.UUID, shoreQ, shoreR, shipQ, shipR int, worldID uuid.UUID,
) (bool, error) {
	var status string
	var unitOwner uuid.UUID
	var q, r *int
	if err := tx.QueryRow(ctx,
		`SELECT status, owner_id, q, r FROM units WHERE id = $1 FOR UPDATE`, pickupUnitID,
	).Scan(&status, &unitOwner, &q, &r); err != nil {
		if err == pgx.ErrNoRows {
			return false, nil // unit gone (disbanded/destroyed) — nothing to board
		}
		return false, fmt.Errorf("board pickup unit: load unit: %w", err)
	}
	// The unit must be standing exactly on the SHORE (a ship can never make
	// landfall — shipQ/shipR below is the ship's own SEA hex, a different
	// place, which is where the embarked unit is placed, not where it must
	// already stand to board).
	if status != "positioned" || unitOwner != ownerID || q == nil || r == nil || *q != shoreQ || *r != shoreR {
		return false, nil
	}

	var shipCargo *uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT cargo_unit_id FROM units WHERE id = $1 FOR UPDATE`, shipID,
	).Scan(&shipCargo); err != nil {
		return false, fmt.Errorf("board pickup unit: load ship: %w", err)
	}
	if shipCargo != nil {
		return false, nil // ship already carries something — defensive, should not happen
	}

	if _, err := tx.Exec(ctx,
		`UPDATE units SET status = 'embarked', settlement_id = NULL, q = $2, r = $3, updated_at = now() WHERE id = $1`,
		pickupUnitID, shipQ, shipR,
	); err != nil {
		return false, fmt.Errorf("board pickup unit: embark: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE units SET cargo_unit_id = $2, updated_at = now() WHERE id = $1`,
		shipID, pickupUnitID,
	); err != nil {
		return false, fmt.Errorf("board pickup unit: load ship: %w", err)
	}

	_, _ = h.eventStore.Append(ctx, shipID, events.StreamType(unit.StreamUnit), unit.EventShipLoaded,
		unit.ShipLoadedPayload{ShipUnitID: shipID, CargoUnitID: pickupUnitID, Q: shipQ, R: shipR}, worldID, nil)

	if h.hub != nil {
		unitName := unit.LoadDisplayName(ctx, tx, pickupUnitID)
		shipName := unit.LoadDisplayName(ctx, tx, shipID)
		_ = h.hub.NotifyPlayer(ctx, worldID, ownerID, "UnitFetched", 4, map[string]any{
			"unit_id": pickupUnitID,
			"ship_id": shipID,
			"q":       shipQ,
			"r":       shipR,
			"note":    fmt.Sprintf("%s is aboard %s, sailing home", unitName, shipName),
		})
	}
	slog.Info("pickup: unit boarded fetching ship", "ship", shipID, "unit", pickupUnitID)
	return true, nil
}

// pickupWaitingShipFor is R4's own half of R5's boarding (megaron_plan_
// hamta_hem.md): the fetched unit reached the shore AFTER its ship already
// did. Looks for a ship of ownerID currently holding pickup_wait for exactly
// pickupUnitID at (shoreQ,shoreR), locking the row FOR UPDATE if found.
// found=false (not an error) when no such ship is waiting there.
func (h *UnitArrivalHandler) pickupWaitingShipFor(
	ctx context.Context, tx pgx.Tx, worldID, ownerID, pickupUnitID uuid.UUID, shoreQ, shoreR int,
) (unitRow, bool, error) {
	var u unitRow
	err := tx.QueryRow(ctx,
		`SELECT id, owner_id, type, category, size, crew, cargo_unit_id,
		        status, q, r, target_q, target_r, stance, march_intent, colony_name, home_settlement_id, capture_mode,
		        carried_silver, provisions
		   FROM units
		  WHERE world_id = $1 AND owner_id = $2 AND status = 'positioned' AND march_intent = 'pickup_wait'
		    AND pickup_unit_id = $3 AND land_target_q = $4 AND land_target_r = $5
		  FOR UPDATE`,
		worldID, ownerID, pickupUnitID, shoreQ, shoreR,
	).Scan(&u.id, &u.ownerID, &u.utype, &u.category, &u.size, &u.crew, &u.cargoUnitID,
		&u.status, &u.q, &u.r, &u.targetQ, &u.targetR, &u.stance, &u.marchIntent, &u.colonyName, &u.homeSettlementID, &u.captureMode,
		&u.carriedSilver, &u.provisions)
	if err != nil {
		if err == pgx.ErrNoRows {
			return unitRow{}, false, nil
		}
		return unitRow{}, false, err
	}
	return u, true, nil
}

// HandlePickupTimeout is the scheduled handler for ScheduledPickupTimeout
// (R6, megaron_plan_hamta_hem.md): a pickup ship's wait off the shore has run
// out — turn it home empty (the unit never made it). Idempotent — a ship no
// longer holding pickup_wait (already released by R3/R4's own boarding,
// destroyed, captured) is a no-op. Mirrors HandleSentryReturn's own shape.
func (h *UnitArrivalHandler) HandlePickupTimeout(ctx context.Context, e events.ScheduledEvent) error {
	var payload unit.ScheduledUnitArrivalPayload
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal pickup timeout payload: %w", err)
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var u unitRow
	var pickupID *uuid.UUID
	// q/r are scanned as nullable: a ship that left 'pickup_wait' by some
	// OTHER path (sunk/captured — disbanded rows carry NULL q/r) must still
	// load cleanly here so the idempotency guard below can see its status and
	// no-op, exactly as HandleSentryReturn's own guard does for the same
	// reason (unit_arrival.go's resolve() doc comment on scanning q/r).
	var qPtr, rPtr *int
	if err := tx.QueryRow(ctx,
		`SELECT id, owner_id, type, category, size, crew, cargo_unit_id,
		        status, q, r, target_q, target_r, stance, march_intent, colony_name, home_settlement_id, capture_mode,
		        carried_silver, provisions, pickup_unit_id
		 FROM units WHERE id = $1 FOR UPDATE`,
		payload.UnitID,
	).Scan(&u.id, &u.ownerID, &u.utype, &u.category, &u.size, &u.crew, &u.cargoUnitID,
		&u.status, &qPtr, &rPtr, &u.targetQ, &u.targetR, &u.stance, &u.marchIntent, &u.colonyName, &u.homeSettlementID, &u.captureMode,
		&u.carriedSilver, &u.provisions, &pickupID); err != nil {
		if err == pgx.ErrNoRows {
			return nil // ship gone (disbanded/destroyed/captured)
		}
		return fmt.Errorf("load pickup-waiting ship: %w", err)
	}

	// Idempotent: only a ship still holding pickup_wait times out here.
	if u.status != "positioned" || u.marchIntent == nil || *u.marchIntent != "pickup_wait" || u.homeSettlementID == nil {
		return tx.Commit(ctx)
	}
	if qPtr == nil || rPtr == nil {
		return fmt.Errorf("pickup-waiting ship %s has no position", u.id)
	}
	u.q, u.r = *qPtr, *rPtr

	if h.hub != nil {
		shipName := unit.LoadDisplayName(ctx, tx, u.id)
		var note string
		if pickupID != nil {
			pickupName := unit.LoadDisplayName(ctx, tx, *pickupID)
			note = fmt.Sprintf("%s waited, but %s did not reach the shore — sailing home without it", shipName, pickupName)
		} else {
			note = fmt.Sprintf("%s waited, but the unit it was sent to fetch is gone — sailing home", shipName)
		}
		_ = h.hub.NotifyPlayer(ctx, payload.WorldID, u.ownerID, "PickupTimedOut", 3, map[string]any{
			"unit_id": u.id,
			"q":       u.q,
			"r":       u.r,
			"note":    note,
		})
	}

	if err := h.dispatchReturnHome(ctx, tx, u, u.q, u.r, payload.WorldID, returnReasonPickup); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// pickupReturned re-garrisons a pickup ship that finished its return leg (R6,
// megaron_plan_hamta_hem.md) and — if it made it home with the fetched unit
// still aboard (cargoUnitID set) — disembarks that unit straight into the
// home settlement's garrison in the SAME transaction, exactly like Unload's
// own fall (a): the mission's natural completion, not a second sequence the
// Wanax must remember to run (an embarked unit left sitting in port pays
// field ration until unloaded — upkeep.go). Same "bypass the hex→settlement
// lookup" reason as exploreReturned/damagedShipReturned above.
func (h *UnitArrivalHandler) pickupReturned(
	ctx context.Context, tx pgx.Tx,
	u unitRow, destQ, destR int, worldID uuid.UUID,
) error {
	if u.homeSettlementID == nil {
		slog.Warn("pickup return: unit has no home_settlement_id, garrisoning in place instead", "unit", u.id)
		return h.arriveGarrison(ctx, tx, u, destQ, destR, nil, worldID)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE units SET
		   status             = 'garrison',
		   q                  = $2,
		   r                  = $3,
		   settlement_id      = $4,
		   home_settlement_id = NULL,
		   target_q           = NULL,
		   target_r           = NULL,
		   departs_at         = NULL,
		   arrives_at         = NULL,
		   depart_tick        = NULL,
		   arrive_tick        = NULL,
		   march_intent       = NULL,
		   updated_at         = now()
		 WHERE id = $1`,
		u.id, destQ, destR, *u.homeSettlementID,
	); err != nil {
		return fmt.Errorf("pickupReturned: re-garrison ship: %w", err)
	}

	_, _ = h.eventStore.Append(ctx, u.id, events.StreamType(unit.StreamUnit), unit.EventUnitArrived,
		unit.UnitArrivedPayload{UnitID: u.id, Q: destQ, R: destR, NewStatus: "garrison"}, worldID, nil)

	if h.hub != nil {
		_ = h.hub.NotifyPlayer(ctx, worldID, u.ownerID, "UnitArrived", 4, map[string]any{
			"unit_id": u.id,
			"name":    unit.LoadDisplayName(ctx, tx, u.id),
			"q":       destQ,
			"r":       destR,
			"status":  "garrison",
		})
	}

	if u.cargoUnitID != nil {
		cargoID := *u.cargoUnitID
		if _, err := tx.Exec(ctx,
			`UPDATE units SET status = 'garrison', settlement_id = $2, q = $3, r = $4, updated_at = now()
			 WHERE id = $1`,
			cargoID, *u.homeSettlementID, destQ, destR,
		); err != nil {
			return fmt.Errorf("pickupReturned: disembark fetched unit: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE units SET cargo_unit_id = NULL WHERE id = $1`, u.id); err != nil {
			return fmt.Errorf("pickupReturned: clear ship cargo: %w", err)
		}
		_, _ = h.eventStore.Append(ctx, u.id, events.StreamType(unit.StreamUnit), unit.EventShipUnloaded,
			unit.ShipUnloadedPayload{ShipUnitID: u.id, CargoUnitID: cargoID, Q: destQ, R: destR}, worldID, nil)

		if h.hub != nil {
			_ = h.hub.NotifyPlayer(ctx, worldID, u.ownerID, "UnitArrived", 4, map[string]any{
				"unit_id": cargoID,
				"name":    unit.LoadDisplayName(ctx, tx, cargoID),
				"q":       destQ,
				"r":       destR,
				"status":  "garrison",
			})
		}
		slog.Info("pickup: ship returned home with fetched unit", "unit", u.id, "settlement", *u.homeSettlementID, "cargo", cargoID)
		return nil
	}

	slog.Info("pickup: ship returned home empty", "unit", u.id, "settlement", *u.homeSettlementID)
	return nil
}

// exploreArrived handles a unit reaching its explore target: instead of
// garrisoning or fighting, it immediately turns back toward the settlement it
// departed from (captured at dispatch as home_settlement_id, since the normal
// march dispatch nulls settlement_id). The outbound fog sweep already ran at
// the top of resolve(); the return leg's own arrival sweeps the way home.
func (h *UnitArrivalHandler) exploreArrived(
	ctx context.Context, tx pgx.Tx,
	u unitRow, destQ, destR int, worldID uuid.UUID,
) error {

	// An area expedition (expedition.go) walks on to its next unseen hex
	// instead; a point explore — or one whose expedition was ended by a
	// recall/redirect — turns here as it always has.
	if e, ok, err := loadExpedition(ctx, tx, u.id); err != nil {
		return err
	} else if ok && !e.Homeward {
		return h.expeditionArrived(ctx, tx, u, destQ, destR, worldID, e)
	}

	if u.homeSettlementID == nil {
		// Defensive: dispatch validated the unit had a home settlement, but
		// never strand a unit with nothing to return to.
		slog.Warn("explore arrival: unit has no home_settlement_id, garrisoning in place instead of returning", "unit", u.id)
		return h.arriveGarrison(ctx, tx, u, destQ, destR, nil, worldID)
	}

	h.reportScoutFindings(ctx, tx, u, destQ, destR, worldID)

	// The generic FOW sweep at the top of resolve() only recorded the literal
	// path hexes walked, not the live-vision radius a real eye standing at
	// destQ,destR would reveal (temenos_buggrapporter.md "FOW lyfter inte runt
	// en nyutforskad hex", 2026-08-08). An explore unit turns for home in this
	// same transaction, so — unlike a garrisoning unit, which sits still until
	// the player's next /map read captures its radius — it may never get a
	// live-vision window an ordinary poll can catch. Force that same
	// radius-write once, here, while the eye is known to stand at destQ,destR.
	eyeKind := province.EyeLandUnit
	if u.category == "naval" {
		eyeKind = province.EyeShip
	}
	if err := province.SweepLiveRadius(ctx, tx, worldID, u.ownerID,
		province.MapPosition{Q: destQ, R: destR}, eyeKind); err != nil {
		slog.Warn("explore arrival: live-radius FOW sweep failed", "unit", u.id, "err", err)
	}

	return h.dispatchReturnHome(ctx, tx, u, destQ, destR, worldID, returnReasonExplore)
}

// reportScoutFindings tells the owner what their scout found at the far hex it
// just revealed (temenos_todo.md "Explore-order kommer hem utan rapport" —
// previously the map updated silently and the only pings were "turning
// home"/"back in garrison", never what was actually seen). The FOW sweep at
// the top of resolve() already recorded this hex as permanently scouted; this
// reads back its terrain + deposit flags (same map_tiles columns foundColony
// already reads for the identical purpose) so the report states the OUTCOME —
// "copper found" or "nothing of value" — never "go check the hex" (Fas 2.3).
// Best-effort: a read failure here must never abort the arrival — the unit
// still needs to turn for home regardless.
func (h *UnitArrivalHandler) reportScoutFindings(
	ctx context.Context, tx pgx.Tx,
	u unitRow, destQ, destR int, worldID uuid.UUID,
) {
	var terrain string
	var copperDep, tinDep, silverDep, cedarDep bool
	if err := tx.QueryRow(ctx,
		`SELECT terrain, copper_deposit, tin_deposit,
		        COALESCE(silver_deposit,false), COALESCE(cedar_deposit,false)
		 FROM map_tiles WHERE world_id = $1 AND q = $2 AND r = $3`,
		worldID, destQ, destR,
	).Scan(&terrain, &copperDep, &tinDep, &silverDep, &cedarDep); err != nil {
		slog.Warn("scout report: could not load destination tile, skipping report", "unit", u.id, "q", destQ, "r", destR, "err", err)
		return
	}

	_, _ = h.eventStore.Append(ctx, u.id, events.StreamType(unit.StreamUnit), unit.EventUnitScoutReport,
		unit.UnitScoutReportPayload{
			UnitID:        u.id,
			Q:             destQ,
			R:             destR,
			Terrain:       terrain,
			CopperDeposit: copperDep,
			TinDeposit:    tinDep,
			SilverDeposit: silverDep,
			CedarDeposit:  cedarDep,
		}, worldID, nil)

	if h.hub != nil {
		_ = h.hub.NotifyPlayer(ctx, worldID, u.ownerID, "ScoutReport", 4, map[string]any{
			"unit_id":        u.id,
			"q":              destQ,
			"r":              destR,
			"terrain":        terrain,
			"copper_deposit": copperDep,
			"tin_deposit":    tinDep,
			"silver_deposit": silverDep,
			"cedar_deposit":  cedarDep,
		})
	}

	slog.Info("scout report filed", "unit", u.id, "q", destQ, "r", destR, "terrain", terrain,
		"copper", copperDep, "tin", tinDep, "silver", silverDep, "cedar", cedarDep)
}

// returnReason selects which event type and notification dispatchReturnHome
// emits at the end of the dance. Eventsemantik is frozen forever (CLAUDE.md
// §Events) — a new caller motive gets a new returnReason + a new event type,
// never a reinterpretation of an existing one. The route/scheduling logic
// itself never branches on this; only the tail (event + notify) does.
type returnReason int

const (
	// returnReasonExplore covers both existing callers of dispatchReturnHome
	// (exploreArrived's target-reached auto-return and HandleSentryReturn's
	// patrol timer) — today's exact behaviour, unchanged.
	returnReasonExplore returnReason = iota
	// returnReasonStarvation: a positioned naval unit's crew fell to
	// navalStarvationReturnCrewFraction of its full crew (upkeep.go
	// applyAttrition) and it turns for home on its own.
	returnReasonStarvation
	// returnReasonSweptFromSea: R6's one-time deploy transition
	// (main.go's sweepShipsAtSeaOnDeploy) turns a ship for home because it was
	// left 'positioned' at sea under the pre-R3 rules — R3 means it can never
	// receive a fresh order again otherwise.
	returnReasonSweptFromSea
	// returnReasonPassageRelease: PassageScanHandler's release phase (R4,
	// megaron_plan_ordna_passage.md 3b-3) turns a passage_wait ship for home —
	// either its runner is ready to travel with it, or nothing is left to
	// wait for.
	returnReasonPassageRelease
	// returnReasonPickup: a pickup mission's ship turns for home (R6,
	// megaron_plan_hamta_hem.md, slice 2b) — either the fetched unit boarded
	// (R3/R4) and rides home with it, or ScheduledPickupTimeout gave up
	// waiting and it sails home empty. Which one happened is read off
	// u.cargoUnitID at call time (set by the caller right after a successful
	// boardPickupUnit), not carried as a separate flag.
	returnReasonPickup
)

// dispatchReturnHome turns a field unit around and marches it back to its home
// settlement's departure hex (the settlement's own province hex for land units,
// its nearest sea neighbour for naval). Shared by the explore auto-return
// (exploreArrived), the naval sentry patrol timer (HandleSentryReturn), and the
// naval starvation auto-return (upkeep.go's applyAttrition): all three need the
// identical "route home via A*, mark marching intent=explore_return, schedule
// the return arrival" dance — only the reason for turning differs, and only the
// tail event/notification reflects that. fromQ/fromR is the hex the unit turns
// home FROM. Assumes u.homeSettlementID != nil (callers guard it).
func (h *UnitArrivalHandler) dispatchReturnHome(
	ctx context.Context, tx pgx.Tx,
	u unitRow, fromQ, fromR int, worldID uuid.UUID, reason returnReason,
) error {
	var homeQ, homeR int
	if err := tx.QueryRow(ctx,
		`SELECT p.map_q, p.map_r FROM settlements s JOIN provinces p ON p.id = s.province_id WHERE s.id = $1`,
		*u.homeSettlementID,
	).Scan(&homeQ, &homeR); err != nil {
		return fmt.Errorf("dispatchReturnHome: load home settlement province: %w", err)
	}
	if u.category == "naval" {
		if seaQ, seaR, found, seaErr := province.NearestSeaNeighbor(ctx, tx, worldID, homeQ, homeR); seaErr != nil {
			return fmt.Errorf("dispatchReturnHome: resolve naval return hex: %w", seaErr)
		} else if found {
			homeQ, homeR = seaQ, seaR
		} else {
			slog.Warn("return home: home settlement has no adjacent sea hex, using its land hex", "unit", u.id, "settlement", *u.homeSettlementID)
		}
	}

	// Route home via A* — the same passability graph the outbound leg proved.
	path, pathTicks, pathOK, pathErr := province.FindPath(ctx, tx, worldID,
		province.MapPosition{Q: fromQ, R: fromR},
		province.MapPosition{Q: homeQ, R: homeR},
		u.category,
	)
	var moveTicks float64
	if pathErr == nil && pathOK {
		moveTicks = pathTicks
	} else {
		path = nil // fell back to straight line below — no real path to save (R5)
		// Defensive fallback: the outbound march already proved passability
		// between these regions, so this should not happen.
		slog.Warn("return home: FindPath failed, falling back to straight line", "unit", u.id, "err", pathErr)
		dist := province.HexDistance(province.MapPosition{Q: fromQ, R: fromR}, province.MapPosition{Q: homeQ, R: homeR})
		if dist < 1 {
			dist = 1
		}
		var fromTerrain string
		_ = tx.QueryRow(ctx,
			`SELECT terrain_type FROM provinces WHERE world_id = $1 AND map_q = $2 AND map_r = $3`,
			worldID, fromQ, fromR,
		).Scan(&fromTerrain)
		if fromTerrain == "" {
			fromTerrain = "plains"
		}
		moveTicks = province.TerrainMoveTicks(fromTerrain) * float64(dist)
	}
	returnIntent := "explore_return"
	// R6 (megaron_plan_hamta_hem.md): a pickup ship sailing home WITH its
	// fetched unit aboard gets its own resolve-branch (pickupReturned) so the
	// unit can be unloaded straight into garrison on arrival — an empty
	// pickup return (timed out, or nothing was ever boarded) is otherwise
	// indistinguishable from a plain explore return and reuses that path.
	if reason == returnReasonPickup && u.cargoUnitID != nil {
		returnIntent = "pickup_return"
	}
	arrivesAt, _, err := h.dispatchLeg(ctx, tx, u, fromQ, fromR, homeQ, homeR, path, moveTicks, returnIntent, false, worldID)
	if err != nil {
		return fmt.Errorf("dispatchReturnHome: %w", err)
	}

	// Tail: reason picks the event type + notification kind. The route/dispatch
	// above is identical for every reason — only what the Wanax is told differs.
	// Named once here: both tails below report the same turning-for-home unit
	// (megaron_plan_dispatches.md §4 — "Scout returning home" named no scout).
	name := unit.LoadDisplayName(ctx, tx, u.id)
	if reason == returnReasonStarvation {
		_, _ = h.eventStore.Append(ctx, u.id, events.StreamType(unit.StreamUnit), unit.EventUnitReturnedStarving,
			unit.UnitReturnedStarvingPayload{
				UnitID:           u.id,
				Q:                fromQ,
				R:                fromR,
				HomeSettlementID: *u.homeSettlementID,
				ArrivesAt:        arrivesAt.Format(time.RFC3339),
				CrewAfter:        u.crew,
			}, worldID, nil)

		if h.hub != nil {
			_ = h.hub.NotifyPlayer(ctx, worldID, u.ownerID, "UnitReturnedStarving", 3, map[string]any{
				"unit_id":    u.id,
				"name":       name,
				"q":          fromQ,
				"r":          fromR,
				"arrives_at": arrivesAt,
				"crew_after": u.crew,
			})
		}
		slog.Info("field unit turning for home (starving)", "unit", u.id, "from_q", fromQ, "from_r", fromR, "home_q", homeQ, "home_r", homeR, "crew_after", u.crew)
		return nil
	}
	if reason == returnReasonSweptFromSea {
		_, _ = h.eventStore.Append(ctx, u.id, events.StreamType(unit.StreamUnit), unit.EventUnitSweptFromSea,
			unit.UnitSweptFromSeaPayload{
				UnitID:           u.id,
				Q:                fromQ,
				R:                fromR,
				HomeSettlementID: *u.homeSettlementID,
				ArrivesAt:        arrivesAt.Format(time.RFC3339),
			}, worldID, nil)

		if h.hub != nil {
			_ = h.hub.NotifyPlayer(ctx, worldID, u.ownerID, "ShipSweptFromSea", 3, map[string]any{
				"unit_id":    u.id,
				"name":       name,
				"q":          fromQ,
				"r":          fromR,
				"arrives_at": arrivesAt,
				"reason":     "returning to port — ships at sea take no orders",
			})
		}
		slog.Info("ship swept from sea, turning for home", "unit", u.id, "from_q", fromQ, "from_r", fromR, "home_q", homeQ, "home_r", homeR)
		return nil
	}

	_, _ = h.eventStore.Append(ctx, u.id, events.StreamType(unit.StreamUnit), unit.EventUnitExploreReturned,
		unit.UnitExploreReturnedPayload{
			UnitID:           u.id,
			Q:                fromQ,
			R:                fromR,
			HomeSettlementID: *u.homeSettlementID,
			ArrivesAt:        arrivesAt.Format(time.RFC3339),
		}, worldID, nil)

	if h.hub != nil {
		_ = h.hub.NotifyPlayer(ctx, worldID, u.ownerID, "UnitExploreReturned", 5, map[string]any{
			"unit_id":    u.id,
			"name":       name,
			"q":          fromQ,
			"r":          fromR,
			"arrives_at": arrivesAt,
		})
	}

	slog.Info("field unit turning for home", "unit", u.id, "from_q", fromQ, "from_r", fromR, "home_q", homeQ, "home_r", homeR)
	return nil
}

// dispatchLeg puts a field unit on the march from (fromQ,fromR) to (toQ,toR)
// along an already-found path whose raw cost is moveTicks: the unit's own
// speed (TravelFactor), the saved route, the unit row and its scheduled
// arrival. Shared by dispatchReturnHome and the expedition's legs
// (expedition.go) — one home for "send this unit on its next leg". It clears
// every one-shot mission column, same as the return leg always did.
// home_settlement_id is kept. path may be nil (straight-line fallback).
// keepStance: an expedition's stance (what it does on a meeting) holds for the
// whole journey; a return leg drops it, as it always has.
func (h *UnitArrivalHandler) dispatchLeg(
	ctx context.Context, tx pgx.Tx, u unitRow,
	fromQ, fromR, toQ, toR int, path []province.MapPosition, moveTicks float64,
	intent string, keepStance bool, worldID uuid.UUID,
) (time.Time, int, error) {
	// Mirror the outbound leg's speed multipliers (march_start.go's
	// TravelFactor) — without these a war galley/merchantman's return trip
	// silently used the unmultiplied path cost, making the return leg a
	// different (and for a merchantman, much shorter) duration than the
	// outbound one for the exact same distance: it looked like the ship
	// sailed out at its true speed but teleported home.
	travelTicks := legTravelTicks(unit.Type(u.utype), u.crew, u.cargoUnitID != nil, moveTicks)
	var currentTick int
	_ = tx.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
	// arrives_at mirrors the real tick-scheduled return (travelTicks × real
	// seconds/tick), not moveTicks-as-hours — same reason as the outbound leg in
	// unit.go March: the map animates the ship against this window.
	arrivesAt := h.clk.Now().Add(time.Duration(travelTicks*tick.TickSeconds) * time.Second)

	// movement 2a, R1/R5: save the path already found above — never re-search
	// it at read time. NULL when FindPath fell back to a straight line (path
	// was cleared to nil above).
	var marchRoute []byte
	if len(path) >= 2 {
		if stepHours, shErr := province.StepHoursDB(ctx, tx, worldID, path, u.category); shErr == nil {
			if route, ok := BuildRoute(path, stepHours, currentTick, currentTick+travelTicks); ok {
				if raw, mErr := json.Marshal(route); mErr == nil {
					marchRoute = raw
				}
			}
		}
	}

	// land_target_q/r, land_cargo_intent and passage_messenger_id (mig 147/149)
	// are cleared here, not just at re-garrison: a "land" or "passage" ship's
	// mission data would otherwise ride along, stale, onto this very return
	// leg — and for a passage ship specifically, boardShipMissions reads
	// land_target_q/r generically for ANY marching ship, so a leftover value
	// (the OUTBOUND disembark hex) would make the release phase's return leg
	// try to disembark a runner at the port it is leaving, not the one it is
	// sailing home to (megaron_plan_ordna_passage.md 3b-3 R4). Safe for every
	// other caller too: land_target_q/r is otherwise only ever non-NULL for a
	// mission that has just ended, one way or another, right here.
	if _, err := tx.Exec(ctx,
		`UPDATE units SET
		   status        = 'marching',
		   q             = $2,
		   r             = $3,
		   target_q      = $4,
		   target_r      = $5,
		   departs_at    = now(),
		   arrives_at    = $6,
		   depart_tick   = $8,
		   arrive_tick   = $9,
		   settlement_id = NULL,
		   stance        = CASE WHEN $12 THEN stance ELSE NULL END,
		   sentry_q      = NULL,
		   sentry_r      = NULL,
		   march_intent  = $7,
		   home_settlement_id = $10,
		   land_target_q = NULL,
		   land_target_r = NULL,
		   land_cargo_intent = NULL,
		   passage_messenger_id = NULL,
		   pickup_unit_id = NULL,
		   pickup_wait_ticks = NULL,
		   march_route   = $11,
		   updated_at    = now()
		 WHERE id = $1`,
		u.id, fromQ, fromR, toQ, toR, arrivesAt, intent, currentTick, currentTick+travelTicks, u.homeSettlementID, marchRoute, keepStance,
	); err != nil {
		return time.Time{}, 0, fmt.Errorf("dispatch march leg: %w", err)
	}

	if h.scheduler == nil {
		return time.Time{}, 0, fmt.Errorf("no scheduler configured, cannot dispatch march leg")
	}
	arriveTick := currentTick + travelTicks
	arrPayload := unit.ScheduledUnitArrivalPayload{UnitID: u.id, WorldID: worldID, ArriveTick: &arriveTick}
	if err := h.scheduler.EnqueueTickTx(ctx, tx, worldID, events.ScheduledUnitArrival, arrPayload, currentTick+travelTicks); err != nil {
		return time.Time{}, 0, fmt.Errorf("schedule leg arrival: %w", err)
	}
	return arrivesAt, arriveTick, nil
}

// exploreReturned re-garrisons a unit that finished the explore-order's
// return leg. It forces settlement_id = home_settlement_id directly instead
// of looking up a settlement by (destQ, destR): a naval unit's return target
// is the sea hex adjacent to its home settlement (see exploreArrived above),
// which has no settlement row of its own, so the normal hex→settlement lookup
// would leave it 'positioned' at sea instead of back in garrison.
func (h *UnitArrivalHandler) exploreReturned(
	ctx context.Context, tx pgx.Tx,
	u unitRow, destQ, destR int, worldID uuid.UUID,
) error {
	exp, expOK, err := loadExpedition(ctx, tx, u.id)
	if err != nil {
		return err
	}
	if expOK {
		home, err := expeditionReturnDestination(ctx, tx, u, destQ, destR, worldID)
		if err != nil {
			return err
		}
		if home == nil {
			return h.stopExpedition(ctx, tx, u, destQ, destR, worldID)
		}
		changed := u.homeSettlementID == nil || *u.homeSettlementID != home.id
		u.homeSettlementID = &home.id
		if changed || home.q != destQ || home.r != destR {
			_, arriveTick, err := h.dispatchLeg(ctx, tx, u, destQ, destR, home.q, home.r, home.path, home.cost, "explore_return", false, worldID)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `UPDATE units SET home_settlement_id = $2 WHERE id = $1`, u.id, home.id)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `UPDATE unit_expeditions SET leg_arrive_tick = $2 WHERE unit_id = $1`, u.id, arriveTick)
			return err
		}
	}
	if u.homeSettlementID == nil {
		return h.arriveGarrison(ctx, tx, u, destQ, destR, nil, worldID)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE units SET
		   status             = 'garrison',
		   q                  = $2,
		   r                  = $3,
		   settlement_id      = $4,
		   home_settlement_id = NULL,
		   target_q           = NULL,
		   target_r           = NULL,
		   departs_at         = NULL,
		   arrives_at         = NULL,
		   depart_tick        = NULL,
		   arrive_tick        = NULL,
		   march_intent       = NULL,
		   updated_at         = now()
		 WHERE id = $1`,
		u.id, destQ, destR, *u.homeSettlementID,
	); err != nil {
		return fmt.Errorf("exploreReturned: re-garrison: %w", err)
	}

	arrivedPayload := unit.UnitArrivedPayload{UnitID: u.id, Q: destQ, R: destR, NewStatus: "garrison"}
	arrivedBody := map[string]any{"unit_id": u.id, "name": unit.LoadDisplayName(ctx, tx, u.id), "q": destQ, "r": destR, "status": "garrison"}
	if expOK {
		event, err := h.eventStore.AppendTx(ctx, tx, u.id, events.StreamType(unit.StreamUnit), unit.EventUnitArrived, arrivedPayload, worldID, nil)
		if err != nil {
			return err
		}
		if err := h.expeditionOutcome(ctx, tx, event, worldID, u.ownerID, "UnitArrived", 4, arrivedBody); err != nil {
			return err
		}
	} else {
		_, _ = h.eventStore.Append(ctx, u.id, events.StreamType(unit.StreamUnit), unit.EventUnitArrived, arrivedPayload, worldID, nil)
		if h.hub != nil {
			_ = h.hub.NotifyPlayer(ctx, worldID, u.ownerID, "UnitArrived", 4, arrivedBody)
		}
	}

	if expOK && exp.Homeward {
		if err := h.expeditionHome(ctx, tx, u, *u.homeSettlementID, worldID, exp); err != nil {
			return err
		}
	}

	slog.Info("unit returned home from explore", "unit", u.id, "settlement", *u.homeSettlementID)
	return nil
}

// damagedShipReturned re-garrisons a ship that finished the damaged-return
// leg dispatched by combat.BattleTickHandler.sendDamagedShipHome
// (megaron_plan_skeppsreparation.md Slice B point 3). Same shape as
// exploreReturned above and for the identical reason (the return target is
// the sea hex adjacent to home, which has no settlement row of its own) —
// the only difference is the notification: this is a damaged ship coming
// home to repair, not a scout, so it gets the plain "UnitArrived" kind
// instead of exploreReturned's scout-specific wording. hull is untouched
// here; the ship arrives exactly as damaged as it left the battle, and stays
// that way until Slice C's repair mechanic (not yet built) restores it.
func (h *UnitArrivalHandler) damagedShipReturned(
	ctx context.Context, tx pgx.Tx,
	u unitRow, destQ, destR int, worldID uuid.UUID,
) error {
	if u.homeSettlementID == nil {
		// Defensive: sendDamagedShipHome always resolves and persists one
		// before dispatching. Should not happen.
		slog.Warn("damaged return: unit has no home_settlement_id, garrisoning in place instead", "unit", u.id)
		return h.arriveGarrison(ctx, tx, u, destQ, destR, nil, worldID)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE units SET
		   status             = 'garrison',
		   q                  = $2,
		   r                  = $3,
		   settlement_id      = $4,
		   home_settlement_id = NULL,
		   target_q           = NULL,
		   target_r           = NULL,
		   departs_at         = NULL,
		   arrives_at         = NULL,
		   depart_tick        = NULL,
		   arrive_tick        = NULL,
		   march_intent       = NULL,
		   updated_at         = now()
		 WHERE id = $1`,
		u.id, destQ, destR, *u.homeSettlementID,
	); err != nil {
		return fmt.Errorf("damagedShipReturned: re-garrison: %w", err)
	}

	_, _ = h.eventStore.Append(ctx, u.id, events.StreamType(unit.StreamUnit), unit.EventUnitArrived,
		unit.UnitArrivedPayload{UnitID: u.id, Q: destQ, R: destR, NewStatus: "garrison"}, worldID, nil)

	if h.hub != nil {
		_ = h.hub.NotifyPlayer(ctx, worldID, u.ownerID, "UnitArrived", 4, map[string]any{
			"unit_id": u.id,
			"name":    unit.LoadDisplayName(ctx, tx, u.id),
			"q":       destQ,
			"r":       destR,
			"status":  "garrison",
		})
	}

	// Shared by damaged_return and captured_return; name which one in the log.
	slog.Info("ship returned home", "intent", *u.marchIntent, "unit", u.id, "settlement", *u.homeSettlementID)
	return nil
}

// SentryPatrolTicks is how long a naval sentry holds its patrol hex before the
// auto-return timer turns it home. Tunable (ticks of patrol). No recall verb
// exists — this timer is the only control, so a sentry order can never strand
// a ship ("self-terminating sea orders").
const SentryPatrolTicks = 24

// PickupWaitDefaultTicks/PickupWaitMaxTicks (R1, megaron_plan_hamta_hem.md,
// slice 2b) bound how long a pickup ship holds off the shore for the unit it
// was sent to fetch before ScheduledPickupTimeout sends it home regardless —
// the pickup verb's "nån måtta får det vara" (Timothy). Strawman values,
// same uncalibrated status as SentryPatrolTicks above — tune in soak, not here.
const (
	PickupWaitDefaultTicks = 3
	PickupWaitMaxTicks     = 12
)

// sentryArrived posts a naval unit on patrol: it reached its coastal_sea target
// and now HOLDS there (status='positioned' + stance='sentry' + sentry_q/r) — the
// same posture SetStance produces, so the existing InterceptScan seizes enemy
// caravans passing within reach and the ship projects fog-of-war over the
// approaches. A ScheduledSentryReturn timer (SentryPatrolTicks out) turns it home
// via dispatchReturnHome; there is no recall, the timer is the only control.
func (h *UnitArrivalHandler) sentryArrived(
	ctx context.Context, tx pgx.Tx,
	u unitRow, destQ, destR int, worldID uuid.UUID,
) error {
	if h.scheduler == nil {
		return fmt.Errorf("sentryArrived: no scheduler configured, cannot arm patrol timer")
	}
	if _, err := tx.Exec(ctx,
		`UPDATE units SET
		   status       = 'positioned',
		   q            = $2,
		   r            = $3,
		   stance       = 'sentry',
		   sentry_q     = $2,
		   sentry_r     = $3,
		   target_q     = NULL,
		   target_r     = NULL,
		   departs_at   = NULL,
		   arrives_at   = NULL,
		   depart_tick  = NULL,
		   arrive_tick  = NULL,
		   march_intent = NULL,
		   updated_at   = now()
		 WHERE id = $1`,
		u.id, destQ, destR,
	); err != nil {
		return fmt.Errorf("sentryArrived: post sentry: %w", err)
	}

	var currentTick int
	_ = tx.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
	retPayload := unit.ScheduledUnitArrivalPayload{UnitID: u.id, WorldID: worldID}
	if err := h.scheduler.EnqueueTickTx(ctx, tx, worldID, events.ScheduledSentryReturn, retPayload, currentTick+SentryPatrolTicks); err != nil {
		return fmt.Errorf("sentryArrived: arm patrol timer: %w", err)
	}

	if h.hub != nil {
		_ = h.hub.NotifyPlayer(ctx, worldID, u.ownerID, "UnitArrived", 4, map[string]any{
			"unit_id": u.id,
			"name":    unit.LoadDisplayName(ctx, tx, u.id),
			"q":       destQ,
			"r":       destR,
			"status":  "positioned",
			"stance":  "sentry",
		})
	}

	slog.Info("naval unit posted on sentry patrol", "unit", u.id, "q", destQ, "r", destR, "return_tick", currentTick+SentryPatrolTicks)
	return nil
}

// HandleSentryReturn is the scheduled handler for ScheduledSentryReturn: the
// patrol timer fired, so turn the ship home. Idempotent — if the unit is no
// longer holding sentry (it moved, was intercepted/destroyed, or already turned
// home) it is a no-op. Mirrors Handle: FOR UPDATE, guard, then dispatch home.
func (h *UnitArrivalHandler) HandleSentryReturn(ctx context.Context, e events.ScheduledEvent) error {
	var payload unit.ScheduledUnitArrivalPayload
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal sentry return payload: %w", err)
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var u unitRow
	if err := tx.QueryRow(ctx,
		`SELECT id, owner_id, type, category, size, crew, cargo_unit_id,
		        status, q, r, target_q, target_r, stance, march_intent, colony_name, home_settlement_id, capture_mode,
		        carried_silver, provisions
		 FROM units WHERE id = $1 FOR UPDATE`,
		payload.UnitID,
	).Scan(&u.id, &u.ownerID, &u.utype, &u.category, &u.size, &u.crew, &u.cargoUnitID,
		&u.status, &u.q, &u.r, &u.targetQ, &u.targetR, &u.stance, &u.marchIntent, &u.colonyName, &u.homeSettlementID, &u.captureMode,
		&u.carriedSilver, &u.provisions); err != nil {
		if err == pgx.ErrNoRows {
			return nil // unit gone (disbanded/destroyed) — nothing to return
		}
		return fmt.Errorf("load sentry unit: %w", err)
	}

	// Idempotent: only a unit still holding its sentry patrol turns home here.
	if u.status != "positioned" || u.stance == nil || *u.stance != "sentry" || u.homeSettlementID == nil {
		return tx.Commit(ctx)
	}

	if err := h.dispatchReturnHome(ctx, tx, u, u.q, u.r, payload.WorldID, returnReasonExplore); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// foundColony establishes a new colony settlement at an empty destination hex.
// The arriving unit disbands into the colony's founding populace (colonists
// become citizens, not a garrison — an undefended new colony is the intended
// cost of expansion). This is the discrete-unit equivalent of the legacy
// ArmyComposition colonize() in arrival.go: a genuinely separate
// settlement (own catchment, loyalty, governor, building queue) that is still
// integrated into the Wanax's network (same owner, shares the per-Wanax kharis
// pool, revolts if the capital falls, counts toward the divine expansion brake).
//
// existingProvinceID is uuid.Nil when no provinces row exists for the hex yet
// (the common case — provinces are sparse); then we create one from map_tiles.
// If a row already exists (e.g. a prior outpost province) we reuse it.
func (h *UnitArrivalHandler) foundColony(
	ctx context.Context, tx pgx.Tx,
	u unitRow, existingProvinceID uuid.UUID, destQ, destR int, worldID uuid.UUID,
) error {
	// Owner's culture + parent settlement come from their capital (fallback: any
	// of their settlements). The parent is recorded as founded_from (lineage).
	var culture string
	var parentID uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT id, culture_id FROM settlements WHERE owner_id = $1
		 ORDER BY is_capital DESC LIMIT 1`,
		u.ownerID,
	).Scan(&parentID, &culture); err != nil {
		return fmt.Errorf("foundColony: load owner capital: %w", err)
	}

	// Ensure a province row exists for the hex, with deposit flags copied from the map.
	provinceID := existingProvinceID
	if provinceID == uuid.Nil {
		var terrain string
		var copperDep, tinDep, silverDep, cedarDep, coastal bool
		if err := tx.QueryRow(ctx,
			`SELECT terrain, copper_deposit, tin_deposit,
			        COALESCE(silver_deposit,false), COALESCE(cedar_deposit,false), COALESCE(coastal,false)
			 FROM map_tiles WHERE world_id = $1 AND q = $2 AND r = $3`,
			worldID, destQ, destR,
		).Scan(&terrain, &copperDep, &tinDep, &silverDep, &cedarDep, &coastal); err != nil {
			return fmt.Errorf("foundColony: load map tile: %w", err)
		}
		if err := tx.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, territory_state,
			                        copper_deposit, tin_deposit, silver_deposit, cedar_deposit, coastal)
			 VALUES ($1,$2,$3,$4,'controlled',$5,$6,$7,$8,$9) RETURNING id`,
			worldID, destQ, destR, terrain, copperDep, tinDep, silverDep, cedarDep, coastal,
		).Scan(&provinceID); err != nil {
			return fmt.Errorf("foundColony: create province: %w", err)
		}
	} else {
		_, _ = tx.Exec(ctx,
			`UPDATE provinces SET territory_state='controlled' WHERE id=$1`, provinceID)
	}

	// Colony name: chosen, else culture-appropriate default. Both go through the
	// world's taken-name set — march_start already refused a chosen duplicate, but
	// the name can be claimed while the colonists are on the road, and a founding
	// must never fail over a name: the generator takes over instead.
	name := province.UniqueSettlementName(ctx, tx, worldID, culture)
	if u.colonyName != nil && *u.colonyName != "" {
		if taken, nErr := province.SettlementNameIsTaken(ctx, tx, worldID, *u.colonyName); nErr != nil || !taken {
			name = *u.colonyName
		}
	}

	// Create the colony. Baseline population (economy.ColonyBaseFoundingPopulation,
	// currently 1500 — a real but modest second city) plus the colonizing unit's
	// own size, since its colonists join the founding populace (they become
	// citizens, not a garrison; see below). Unlike the capital the colony is NOT
	// guaranteed self-sufficient (it can starve if neglected); that asymmetry is
	// the intended cost of expansion.
	population := economy.ColonyBaseFoundingPopulation + u.size
	var colonyID uuid.UUID
	if err := tx.QueryRow(ctx,
		`INSERT INTO settlements
		 (world_id, province_id, name, culture_id, owner_id, control_type, is_capital,
		  loyalty, loyalty_points, governor_is_ai, population, founded_from)
		 VALUES ($1,$2,$3,$4,$5,'colony',false,2,$8,true,$7,$6)
		 RETURNING id`,
		worldID, provinceID, name, culture, u.ownerID, parentID, population, loyalty.LoyaltyStartColony,
	).Scan(&colonyID); err != nil {
		return fmt.Errorf("foundColony: create settlement: %w", err)
	}

	// No Sitos seed. The fund this used to sow was silver minted out of nothing
	// at every founding — pop × 10.5, so a ~2000-pop colony printed ~21 000 into
	// a world whose entire liquid supply measured 106 678. Migration 106 replaced
	// the fund with a granary, and the granary starts EMPTY: a new colony has no
	// reserve because it has not had a harvest yet. It earns one by having a
	// surplus.

	// Link province back to its controlling settlement.
	_, _ = tx.Exec(ctx, `UPDATE provinces SET controller_id=$1 WHERE id=$2`, colonyID, provinceID)

	// Seed a zero/baseline row for every good (mirrors join.go), then let
	// RecomputeProduction write real rates from the catchment + labor weights.
	if _, err := tx.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 SELECT $1, g.key,
		        -- Startlagret (megaron_plan_byggkostnader steg 3): inget timmer och
		        -- ingen sten. Grain kommer från economy.ColonyGrainSeed och MÅSTE
		        -- castas till numeric, inte int — ::int trunkerade decimaler tyst.
		        -- En kopia finns i api/handlers/create_metropolis.go.
		        -- Livestock är ett antal djur, inte en matmängd, och skalas inte.
		        CASE g.key
		            WHEN 'grain'     THEN $2::numeric
		            WHEN 'timber'    THEN 0
		            WHEN 'stone'     THEN 0
		            WHEN 'livestock' THEN $3::numeric
		            ELSE 0
		        END,
		        0,
		        $4, -- shared non-binding storage ceiling (also used by
		                 -- create_metropolis.go). The per-good CASE that stood here
		                 -- predated the 2026-07-05 cap loosening (fc8d424) and was the
		                 -- one seed site that sweep missed: every colony was founded
		                 -- with cedar/timber capped at 500, ore at 300 and craft goods
		                 -- at 200, so a colony on a forest hex pegged its cedar store
		                 -- within eight ticks and burned the rest of its production
		                 -- forever. Capitals never had this. Silver's real cap is set
		                 -- by the Sitos liquid-silver seed below, same as for capitals.
		        current_world_tick()
		 FROM goods g
		 ON CONFLICT (settlement_id, good_key) DO NOTHING`,
		colonyID, economy.ColonyGrainSeed, economy.FoundingHerdLivestock, province.DefaultGoodStorageCap,
	); err != nil {
		return fmt.Errorf("foundColony: seed goods: %w", err)
	}

	// The colony's starting silver is the purse the colonists CARRIED here
	// (mig 107), not silver minted at the moment of founding. Until 2026-08-03
	// this line called GenesisSilverLiquid and created ~10 500 silver per colony
	// out of nothing, in a world holding 106 678 liquid — expansion was a
	// printing press, and a Wanax could always found their way out of
	// insolvency. B3: silver enters only via genesis and mines.
	//
	// The cap still comes from GenesisSilverLiquid: a cap is a shape, not silver,
	// and the colony needs the same headroom a capital gets or it would clip its
	// own income later. If the expedition arrived empty-handed the colony starts
	// at 0 — poor, unable to pay upkeep, and that is a real consequence of
	// sending it out of a treasury that had nothing to give.
	if grainBaseValue, gbErr := economy.GoodBaseValue(ctx, tx, "grain"); gbErr != nil {
		slog.Error("colony silver: load grain base value", "err", gbErr)
	} else {
		_, liquidCap := economy.GenesisSilverLiquid(population, grainBaseValue, h.sitosCfg)
		if _, err := tx.Exec(ctx,
			`UPDATE settlement_goods SET amount = $1, cap = $2, calc_tick = current_world_tick()
			 WHERE settlement_id = $3 AND good_key = 'silver'`,
			u.carriedSilver, liquidCap, colonyID,
		); err != nil {
			slog.Error("colony silver: credit carried purse failed", "err", err, "settlement", colonyID)
		} else if u.carriedSilver > 0 {
			// The purse is spent the moment it lands. Zeroing it in the same
			// transaction is what keeps the silver from existing twice if the
			// unit row outlives the founding for any reason.
			if _, err := tx.Exec(ctx,
				`UPDATE units SET carried_silver = 0 WHERE id = $1`, u.id,
			); err != nil {
				return fmt.Errorf("foundColony: clear carried purse: %w", err)
			}
		}
	}

	// Cult floor keeps a temple (once built) non-inert — cult labor is a
	// separate path from P4's placement model (megaron_cult_ar_ingen_vara_plan.md),
	// untouched by the grain seed it used to sit alongside.
	if _, err := tx.Exec(ctx,
		`INSERT INTO settlement_labor (settlement_id, good_key, weight)
		 VALUES ($1,'cult',0.15)
		 ON CONFLICT (settlement_id, good_key) DO NOTHING`,
		colonyID,
	); err != nil {
		return fmt.Errorf("foundColony: seed labor: %w", err)
	}

	// P4 (megaron_plan_fysisk_gubbemodell.md): auto-place the starting gubbar on
	// the best available food hexes, greedily, stopping once the colony is
	// self-sufficient — see economy.PlaceStartingWorkforce's doc comment.
	// Whatever gubbar aren't needed for food stay unplaced for the Wanax to
	// assign, e.g. toward ore once a mine is built on the deposit colonized for.
	if _, _, err := economy.PlaceStartingWorkforce(ctx, tx, colonyID); err != nil {
		return fmt.Errorf("foundColony: place starting workforce: %w", err)
	}

	if err := economy.RecomputeProduction(ctx, tx, colonyID); err != nil {
		return fmt.Errorf("foundColony: recompute production: %w", err)
	}

	// Disband the colonizing unit into the colony's populace — colonists become
	// citizens, not a garrison (their headcount is already folded into
	// `population` above). No garrison remains: a new colony is undefended by
	// design. Clears march + intent fields the same way arriveGarrison would.
	if _, err := tx.Exec(ctx,
		`UPDATE units SET
		   status        = 'disbanded',
		   settlement_id = $2,
		   target_q      = NULL,
		   target_r      = NULL,
		   departs_at    = NULL,
		   arrives_at    = NULL,
		   depart_tick   = NULL,
		   arrive_tick   = NULL,
		   march_intent  = NULL,
		   colony_name   = NULL,
		   updated_at    = now()
		 WHERE id = $1`,
		u.id, colonyID,
	); err != nil {
		return fmt.Errorf("foundColony: disband colonizing unit: %w", err)
	}

	_, _ = h.eventStore.Append(ctx, u.id, events.StreamType(unit.StreamUnit), unit.EventUnitArrived,
		unit.UnitArrivedPayload{UnitID: u.id, Q: destQ, R: destR, NewStatus: "disbanded"}, worldID, nil)

	// Rumor: a new colony nearby is news — minor, witnessed only by nearby owners
	// (temenos_gossip.md PASS 2b). Subject = the colony itself, so it registers as
	// rumour-known for anyone who hears of it without having seen it. Best-effort —
	// never fail colonization over gossip.
	if err := gossip.Broadcast(ctx, tx, worldID, colonyID, "political",
		name+" has been founded nearby.", 6,
		gossip.ImportanceMinor, colonyID, ""); err != nil {
		slog.Warn("foundColony: broadcast gossip", "colony", colonyID, "err", err)
	}

	if h.hub != nil {
		// DEL B (megaron_koloni_legibilitet_plan.md): carry the colony's founding
		// grain balance additively in the payload so the notification can warn that
		// the colony starts draining its seed THIS tick if net < 0. RecomputeProduction
		// above already wrote the real production rate into settlement_goods (same
		// TX), so read it back here rather than re-deriving it. Best-effort: a
		// missing row just leaves the fields at zero. Old payload fields are
		// unchanged (web stays back-compatible — see the plan's web note).
		//
		// Since Utfodringsordningen D1 (megaron_plan_utfodringsordningen.md,
		// 2026-08-26) the stored rate is RAW production, not net — economy.
		// GrainBalance (D6) turns it back into a net figure using the colony's own
		// population, which was already loaded above. Reading `rate` straight as
		// "net" would report gross production and never warn of a deficit, since
		// D1 means this rate can no longer go negative on its own.
		var grainAmount, grainRate float64
		_ = tx.QueryRow(ctx,
			`SELECT amount, rate FROM settlement_goods
			 WHERE settlement_id = $1 AND good_key = 'grain'`,
			colonyID,
		).Scan(&grainAmount, &grainRate)
		_, grainNet := economy.GrainBalance(grainRate, population)

		payload := map[string]any{
			"settlement_id":      colonyID,
			"name":               name,
			"province_id":        provinceID,
			"q":                  destQ,
			"r":                  destR,
			"grain_amount":       grainAmount,
			"grain_net_per_tick": grainNet,
		}
		// grain_ticks: how long the seed lasts at the current deficit; null
		// (omitted) when the colony is self-sustaining (net ≥ 0). This payload is
		// persisted (NotifyPlayer writes to `notifications`), so grain_days is
		// kept alongside it with the same value — old rows already carry
		// grain_days and readers fall back to it. Don't remove grain_days.
		if grainNet < 0 {
			if tickDrain := -grainNet; tickDrain > 0 {
				ticksLeft := grainAmount / tickDrain
				payload["grain_ticks"] = ticksLeft
				payload["grain_days"] = ticksLeft
			}
		}
		_ = h.hub.NotifyPlayer(ctx, worldID, u.ownerID, "ColonyFounded", 3, payload)
	}

	slog.Info("colony founded (discrete unit)", "settlement", colonyID, "name", name,
		"province", provinceID, "owner", u.ownerID, "founding_unit", u.id, "population", population, "q", destQ, "r", destR)
	return nil
}

// resolveCombat handles the arriving unit attacking an enemy settlement.
//
// KR3 cutover (megaron_plan_kr3_stridssystem.md §8, mirrors resolveFieldCombat
// in unit_arrival_field.go — the pattern this was cut over to match): this no
// longer resolves the fight itself with a one-shot strength/fortune roll. It
// only creates/joins a persistent battles row via initiateOrJoinBattle, with
// EVERY garrison unit sent in as its own defender participant (multi-garrison
// was never a blocker — battle.go already carries N participants per side).
// Actual dice rolling, wall absorption (§8 beslut 7) and loss application
// happen later, in BattleTickHandler. The old one-shot strength/fortune/wall
// path (resolver.go's Resolve family, fortune.go, and the applyAttackerWins/
// applyDefenderWins/unitStrength/sack methods) was removed once every entry
// point had cut over to initiateOrJoinBattle — see battle.go's header comment
// for the resulting capture gap.
//
// Idempotency: u.status == 'marching' was checked at top; all writes are
// conditional on status or use ON CONFLICT DO NOTHING.
func (h *UnitArrivalHandler) resolveCombat(
	ctx context.Context, tx pgx.Tx,
	u unitRow, dest destSettlement, destQ, destR int, worldID uuid.UUID,
) error {
	arriving := battleParticipant{unitID: u.id, ownerID: u.ownerID, utype: u.utype, side: "attacker", currentSize: u.size, stance: u.stance}

	garrisonRows, err := tx.Query(ctx,
		`SELECT id, owner_id, type, size FROM units
		 WHERE settlement_id = $1 AND status = 'garrison' AND status != 'disbanded'`,
		*dest.settlementID,
	)
	if err != nil {
		return fmt.Errorf("settlement combat: load garrison: %w", err)
	}
	var defenders []battleParticipant
	for garrisonRows.Next() {
		var d battleParticipant
		d.side = "defender"
		if scanErr := garrisonRows.Scan(&d.unitID, &d.ownerID, &d.utype, &d.currentSize); scanErr != nil {
			garrisonRows.Close()
			return fmt.Errorf("settlement combat: scan garrison: %w", scanErr)
		}
		defenders = append(defenders, d)
	}
	garrisonRows.Close()
	if err := garrisonRows.Err(); err != nil {
		return fmt.Errorf("settlement combat: garrison rows: %w", err)
	}

	if err := h.initiateOrJoinBattle(ctx, tx, worldID, destQ, destR, arriving, defenders); err != nil {
		return fmt.Errorf("settlement combat: initiate/join battle: %w", err)
	}

	slog.Info("settlement combat: battle initiated/joined", "unit", u.id, "settlement", *dest.settlementID, "q", destQ, "r", destR, "defenders", len(defenders))

	// The besieging unit holds the contested hex outside the settlement while
	// the battle resolves over subsequent battle-ticks — mirrors
	// resolveFieldCombat: no immediate win/lose branch here anymore.
	if _, err := tx.Exec(ctx,
		`UPDATE units SET
		   status        = 'positioned',
		   q             = $2,
		   r             = $3,
		   settlement_id = NULL,
		   target_q      = NULL,
		   target_r      = NULL,
		   departs_at    = NULL,
		   arrives_at    = NULL,
		   depart_tick   = NULL,
		   arrive_tick   = NULL,
		   updated_at    = now()
		 WHERE id = $1`,
		u.id, destQ, destR,
	); err != nil {
		return fmt.Errorf("settlement combat: position besieging unit: %w", err)
	}

	return nil
}

// resolveAmphibiousAssault handles a laden galley arriving at the sea hex next
// to an enemy coastal settlement. The ship cannot enter land, so the CARGO
// land unit does the fighting (not the galley) — it disembarks immediately
// and holds the settlement's hex.
//
// KR3 cutover (megaron_plan_kr3_stridssystem.md §8, mirrors resolveCombat/
// resolveFieldCombat): this no longer resolves the fight itself. It creates/
// joins a persistent battles row via initiateOrJoinBattle — the cargo as the
// attacker participant, every garrison unit as its own defender participant
// — then lets ScheduledBattleTick resolve it. Ship/cargo handling (disembark,
// empty the galley) is kept exactly as before; only the COMBAT OUTCOME moved.
// storm is read from the SHIP's stance (u.stance), same source the old model
// used here, even though the cargo is the one that fights (see
// battleParticipant.stance's doc comment). No capture on a win — see
// battle.go's header comment for that gap, shared with resolveCombat.
func (h *UnitArrivalHandler) resolveAmphibiousAssault(
	ctx context.Context, tx pgx.Tx, u unitRow, seaQ, seaR int, worldID uuid.UUID,
) error {
	// sailHomeOrGarrison is R5's return leg (megaron_plan_skeppsuppdrag_
	// landsatt.md) — the ship sails home on its own rather than sitting at the
	// beach forever. Defensive fallback to the old "garrison at sea" outcome
	// for a unit somehow missing home_settlement_id (a real dispatch always
	// sets it, mirroring patrol/land above) — same posture as exploreArrived's
	// own nil guard: never strand a unit with nothing to return to.
	sailHomeOrGarrison := func(q, r int) error {
		if u.homeSettlementID == nil {
			slog.Warn("amphibious assault: unit has no home_settlement_id, garrisoning at sea instead of returning", "unit", u.id)
			return h.arriveGarrison(ctx, tx, u, q, r, nil, worldID)
		}
		return h.dispatchReturnHome(ctx, tx, u, q, r, worldID, returnReasonExplore)
	}

	// No cargo → nothing to land.
	if u.cargoUnitID == nil {
		return sailHomeOrGarrison(seaQ, seaR)
	}

	// Find an enemy (non-owned) coastal settlement adjacent to the landing hex.
	var dest destSettlement
	var settleQ, settleR int
	if err := tx.QueryRow(ctx,
		`SELECT s.id, s.owner_id, s.wall_level, p.id, p.terrain_type, p.map_q, p.map_r
		 FROM provinces p
		 JOIN settlements s ON s.province_id = p.id
		 WHERE p.world_id = $1 AND s.state = 'active' AND s.owner_id <> $2
		   AND COALESCE(p.coastal, false) = true
		   AND (ABS(p.map_q - $3) + ABS(p.map_r - $4) +
		        ABS((p.map_q + p.map_r) - ($3 + $4))) / 2 = 1
		 ORDER BY s.id
		 LIMIT 1`,
		worldID, u.ownerID, seaQ, seaR,
	).Scan(&dest.settlementID, &dest.ownerID, &dest.wallLevel,
		&dest.provinceID, &dest.terrain, &settleQ, &settleR); err != nil {
		// The target vanished (captured, abandoned, or ownership changed) before
		// the landing. Nothing to storm — the ship (and its still-embarked cargo)
		// sails home instead of sitting at the beach (R5).
		slog.Info("amphibious assault: no adjacent enemy coastal settlement, returning home",
			"ship", u.id, "q", seaQ, "r", seaR)
		return sailHomeOrGarrison(seaQ, seaR)
	}

	// Load the cargo land unit — it is the real attacker.
	cargoID := *u.cargoUnitID
	var cargoType string
	var cargoSize int
	if err := tx.QueryRow(ctx,
		`SELECT type, size FROM units WHERE id = $1`, cargoID,
	).Scan(&cargoType, &cargoSize); err != nil {
		return fmt.Errorf("amphibious assault: load cargo: %w", err)
	}

	arriving := battleParticipant{unitID: cargoID, ownerID: u.ownerID, utype: cargoType, side: "attacker", currentSize: cargoSize, stance: u.stance}

	garrisonRows, err := tx.Query(ctx,
		`SELECT id, owner_id, type, size FROM units
		 WHERE settlement_id = $1 AND status = 'garrison' AND status != 'disbanded'`,
		*dest.settlementID,
	)
	if err != nil {
		return fmt.Errorf("amphibious assault: load garrison: %w", err)
	}
	var defenders []battleParticipant
	for garrisonRows.Next() {
		var d battleParticipant
		d.side = "defender"
		if scanErr := garrisonRows.Scan(&d.unitID, &d.ownerID, &d.utype, &d.currentSize); scanErr != nil {
			garrisonRows.Close()
			return fmt.Errorf("amphibious assault: scan garrison: %w", scanErr)
		}
		defenders = append(defenders, d)
	}
	garrisonRows.Close()
	if err := garrisonRows.Err(); err != nil {
		return fmt.Errorf("amphibious assault: garrison rows: %w", err)
	}

	if err := h.initiateOrJoinBattle(ctx, tx, worldID, settleQ, settleR, arriving, defenders); err != nil {
		return fmt.Errorf("amphibious assault: initiate/join battle: %w", err)
	}

	slog.Info("amphibious assault: battle initiated/joined", "ship", u.id, "cargo", cargoID, "settlement", *dest.settlementID, "defenders", len(defenders))

	// The cargo storms ashore and holds the settlement's hex while the battle
	// resolves over subsequent battle-ticks — no immediate win/lose branch
	// here anymore.
	if _, err := tx.Exec(ctx,
		`UPDATE units SET
		   status = 'positioned', settlement_id = NULL,
		   q = $2, r = $3, target_q = NULL, target_r = NULL,
		   departs_at = NULL, arrives_at = NULL, depart_tick = NULL, arrive_tick = NULL, updated_at = now()
		 WHERE id = $1`,
		cargoID, settleQ, settleR,
	); err != nil {
		return fmt.Errorf("amphibious assault: position landed cargo: %w", err)
	}

	// The galley's part is done the instant its cargo lands — it empties and
	// sails home on its own (R5, megaron_plan_skeppsuppdrag_landsatt.md)
	// regardless of how the fight ashore eventually goes (ship/cargo landing
	// handling kept from the old model; only the "sits at the beach forever"
	// outcome is gone).
	if _, err := tx.Exec(ctx,
		`UPDATE units SET cargo_unit_id = NULL, q = $2, r = $3, updated_at = now() WHERE id = $1`,
		u.id, seaQ, seaR,
	); err != nil {
		return fmt.Errorf("amphibious assault: empty galley: %w", err)
	}

	return sailHomeOrGarrison(seaQ, seaR)
}

// ── Internal types ─────────────────────────────────────────────────────────────

type unitRow struct {
	id               uuid.UUID
	ownerID          uuid.UUID
	utype            string
	category         string
	size             int
	crew             int
	cargoUnitID      *uuid.UUID
	status           string
	q                int // origin hex (set by march handler; used for rout routing)
	r                int
	targetQ          *int
	targetR          *int
	stance           *string    // C5: fortify/storm/sentry or nil
	marchIntent      *string    // "colonize" | "explore" | "explore_return" | "land" | nil (plain march)
	colonyName       *string    // chosen colony name or nil
	homeSettlementID *uuid.UUID // set for "explore"/"explore_return"/"land"; the settlement to return to
	captureMode      string     // "sack" (default) | "annex" — set at march dispatch, read on conquest
	// landTargetQ/R + landCargoIntent (mig 147, megaron_plan_skeppsuppdrag_
	// landsatt.md R1): set only for marchIntent == "land" — the CHOSEN land
	// hex (this ship's own q/r,target_q/target_r are the sea waypoint it
	// actually sails to) and the optional grounding order for the cargo it
	// puts ashore there ("" or "colonize").
	landTargetQ     *int
	landTargetR     *int
	landCargoIntent *string
	// passageMessengerID (mig 149, megaron_plan_ordna_passage.md 3b-3): set
	// only for marchIntent == "passage"/"passage_wait" — the messenger this
	// ship was arranged to carry (outbound) or fetch (pickup). Read at arrival
	// (passageArrived) and by the passage scan's release phase.
	passageMessengerID *uuid.UUID
	// pickupUnitID/pickupWaitTicks (mig 152, megaron_plan_hamta_hem.md, slice
	// 2b): set only for marchIntent == "pickup"/"pickup_wait" — the unit this
	// ship was sent to fetch, and how many ticks it waits off the shore for
	// that unit before giving up and turning for home empty (HandlePickupTimeout).
	pickupUnitID    *uuid.UUID
	pickupWaitTicks *int
	// carriedSilver is the colonist purse (mig 107): silver debited from the
	// mother city at dispatch and riding on this unit. Credited to the colony it
	// founds, or back into whatever settlement it walks into if it turns around.
	carriedSilver float64
	// provisions är skeppets matlager (mig 133): korn draget ur hemstaden vid
	// avfärd, ätet under resan, och avlastat igen när skeppet når hamn.
	provisions float64
}

type destSettlement struct {
	provinceID   uuid.UUID
	settlementID *uuid.UUID
	ownerID      *uuid.UUID
	wallLevel    int
	terrain      string
}
