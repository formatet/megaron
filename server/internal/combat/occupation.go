package combat

// KR3 erövringens efterspel (megaron_plan_erovring.md). Fills the cutover-mur
// gap flagged in battle.go's header comment: a besieging force that wins can
// now actually TAKE the city, not just annihilate/rout its garrison.
//
// Model (Timothy 2026-08-07, PO1/PO2 megaron_valtabell.md):
//   - S1 (this file, called from battle.go's resolveTick): a battle against a
//     settlement that ends with the attacker winning does NOT annex directly —
//     it puts the city into an OCCUPIED holding state under the winning army,
//     ownership unchanged. termination_reason becomes "attacker_reached_city"
//     instead of "annihilation"/"rout" for this one case.
//   - S2: occupied_since_tick is a counter that matures into an annex offer
//     after occupationTicksToAnnex UNCHALLENGED ticks. Any new attack against
//     the occupied city — win OR lose for the occupant — resets it (the
//     contestable async window: a relief force has time to act). An occupied
//     city produces/pays nothing for the occupant (no code touches its
//     production while state='occupied' — RecomputeProduction is never called
//     for it in that state, so its rates stay exactly what they were the
//     instant it fell, until sack/burn/annex changes them).
//   - S3/S4/S5 (ExecuteOccupyAction, reached via ScheduledOrderDelivery's
//     "occupy_action" verb — command is never instant, even for an army
//     already standing in the city): sack (loot + pop -⅓ + top production
//     building -1 level, city stays with the defender, no ownership change),
//     burn (sack's loot step + raze, ownerless, blocked from recolonization
//     for burnRecolonizeKaren ticks instead of the old permanent block), or
//     annex (ownership transfers — only once the occupation counter has
//     matured).
//
// NOT reconciled here (flagged, not decided): units.capture_mode ("sack"|
// "annex", set at march dispatch via keryx --mode) is left completely
// unread by this file. The plan's S3 line — "Reconcilera med keryx --mode
// sack|annex (blir default som notisvalet överrider)" — sits in real tension
// with S3's headline rule "default = occupy om spelaren inte svarar":
// capture_mode defaults to "sack" for EVERY march that doesn't set --mode
// explicitly (march_start.go), so treating it as an auto-trigger would sack
// nearly every conquered city with no player action at all, the opposite of
// "default=occupy". Auto-annex-on-maturity for an explicit --mode annex would
// be a defensible reading, but is a product call, not an engineering one —
// left for Timothy/the planner rather than guessed at here.
//
// Old applyAttackerWins/applyDefenderWins/sackSettlement (unit_arrival.go,
// sack.go) are UNCHANGED — still dead from any production entry point (see
// battle.go's header), left in place per the plan ("radera inte förrän
// ersatt"). This file does not import from or call into them; the loot-
// manifest formula is intentionally re-derived here rather than shared, so
// touching one never risks the other's still-passing direct-call unit tests.

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/gossip"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/transport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Rattar (kalibrering, INTE lås — megaron_plan_erovring.md §Rattar) ───────

const (
	// occupationTicksToAnnex: how many UNCHALLENGED ticks an occupation must
	// stand before an annex offer fires (~1 in-game month, strawman 30).
	// ⚠️ per the plan: this MUST exceed a plausible relief march's travel time
	// or the annex becomes uncontestable (asynkronitetsgrinden fails) —
	// unvalidated against real march times this slice, flagged in the report.
	occupationTicksToAnnex = 30
	// sackPopLossFraction: population lost when a held-but-not-annexed city is
	// sacked (strawman ⅓).
	sackPopLossFraction = 1.0 / 3.0
	// burnRecolonizeKaren: ticks a burned settlement's hex is blocked from
	// colonize-in-place (~2 in-game months, strawman 60) — replaces the old
	// sackSettlement's PERMANENT block for this path only.
	burnRecolonizeKaren = 60
)

// ── Frozen event family (additive — see file header) ────────────────────────

const (
	EventSettlementOccupied = "SettlementOccupied"
	EventSettlementLooted   = "SettlementLooted" // sack, no ownership change
	EventSettlementBurned   = "SettlementBurned" // sack-and-burn, razed
)

// SettlementOccupiedPayload is emitted once, the instant a battle against a
// settlement ends with the attacker winning (S1). Not "SettlementCaptured" —
// ownership has NOT changed; that event is reserved for the eventual annex
// (executeAnnex reuses SettlementCaptured, same true fact as the old one-shot
// annex: ownership transferred).
type SettlementOccupiedPayload struct {
	SettlementID  uuid.UUID  `json:"settlement_id"`
	WorldID       uuid.UUID  `json:"world_id"`
	OccupantID    uuid.UUID  `json:"occupant_id"`
	FormerOwnerID *uuid.UUID `json:"former_owner_id,omitempty"`
	Q             int        `json:"q"`
	R             int        `json:"r"`
	OccupiedTick  int        `json:"occupied_tick"`
}

// SettlementLootedPayload / SettlementBurnedPayload record the OUTCOME of a
// resolved sack/burn choice (events store outcomes, never intentions).
type SettlementLootedPayload struct {
	SettlementID uuid.UUID          `json:"settlement_id"`
	WorldID      uuid.UUID          `json:"world_id"`
	RaiderID     uuid.UUID          `json:"raider_id"`
	FormerOwner  *uuid.UUID         `json:"former_owner,omitempty"`
	Looted       transport.Manifest `json:"looted"`
	PopLost      int                `json:"pop_lost"`
	BuildingHit  string             `json:"building_hit,omitempty"`
}

type SettlementBurnedPayload struct {
	SettlementID           uuid.UUID          `json:"settlement_id"`
	WorldID                uuid.UUID          `json:"world_id"`
	RaiderID               uuid.UUID          `json:"raider_id"`
	FormerOwner            *uuid.UUID         `json:"former_owner,omitempty"`
	Looted                 transport.Manifest `json:"looted"`
	RecolonizableAfterTick int                `json:"recolonizable_after_tick"`
}

// ── S1/S2: battle-end hook, called from battle.go's resolveTick ────────────

// citySettlement is the settlement (if any) sitting at a battle's hex,
// loaded FOR UPDATE so occupySettlement/resetOccupationDefense can safely
// read-then-write it in the same transaction as the rest of the battle-tick.
type citySettlement struct {
	id                uuid.UUID
	ownerID           *uuid.UUID
	state             string
	occupantID        *uuid.UUID
	occupiedSinceTick *int
	provinceID        uuid.UUID
	name              string
}

// loadCitySettlementForUpdate returns nil, nil when no settlement sits at
// (q,r) — the common case for field battles and interception, which this
// whole file is a no-op for.
func loadCitySettlementForUpdate(ctx context.Context, tx pgx.Tx, worldID uuid.UUID, q, r int) (*citySettlement, error) {
	var cs citySettlement
	err := tx.QueryRow(ctx,
		`SELECT s.id, s.owner_id, s.state, s.occupant_id, s.occupied_since_tick, s.province_id, s.name
		 FROM settlements s JOIN provinces p ON p.id = s.province_id
		 WHERE p.world_id = $1 AND p.map_q = $2 AND p.map_r = $3
		 FOR UPDATE OF s`,
		worldID, q, r,
	).Scan(&cs.id, &cs.ownerID, &cs.state, &cs.occupantID, &cs.occupiedSinceTick, &cs.provinceID, &cs.name)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load city settlement: %w", err)
	}
	return &cs, nil
}

// occupySettlement is S1's hook: the attacker just won a battle whose
// defending side was this settlement's garrison (fresh conquest, OR a
// relief force retaking an already-occupied city from its current
// occupant — both look identical from here: a new army just won at this
// hex, so it becomes the new occupant, counter reset to now). Called from
// battle.go's resolveTick with the tx still open.
//
// survivingAttackerUnitIDs are the winning side's participants with
// sizes[i] > 0 at battle end — they become the occupying garrison.
// evictStaleGarrison disbands any unit still marked 'garrison' at
// settlementID that isn't owned by the new occupant. Shared by
// occupySettlement (battle-won occupation) and capitulateSettlement
// (siege-starvation occupation, siege_capitulation.go) — both hand a city to
// a new occupant without necessarily moving any of the occupant's own units
// in, and either way a stale defender garrison cannot be left standing.
//
// For occupySettlement specifically: the straightforward case (defender
// wiped) already disbanded every defender unit in resolveTick's
// apply-final-sizes loop, so this is a no-op there. It only bites when the
// defender ROUTED instead: §5's markSideRouted (battle.go) deliberately
// leaves a routed side's survivors' status/settlement_id untouched (their
// own doc comment) — a pre-existing gap in the rout mechanic, not this
// slice's to fix, but an occupied city cannot be left showing a live enemy
// garrison.
func evictStaleGarrison(ctx context.Context, tx pgx.Tx, settlementID, occupantOwnerID uuid.UUID) error {
	if _, err := tx.Exec(ctx,
		`UPDATE units SET status = 'disbanded', size = 0, updated_at = now()
		 WHERE settlement_id = $1 AND status = 'garrison' AND owner_id IS DISTINCT FROM $2`,
		settlementID, occupantOwnerID,
	); err != nil {
		return fmt.Errorf("evict stale garrison: %w", err)
	}
	return nil
}

func (h *BattleTickHandler) occupySettlement(
	ctx context.Context, tx pgx.Tx, worldID uuid.UUID, q, r, tickIndex int,
	city *citySettlement, occupantOwnerID uuid.UUID, survivingAttackerUnitIDs []uuid.UUID,
) error {
	if err := evictStaleGarrison(ctx, tx, city.id, occupantOwnerID); err != nil {
		return err
	}

	if len(survivingAttackerUnitIDs) > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE units SET
			   status        = 'garrison',
			   settlement_id = $1,
			   q             = $2,
			   r             = $3,
			   target_q      = NULL,
			   target_r      = NULL,
			   departs_at    = NULL,
			   arrives_at    = NULL,
			   depart_tick   = NULL,
			   arrive_tick   = NULL,
			   updated_at    = now()
			 WHERE id = ANY($4)`,
			city.id, q, r, survivingAttackerUnitIDs,
		); err != nil {
			return fmt.Errorf("occupy: garrison winning attacker: %w", err)
		}
	}

	if _, err := tx.Exec(ctx,
		`UPDATE settlements SET
		   state = 'occupied', occupant_id = $2, occupied_since_tick = $3,
		   annex_ready_notified = false, updated_at = now()
		 WHERE id = $1`,
		city.id, occupantOwnerID, tickIndex,
	); err != nil {
		return fmt.Errorf("occupy: mark settlement occupied: %w", err)
	}

	_, _ = h.eventStore.Append(ctx, city.id, events.StreamProvince, EventSettlementOccupied,
		SettlementOccupiedPayload{
			SettlementID: city.id, WorldID: worldID, OccupantID: occupantOwnerID,
			FormerOwnerID: city.ownerID, Q: q, R: r, OccupiedTick: tickIndex,
		}, worldID, nil)

	if err := scheduleOccupationCheck(ctx, tx, h.scheduler, worldID, city.id, tickIndex); err != nil {
		return err
	}

	if err := gossip.Broadcast(ctx, tx, worldID, city.id, "military",
		city.name+" has fallen under occupation.", 6, gossip.ImportanceMajor, city.id, ""); err != nil {
		slog.Warn("occupy: broadcast gossip", "settlement", city.id, "err", err)
	}

	if h.hub != nil {
		if city.ownerID != nil {
			_ = h.hub.NotifyPlayer(ctx, worldID, *city.ownerID, "CityOccupied", 1, map[string]any{
				"settlement_id": city.id, "name": city.name, "role": "defender",
				"occupation_ticks_to_annex": occupationTicksToAnnex,
			})
		}
		_ = h.hub.NotifyPlayer(ctx, worldID, occupantOwnerID, "CityOccupied", 1, map[string]any{
			"settlement_id": city.id, "name": city.name, "role": "attacker",
			"occupation_ticks_to_annex": occupationTicksToAnnex,
			"choices":                   []string{"occupy", "sack", "burn"},
			"default":                   "occupy",
		})
	}
	return nil
}

// resetOccupationDefense is S2's "en attack nollar räknaren": the occupant's
// garrison just DEFENDED the city successfully against a new attack — same
// occupant, counter restarts from this tick.
func (h *BattleTickHandler) resetOccupationDefense(
	ctx context.Context, tx pgx.Tx, worldID uuid.UUID, tickIndex int, city *citySettlement,
) error {
	if _, err := tx.Exec(ctx,
		`UPDATE settlements SET occupied_since_tick = $2, annex_ready_notified = false, updated_at = now()
		 WHERE id = $1 AND state = 'occupied'`,
		city.id, tickIndex,
	); err != nil {
		return fmt.Errorf("occupy: reset defended counter: %w", err)
	}
	if err := scheduleOccupationCheck(ctx, tx, h.scheduler, worldID, city.id, tickIndex); err != nil {
		return err
	}
	if h.hub != nil && city.occupantID != nil {
		_ = h.hub.NotifyPlayer(ctx, worldID, *city.occupantID, "OccupationDefended", 2, map[string]any{
			"settlement_id": city.id, "name": city.name,
			"occupation_ticks_to_annex": occupationTicksToAnnex,
		})
	}
	return nil
}

// scheduleOccupationCheck (re)schedules the ScheduledOccupationCheck for a
// settlement, due when its CURRENT counter would mature. Shared by
// occupySettlement, resetOccupationDefense and the check handler's own
// self-reschedule (occupation_check.go) — always computed fresh from the
// live sinceTick passed in, never cached, so a reset before the old check
// fires is picked up automatically the next time it does.
func scheduleOccupationCheck(ctx context.Context, tx pgx.Tx, sched *events.Scheduler, worldID, settlementID uuid.UUID, sinceTick int) error {
	due := sinceTick + occupationTicksToAnnex
	if err := sched.EnqueueTickTx(ctx, tx, worldID, events.ScheduledOccupationCheck,
		OccupationCheckPayload{SettlementID: settlementID}, due,
	); err != nil {
		return fmt.Errorf("schedule occupation check: %w", err)
	}
	return nil
}

// ── S3/S4/S5: the runner-delivered occupation order ─────────────────────────

// OccupyActionOrder is the ScheduledOrderDelivery "occupy_action" verb
// payload (megaron_plan_erovring.md S3) — what the occupying Wanax chose to
// do with a city they hold, carried by a Runner and executed only on
// delivery ("command is never instant" — no exception even for an army
// already standing in the city, per plan).
type OccupyActionOrder struct {
	WorldID      uuid.UUID
	PlayerID     uuid.UUID
	SettlementID uuid.UUID
	Action       string   // "sack" | "burn" | "annex"
	Goods        []string // sack/burn only; empty = every lootable good (old sackSettlement default)
}

// OccupyActionResult is the accepted outcome (execution succeeded).
type OccupyActionResult struct {
	SettlementID uuid.UUID
	Action       string
}

// ExecuteOccupyAction validates and executes one occupation choice. Shared
// core between the (not-yet-built) HTTP dispatch path and
// messenger.OrderDeliveryHandler, same shape as combat.SetStance/
// combat.ExecuteRecall.
func ExecuteOccupyAction(
	ctx context.Context, pool *pgxpool.Pool, scheduler *events.Scheduler, eventStore *events.Store,
	clk clock.Clock, hub Broadcaster, o OccupyActionOrder,
) (*OccupyActionResult, error) {
	switch o.Action {
	case "sack", "burn", "annex":
	default:
		return nil, reject(http.StatusBadRequest, `invalid action: must be "sack", "burn", or "annex"`)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var ownerID, occupantID *uuid.UUID
	var sinceTick *int
	var state, name string
	var provinceID, worldID uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT owner_id, occupant_id, occupied_since_tick, state, province_id, name, world_id
		 FROM settlements WHERE id = $1 FOR UPDATE`, o.SettlementID,
	).Scan(&ownerID, &occupantID, &sinceTick, &state, &provinceID, &name, &worldID); err != nil {
		return nil, reject(http.StatusNotFound, "settlement not found")
	}
	if worldID != o.WorldID {
		return nil, reject(http.StatusForbidden, "settlement not in this world")
	}
	if state != "occupied" || occupantID == nil || *occupantID != o.PlayerID {
		return nil, reject(http.StatusUnprocessableEntity,
			"you do not currently hold this city under occupation")
	}
	var q, r int
	if err := tx.QueryRow(ctx, `SELECT map_q, map_r FROM provinces WHERE id = $1`, provinceID).Scan(&q, &r); err != nil {
		return nil, fmt.Errorf("execute occupy action: load province: %w", err)
	}

	switch o.Action {
	case "annex":
		var currentTick int
		_ = tx.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
		if sinceTick == nil || currentTick-*sinceTick < occupationTicksToAnnex {
			return nil, reject(http.StatusUnprocessableEntity,
				"the occupation has not gone unchallenged long enough to annex yet")
		}
		if err := executeAnnex(ctx, tx, eventStore, hub, worldID, o.SettlementID, provinceID, name, ownerID, o.PlayerID); err != nil {
			return nil, err
		}
	case "sack":
		if err := executeSack(ctx, tx, scheduler, eventStore, clk, hub, worldID, o.SettlementID, provinceID,
			q, r, name, ownerID, o.PlayerID, o.Goods, false); err != nil {
			return nil, err
		}
	case "burn":
		if err := executeSack(ctx, tx, scheduler, eventStore, clk, hub, worldID, o.SettlementID, provinceID,
			q, r, name, ownerID, o.PlayerID, o.Goods, true); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit occupy action: %w", err)
	}
	return &OccupyActionResult{SettlementID: o.SettlementID, Action: o.Action}, nil
}

// executeAnnex transfers ownership — the only moment this whole feature
// changes owner_id. Mirrors the old applyAttackerWins annex branch (same
// true outcome), minus placing the attacker as garrison (already done by
// occupySettlement at S1).
func executeAnnex(
	ctx context.Context, tx pgx.Tx, eventStore *events.Store, hub Broadcaster,
	worldID, settlementID, provinceID uuid.UUID, name string, formerOwnerID *uuid.UUID, newOwnerID uuid.UUID,
) error {
	if _, err := tx.Exec(ctx,
		`UPDATE settlements SET
		   owner_id = $2, control_type = 'occupied', is_capital = false, kingdom_id = NULL,
		   state = 'active', occupant_id = NULL, occupied_since_tick = NULL, annex_ready_notified = false,
		   updated_at = now()
		 WHERE id = $1`,
		settlementID, newOwnerID,
	); err != nil {
		return fmt.Errorf("annex: transfer ownership: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE provinces SET territory_state = 'controlled' WHERE id = $1`, provinceID); err != nil {
		return fmt.Errorf("annex: update territory: %w", err)
	}
	if formerOwnerID != nil {
		if _, err := handleOwnerCityLoss(ctx, tx, *formerOwnerID, worldID, settlementID); err != nil {
			return fmt.Errorf("annex: handle former owner city loss: %w", err)
		}
	}
	if err := gossip.Broadcast(ctx, tx, worldID, settlementID, "military",
		name+" has been annexed after occupation.", 6, gossip.ImportanceMajor, settlementID, ""); err != nil {
		slog.Warn("annex: broadcast gossip", "settlement", settlementID, "err", err)
	}
	// Reuses SettlementCaptured (unit_arrival.go's old annex path) — same true
	// fact (ownership transferred), not a reinterpretation: only the ROUTE to
	// this moment changed (occupation-then-annex instead of instant), the
	// event's own meaning ("this settlement now belongs to new_owner") did not.
	_, _ = eventStore.Append(ctx, settlementID, events.StreamProvince, "SettlementCaptured",
		map[string]any{"settlement_id": settlementID, "former_owner": formerOwnerID, "new_owner": newOwnerID},
		worldID, nil)
	if hub != nil {
		if formerOwnerID != nil {
			_ = hub.NotifyPlayer(ctx, worldID, *formerOwnerID, "SettlementCaptured", 1, map[string]any{
				"settlement_id": settlementID, "role": "defender",
			})
		}
		_ = hub.NotifyPlayer(ctx, worldID, newOwnerID, "SettlementCaptured", 3, map[string]any{
			"settlement_id": settlementID, "role": "attacker",
		})
	}
	return economy.RecomputeProduction(ctx, tx, settlementID)
}

// executeSack is S4 (burn=false) and S5's loot-then-destroy half (burn=true).
// Loot always happens the same way; what differs is what's left of the city
// afterward — S4 leaves it standing, weakened, with the defender; S5 razes it.
func executeSack(
	ctx context.Context, tx pgx.Tx, scheduler *events.Scheduler, eventStore *events.Store, clk clock.Clock, hub Broadcaster,
	worldID, settlementID, provinceID uuid.UUID, destQ, destR int, name string,
	formerOwnerID *uuid.UUID, raiderID uuid.UUID, goodsFilter []string, burn bool,
) error {
	manifest, err := lootSettlementGoods(ctx, tx, settlementID, goodsFilter)
	if err != nil {
		return err
	}

	// The occupying army leaves the city either way — it marches the loot
	// home, it does not stay garrisoned in what it just plundered.
	if _, err := tx.Exec(ctx,
		`UPDATE units SET status = 'positioned', settlement_id = NULL, updated_at = now()
		 WHERE settlement_id = $1 AND status = 'garrison' AND owner_id = $2`,
		settlementID, raiderID,
	); err != nil {
		return fmt.Errorf("sack: release occupying garrison: %w", err)
	}

	var terrain string
	_ = tx.QueryRow(ctx, `SELECT terrain_type FROM provinces WHERE id = $1`, provinceID).Scan(&terrain)

	if len(manifest) > 0 {
		if err := dispatchPlunderCaravan(ctx, tx, scheduler, clk, worldID, raiderID, settlementID,
			destQ, destR, terrain, manifest); err != nil {
			return err
		}
	}

	if burn {
		if _, err := tx.Exec(ctx,
			`UPDATE settlements SET
			   owner_id = NULL, control_type = 'occupied', kingdom_id = NULL, state = 'razed',
			   occupant_id = NULL, occupied_since_tick = NULL, annex_ready_notified = false,
			   recolonizable_after_tick = current_world_tick() + $2, updated_at = now()
			 WHERE id = $1`,
			settlementID, burnRecolonizeKaren,
		); err != nil {
			return fmt.Errorf("burn: raze settlement: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE provinces SET territory_state = 'free', controller_id = NULL WHERE id = $1`, provinceID,
		); err != nil {
			return fmt.Errorf("burn: free province: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE units SET status = 'disbanded', size = 0, updated_at = now()
			 WHERE settlement_id = $1 AND status = 'garrison'`,
			settlementID,
		); err != nil {
			return fmt.Errorf("burn: disband remaining garrison: %w", err)
		}
		if formerOwnerID != nil {
			if _, err := handleOwnerCityLoss(ctx, tx, *formerOwnerID, worldID, settlementID); err != nil {
				return fmt.Errorf("burn: handle former owner city loss: %w", err)
			}
		}
		if err := gossip.Broadcast(ctx, tx, worldID, settlementID, "military",
			name+" was sacked and burned.", 6, gossip.ImportanceMajor, settlementID, ""); err != nil {
			slog.Warn("burn: broadcast gossip", "settlement", settlementID, "err", err)
		}
		var recolAfter int
		_ = tx.QueryRow(ctx, `SELECT recolonizable_after_tick FROM settlements WHERE id = $1`, settlementID).Scan(&recolAfter)
		_, _ = eventStore.Append(ctx, settlementID, events.StreamProvince, EventSettlementBurned,
			SettlementBurnedPayload{
				SettlementID: settlementID, WorldID: worldID, RaiderID: raiderID, FormerOwner: formerOwnerID,
				Looted: manifest, RecolonizableAfterTick: recolAfter,
			}, worldID, nil)
		if hub != nil {
			if formerOwnerID != nil {
				_ = hub.NotifyPlayer(ctx, worldID, *formerOwnerID, "SettlementBurned", 1, map[string]any{
					"settlement_id": settlementID, "name": name, "role": "defender",
				})
			}
			_ = hub.NotifyPlayer(ctx, worldID, raiderID, "SettlementBurned", 3, map[string]any{
				"settlement_id": settlementID, "name": name, "role": "attacker", "looted": manifest,
			})
		}
		return nil
	}

	// ── S4 plain sack: city stands, stays with the defender, weakened. ──────
	var population int
	_ = tx.QueryRow(ctx, `SELECT population FROM settlements WHERE id = $1`, settlementID).Scan(&population)
	popLoss := int(math.Round(float64(population) * sackPopLossFraction))

	buildingHit, err := decrementTopProductionBuilding(ctx, tx, settlementID)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE settlements SET
		   population = GREATEST(50, population - $2),
		   state = 'active', occupant_id = NULL, occupied_since_tick = NULL, annex_ready_notified = false,
		   updated_at = now()
		 WHERE id = $1`,
		settlementID, popLoss,
	); err != nil {
		return fmt.Errorf("sack: apply pop loss: %w", err)
	}

	if err := gossip.Broadcast(ctx, tx, worldID, settlementID, "military",
		name+" was sacked.", 6, gossip.ImportanceMajor, settlementID, ""); err != nil {
		slog.Warn("sack: broadcast gossip", "settlement", settlementID, "err", err)
	}

	_, _ = eventStore.Append(ctx, settlementID, events.StreamProvince, EventSettlementLooted,
		SettlementLootedPayload{
			SettlementID: settlementID, WorldID: worldID, RaiderID: raiderID, FormerOwner: formerOwnerID,
			Looted: manifest, PopLost: popLoss, BuildingHit: buildingHit,
		}, worldID, nil)

	if hub != nil {
		if formerOwnerID != nil {
			_ = hub.NotifyPlayer(ctx, worldID, *formerOwnerID, "SettlementLooted", 1, map[string]any{
				"settlement_id": settlementID, "name": name, "role": "defender",
				"pop_lost": popLoss, "building_hit": buildingHit,
			})
		}
		_ = hub.NotifyPlayer(ctx, worldID, raiderID, "SettlementLooted", 3, map[string]any{
			"settlement_id": settlementID, "name": name, "role": "attacker", "looted": manifest,
		})
	}

	return economy.RecomputeProduction(ctx, tx, settlementID)
}

// decrementTopProductionBuilding drops the settlement's highest-level
// PRODUCTION building (province.LevelledBuildings — excludes wall and any
// unlevelled building) by one level, deterministically (ties broken by
// building_type name, no RNG — plan requirement). A level-1 building is
// destroyed outright (row deleted) rather than going to level 0. Returns the
// building type hit, or "" if the settlement had no levelled building at all.
func decrementTopProductionBuilding(ctx context.Context, tx pgx.Tx, settlementID uuid.UUID) (string, error) {
	rows, err := tx.Query(ctx, `SELECT building_type, level FROM buildings WHERE settlement_id = $1`, settlementID)
	if err != nil {
		return "", fmt.Errorf("sack: load buildings: %w", err)
	}
	var topType string
	var topLevel int
	found := false
	for rows.Next() {
		var bt string
		var lvl int
		if scanErr := rows.Scan(&bt, &lvl); scanErr != nil {
			rows.Close()
			return "", fmt.Errorf("sack: scan building: %w", scanErr)
		}
		if !province.LevelledBuildings[province.BuildingType(bt)] {
			continue
		}
		if !found || lvl > topLevel || (lvl == topLevel && bt < topType) {
			found, topType, topLevel = true, bt, lvl
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", err
	}
	if !found {
		return "", nil
	}
	if topLevel <= 1 {
		if _, err := tx.Exec(ctx, `DELETE FROM buildings WHERE settlement_id = $1 AND building_type = $2`,
			settlementID, topType); err != nil {
			return "", fmt.Errorf("sack: destroy building: %w", err)
		}
	} else {
		if _, err := tx.Exec(ctx, `UPDATE buildings SET level = level - 1 WHERE settlement_id = $1 AND building_type = $2`,
			settlementID, topType); err != nil {
			return "", fmt.Errorf("sack: downgrade building: %w", err)
		}
	}
	return topType, nil
}

// lootSettlementGoods computes the loot manifest and deducts it from the
// settlement's goods/granary in the same transaction. Same formula as the
// old (now-dead) sackSettlement (sack.go): silver 50%, every other good
// 0.5/weight — re-derived here rather than shared (see file header). If
// goodsFilter is non-empty, only those good keys are looted (S4's "anfallaren
// väljer vilka varor"); empty means every lootable good, matching the old
// unconditional default.
func lootSettlementGoods(ctx context.Context, tx pgx.Tx, settlementID uuid.UUID, goodsFilter []string) (transport.Manifest, error) {
	allowed := map[string]bool{}
	for _, g := range goodsFilter {
		allowed[g] = true
	}
	filterAll := len(allowed) == 0

	manifest := transport.Manifest{}

	rows, err := tx.Query(ctx,
		`SELECT sg.good_key, floor(settled(sg.amount, sg.rate, sg.calc_tick) *
		        CASE WHEN sg.good_key = 'silver' THEN 0.5 ELSE 0.5 / g.weight END) AS loot
		 FROM settlement_goods sg JOIN goods g ON g.key = sg.good_key
		 WHERE sg.settlement_id = $1 AND g.weight > 0`,
		settlementID,
	)
	if err != nil {
		return nil, fmt.Errorf("loot: load manifest: %w", err)
	}
	type lootRow struct {
		good string
		qty  float64
	}
	var loots []lootRow
	for rows.Next() {
		var lr lootRow
		if scanErr := rows.Scan(&lr.good, &lr.qty); scanErr != nil {
			rows.Close()
			return nil, fmt.Errorf("loot: scan row: %w", scanErr)
		}
		if lr.qty > 0 && (filterAll || allowed[lr.good]) {
			loots = append(loots, lr)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, lr := range loots {
		manifest[lr.good] += lr.qty
		if _, err := tx.Exec(ctx,
			`UPDATE settlement_goods SET
			   amount    = GREATEST(0, settled(amount, rate, calc_tick) - $2),
			   calc_tick = current_world_tick()
			 WHERE settlement_id = $1 AND good_key = $3`,
			settlementID, lr.qty, lr.good,
		); err != nil {
			return nil, fmt.Errorf("loot: deduct %s: %w", lr.good, err)
		}
	}

	grows, err := tx.Query(ctx,
		`SELECT good_key, floor(GREATEST(0, amount) * 0.5) FROM settlement_granary
		 WHERE settlement_id = $1 AND amount > 0`, settlementID)
	if err != nil {
		return nil, fmt.Errorf("loot: load granary: %w", err)
	}
	type granaryLoot struct {
		good string
		qty  float64
	}
	var granaryLoots []granaryLoot
	for grows.Next() {
		var gl granaryLoot
		if scanErr := grows.Scan(&gl.good, &gl.qty); scanErr != nil {
			grows.Close()
			return nil, fmt.Errorf("loot: scan granary: %w", scanErr)
		}
		if gl.qty > 0 && (filterAll || allowed[gl.good]) {
			granaryLoots = append(granaryLoots, gl)
		}
	}
	grows.Close()
	if err := grows.Err(); err != nil {
		return nil, err
	}
	for _, gl := range granaryLoots {
		manifest[gl.good] += gl.qty
		if _, err := tx.Exec(ctx,
			`UPDATE settlement_granary SET amount = GREATEST(0, amount - $2)
			 WHERE settlement_id = $1 AND good_key = $3`,
			settlementID, gl.qty, gl.good,
		); err != nil {
			return nil, fmt.Errorf("loot: deduct granary %s: %w", gl.good, err)
		}
	}

	return manifest, nil
}

// dispatchPlunderCaravan sends the loot home as a physical, interceptable
// caravan (kind="plunder") toward the raider's capital — same mechanics as
// the old sackSettlement (sack.go), re-derived here (see file header).
func dispatchPlunderCaravan(
	ctx context.Context, tx pgx.Tx, scheduler *events.Scheduler, clk clock.Clock,
	worldID, raiderID, settlementID uuid.UUID, destQ, destR int, originTerrain string, manifest transport.Manifest,
) error {
	var capitalID uuid.UUID
	var capQ, capR int
	if err := tx.QueryRow(ctx,
		`SELECT s.id, p.map_q, p.map_r FROM settlements s JOIN provinces p ON p.id = s.province_id
		 WHERE s.owner_id = $1 AND s.world_id = $2 AND s.is_capital = true`,
		raiderID, worldID,
	).Scan(&capitalID, &capQ, &capR); err != nil {
		slog.Warn("loot: raider has no capital, loot lost", "owner", raiderID, "err", err)
		return nil
	}

	_, pathTicks, pathOK, pathErr := province.FindPath(ctx, tx, worldID,
		province.MapPosition{Q: destQ, R: destR}, province.MapPosition{Q: capQ, R: capR}, "land")
	var moveTicks float64
	if pathErr == nil && pathOK {
		moveTicks = pathTicks
	} else {
		// Island-raid degradation, same accepted MVP behaviour as the old
		// sackSettlement: no land route home → uninterceptable but still
		// arrives.
		dist := province.HexDistance(province.MapPosition{Q: destQ, R: destR}, province.MapPosition{Q: capQ, R: capR})
		if dist < 1 {
			dist = 1
		}
		moveTicks = province.TerrainMoveTicks(originTerrain) * float64(dist)
	}
	travelTicks := int(math.Round(moveTicks))
	if travelTicks < 1 {
		travelTicks = 1
	}
	var currentTick int
	_ = tx.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
	now := clk.Now()
	arrivesAt := now.Add(time.Duration(travelTicks*tick.TickSeconds) * time.Second)

	if _, err := transport.Dispatch(ctx, tx, scheduler, transport.DispatchParams{
		WorldID: worldID, OwnerID: raiderID, Kind: "plunder",
		OriginID: settlementID, DestID: capitalID, Category: "land",
		OriginQ: destQ, OriginR: destR, DestQ: capQ, DestR: capR,
		DepartsAt: now, ArrivesAt: arrivesAt, DueTick: currentTick + travelTicks,
		Manifest: manifest, Interceptable: true,
	}); err != nil {
		return fmt.Errorf("loot: dispatch plunder caravan: %w", err)
	}
	return nil
}
