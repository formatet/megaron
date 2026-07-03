package combat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/economy"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/gossip"
	"github.com/poleia/server/internal/province"
	"github.com/poleia/server/internal/unit"
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
	pool       *pgxpool.Pool
	eventStore *events.Store
	hub        Broadcaster
	scheduler  *events.Scheduler
	clk        clock.Clock
	sitosCfg   economy.SitosConfig
}

// NewUnitArrivalHandler creates a UnitArrivalHandler.
func NewUnitArrivalHandler(pool *pgxpool.Pool, store *events.Store, hub Broadcaster, scheduler *events.Scheduler, clk clock.Clock, sitosCfg economy.SitosConfig) *UnitArrivalHandler {
	return &UnitArrivalHandler{pool: pool, eventStore: store, hub: hub, scheduler: scheduler, clk: clk, sitosCfg: sitosCfg}
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

	if err := h.resolve(ctx, tx, payload.UnitID, payload.WorldID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (h *UnitArrivalHandler) resolve(ctx context.Context, tx pgx.Tx, unitID, worldID uuid.UUID) error {
	// Load arriving unit with FOR UPDATE — idempotency guard.
	var u unitRow
	if err := tx.QueryRow(ctx,
		`SELECT id, owner_id, type, category, size, crew, cargo_unit_id,
		        status, q, r, target_q, target_r, stance, march_intent, colony_name
		 FROM units WHERE id = $1 FOR UPDATE`,
		unitID,
	).Scan(&u.id, &u.ownerID, &u.utype, &u.category, &u.size, &u.crew, &u.cargoUnitID,
		&u.status, &u.q, &u.r, &u.targetQ, &u.targetR, &u.stance, &u.marchIntent, &u.colonyName); err != nil {
		return fmt.Errorf("load arriving unit: %w", err)
	}

	// Idempotent: already resolved.
	if u.status != "marching" {
		return nil
	}
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

	// Find settlement at destination (if any).
	var dest destSettlement
	err := tx.QueryRow(ctx,
		`SELECT s.id, s.owner_id, s.wall_level, p.id, p.terrain_type
		 FROM provinces p
		 LEFT JOIN settlements s ON s.province_id = p.id
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
		return h.foundColony(ctx, tx, u, dest.provinceID, destQ, destR, worldID)
	}

	// No settlement or uncontested → become garrison.
	if !hasSettlement || dest.ownerID == nil || *dest.ownerID == u.ownerID {
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
		   updated_at    = now()
		 WHERE id = $1`,
		u.id, newStatus, destQ, destR, settlementID,
	); err != nil {
		return fmt.Errorf("unit arrive garrison: %w", err)
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

	// Fas 2h: this was the one arrival path with no player-facing notification —
	// ColonyFounded/ArmyArrival/OutpostEstablished already notify, but a plain
	// peaceful march (no combat, no colonize) only ever wrote the audit event
	// above, so a Wanax had to poll `unit list`/`map` to learn a march landed.
	if h.hub != nil {
		_ = h.hub.NotifyPlayer(ctx, worldID, u.ownerID, "UnitArrived", 4, map[string]any{
			"unit_id": u.id,
			"type":    u.utype,
			"q":       destQ,
			"r":       destR,
			"status":  newStatus,
		})
	}

	slog.Info("unit arrived (garrison)", "unit", u.id, "q", destQ, "r", destR, "status", newStatus)
	return nil
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

	// Colony name: chosen, else culture-appropriate default.
	name := province.SettlementNameForCulture(culture)
	if u.colonyName != nil && *u.colonyName != "" {
		name = *u.colonyName
	}

	// Create the colony. Starting population 1500 — a real but modest second city
	// — plus the colonizing unit's own size, since its colonists join the founding
	// populace (they become citizens, not a garrison; see below). Unlike the
	// capital the colony is NOT guaranteed self-sufficient (it can starve if
	// neglected); that asymmetry is the intended cost of expansion.
	const colonyBasePopulation = 1500
	population := colonyBasePopulation + u.size
	var colonyID uuid.UUID
	if err := tx.QueryRow(ctx,
		`INSERT INTO settlements
		 (world_id, province_id, name, culture_id, owner_id, control_type, is_capital,
		  loyalty, governor_is_ai, population, founded_from)
		 VALUES ($1,$2,$3,$4,$5,'colony',false,2,true,$7,$6)
		 RETURNING id`,
		worldID, provinceID, name, culture, u.ownerID, parentID, population,
	).Scan(&colonyID); err != nil {
		return fmt.Errorf("foundColony: create settlement: %w", err)
	}

	// Sitos genesis seed: sow the colony's fund (pop-scaled, so a small colony and
	// a large capital get proportionally identical coverage). Silver-invariant
	// exception, like the colony's start-grain — see temenos_sitos.md.
	if grainBaseValue, gbErr := economy.GoodBaseValue(ctx, tx, "grain"); gbErr != nil {
		slog.Error("sitos genesis: load grain base value", "err", gbErr)
	} else {
		seed, _ := economy.GenesisFundSeed(population, grainBaseValue, h.sitosCfg)
		if _, err := tx.Exec(ctx,
			`UPDATE settlements SET sitos_fund_silver = $1 WHERE id = $2`, seed, colonyID,
		); err != nil {
			slog.Error("sitos genesis seed failed", "err", err, "settlement", colonyID)
		}
	}

	// Link province back to its controlling settlement.
	_, _ = tx.Exec(ctx, `UPDATE provinces SET controller_id=$1 WHERE id=$2`, colonyID, provinceID)

	// Seed a zero/baseline row for every good (mirrors join.go), then let
	// RecomputeProduction write real rates from the catchment + labor weights.
	if _, err := tx.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 SELECT $1, g.key,
		        CASE g.key WHEN 'grain' THEN 300 WHEN 'timber' THEN 200 WHEN 'stone' THEN 300 ELSE 0 END,
		        0,
		        CASE g.key WHEN 'grain' THEN 1000 WHEN 'timber' THEN 500 WHEN 'cedar' THEN 500
		                   WHEN 'stone' THEN 1000 WHEN 'copper' THEN 300 WHEN 'tin' THEN 300
		                   WHEN 'silver' THEN 1000 ELSE 200 END,
		        current_world_tick()
		 FROM goods g
		 ON CONFLICT (settlement_id, good_key) DO NOTHING`,
		colonyID,
	); err != nil {
		return fmt.Errorf("foundColony: seed goods: %w", err)
	}

	// Baseline labor: grain dominates so the colony feeds itself; cult floor keeps a
	// temple (once built) non-inert. Same seed as the capital — the agent reallocates
	// toward ore via LaborAlloc once it builds a mine on the deposit it colonized for.
	if _, err := tx.Exec(ctx,
		`INSERT INTO settlement_labor (settlement_id, good_key, weight)
		 VALUES ($1,'grain',0.85), ($1,'cult',0.15)
		 ON CONFLICT (settlement_id, good_key) DO NOTHING`,
		colonyID,
	); err != nil {
		return fmt.Errorf("foundColony: seed labor: %w", err)
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
		_ = h.hub.NotifyPlayer(ctx, worldID, u.ownerID, "ColonyFounded", 3, map[string]any{
			"settlement_id": colonyID,
			"name":          name,
			"province_id":   provinceID,
			"q":             destQ,
			"r":             destR,
		})
	}

	slog.Info("colony founded (discrete unit)", "settlement", colonyID, "name", name,
		"province", provinceID, "owner", u.ownerID, "founding_unit", u.id, "population", population, "q", destQ, "r", destR)
	return nil
}

// resolveCombat handles the arriving unit attacking an enemy settlement.
//
// Dual-read defence:
//   units at destination (unit.SummaryAtHex / ListBySettlement) +
//   legacy integer columns on settlement.
//
// Idempotency: u.status == 'marching' was checked at top; all writes are
// conditional on status or use ON CONFLICT DO NOTHING.
func (h *UnitArrivalHandler) resolveCombat(
	ctx context.Context, tx pgx.Tx,
	u unitRow, dest destSettlement, destQ, destR int, worldID uuid.UUID,
) error {
	// ── Attack strength ────────────────────────────────────────────────────────
	attStr := unitStrength(u.utype, u.size)

	// ── Defence strength: dual-read ────────────────────────────────────────────
	// 1. Units-table garrison at destination.
	// C5: units in fortify stance receive a +50% defensive multiplier (tunable).
	// Rationale: fortify represents dug-in defensive positions. Applied per-unit so
	// a mixed garrison gets partial benefit. Constant: fortifyBonus = 1.5.
	const fortifyBonus = 1.5
	var defUnitStr float64
	garrisonRows, err := tx.Query(ctx,
		`SELECT type, size, stance FROM units
		 WHERE settlement_id = $1 AND status = 'garrison' AND status != 'disbanded'`,
		*dest.settlementID,
	)
	if err == nil {
		for garrisonRows.Next() {
			var utype string
			var usize int
			var ustance *string
			if scanErr := garrisonRows.Scan(&utype, &usize, &ustance); scanErr == nil {
				str := unitStrength(utype, usize)
				// C5: fortify stance grants +50% defence.
				if ustance != nil && *ustance == "fortify" {
					str *= fortifyBonus
				}
				defUnitStr += str
			}
		}
		garrisonRows.Close()
	}

	// C5: storm stance halves the wall bonus for the attacker.
	// Normal wall multiplier: 1 + level×0.25.
	// Storm effective wall:   1 + level×0.25/2  (tunable; stormWallDivisor = 2.0).
	// Rationale: the attacking unit is carrying siege equipment and focusing on
	// breaching rather than holding field position. The bonus only reduces the wall
	// multiplier, not base unit-vs-unit strength.
	const stormWallDivisor = 2.0
	wallMod := WallModifier(dest.wallLevel)
	if u.stance != nil && *u.stance == "storm" {
		// Halve the extra bonus (the +0.25×level part); the base 1.0 is unchanged.
		extra := float64(dest.wallLevel) * 0.25
		wallMod = 1.0 + extra/stormWallDivisor
	}
	defStr := defUnitStr * wallMod

	// ── Fortune (W5): roll once, bias by kharis delta ─────────────────────────
	var attackerKharis, defenderKharis float64
	_ = tx.QueryRow(ctx,
		`SELECT GREATEST(0, settled(kharis_amount, kharis_rate, kharis_calc_tick))
		 FROM player_world_records WHERE player_id = $1 AND world_id = $2`,
		u.ownerID, worldID,
	).Scan(&attackerKharis)
	if dest.ownerID != nil {
		_ = tx.QueryRow(ctx,
			`SELECT GREATEST(0, settled(kharis_amount, kharis_rate, kharis_calc_tick))
			 FROM player_world_records WHERE player_id = $1 AND world_id = $2`,
			*dest.ownerID, worldID,
		).Scan(&defenderKharis)
	}
	fortune := rollFortune(attackerKharis, defenderKharis)
	attStrWithFortune := attStr * (1 + fortune)

	// ── Resolve ───────────────────────────────────────────────────────────────
	result := ResolveStrengths(attStrWithFortune, defStr, fortune)

	slog.Info("unit combat resolved",
		"unit", u.id, "q", destQ, "r", destR,
		"att", attStr, "fortune", fortune, "def", defStr, "outcome", result.Outcome,
		"rounds", result.Rounds, "att_routed", result.AttackerRouted)

	// ── Apply losses ──────────────────────────────────────────────────────────
	attSizeBefore := u.size
	attSizeAfter := int(float64(u.size) * (1 - result.AttackerLosses))
	attPopLost := attSizeBefore - attSizeAfter

	if result.Outcome == OutcomeAttackerWins {
		if err := h.applyAttackerWins(ctx, tx, u, dest, attSizeAfter, attPopLost, result, destQ, destR, worldID); err != nil {
			return err
		}
	} else {
		if err := h.applyDefenderWins(ctx, tx, u, dest, attSizeAfter, attPopLost, result, destQ, destR, worldID); err != nil {
			return err
		}
	}

	// Append combat events for both sides.
	_, _ = h.eventStore.Append(ctx, u.id, events.StreamType(unit.StreamUnit), unit.EventUnitCombatResolved,
		unit.UnitCombatResolvedPayload{
			UnitID:     u.id,
			Role:       "attacker",
			SizeBefore: attSizeBefore,
			SizeAfter:  attSizeAfter,
			Outcome:    string(result.Outcome),
			PopLost:    attPopLost,
		}, worldID, nil)

	_, _ = h.eventStore.Append(ctx, dest.provinceID, events.StreamCombat, "UnitCombatResolved",
		map[string]any{
			"unit_id":        u.id,
			"outcome":        string(result.Outcome),
			"att":            attStr,
			"fortune":        result.Fortune,
			"def":            defStr,
			"att_losses":     result.AttackerLosses,
			"def_losses":     result.DefenderLosses,
			"att_routed":     result.AttackerRouted,
			"def_routed":     result.DefenderRouted,
			"rounds":         result.Rounds,
		}, worldID, nil)

	return nil
}

// disbandCargoIfPresent disbands a naval unit's cargo land unit when the ship is
// destroyed in combat (C6). The cargo unit is marked 'disbanded'; men are
// permanently lost (demographic cost applied to their owner's capital).
// Non-fatal: errors are logged but do not abort the calling operation.
func (h *UnitArrivalHandler) disbandCargoIfPresent(ctx context.Context, tx pgx.Tx, ship unitRow, worldID uuid.UUID) {
	if ship.cargoUnitID == nil {
		return
	}
	cargoID := *ship.cargoUnitID
	// Mark cargo disbanded.
	if _, err := tx.Exec(ctx,
		`UPDATE units SET status = 'disbanded', updated_at = now()
		 WHERE id = $1 AND status = 'embarked'`,
		cargoID,
	); err != nil {
		slog.Warn("C6: could not disband cargo unit after ship loss", "ship", ship.id, "cargo", cargoID, "err", err)
		return
	}
	// Demographic loss: fetch cargo size and apply to owner's capital.
	var cargoSize int
	var cargoOwnerID uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT size, owner_id FROM units WHERE id = $1`, cargoID,
	).Scan(&cargoSize, &cargoOwnerID); err != nil {
		slog.Warn("C6: could not read cargo unit for pop loss", "cargo", cargoID, "err", err)
		return
	}
	if cargoSize > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE settlements SET
			   population = GREATEST(50, population - $2)
			 WHERE owner_id = $1 AND world_id = $3 AND is_capital = true`,
			cargoOwnerID, cargoSize, worldID,
		); err != nil {
			slog.Warn("C6: could not apply cargo pop loss", "cargo", cargoID, "err", err)
		}
	}
	slog.Info("C6: cargo unit disbanded after ship destruction", "ship", ship.id, "cargo", cargoID, "men_lost", cargoSize)
}

// applyAttackerWins: arriving unit captures the settlement; defender units take losses.
func (h *UnitArrivalHandler) applyAttackerWins(
	ctx context.Context, tx pgx.Tx,
	u unitRow, dest destSettlement,
	attSizeAfter, attPopLost int,
	result CombatResult,
	destQ, destR int, worldID uuid.UUID,
) error {
	// Apply attacker losses to the arriving unit.
	if attSizeAfter <= 0 {
		// Attacker destroyed (shouldn't happen on win, but be safe).
		if _, err := tx.Exec(ctx,
			`UPDATE units SET status = 'disbanded', updated_at = now() WHERE id = $1`, u.id,
		); err != nil {
			return fmt.Errorf("disband zeroed attacker: %w", err)
		}
		// C6: if this ship had cargo, disband the cargo too.
		h.disbandCargoIfPresent(ctx, tx, u, worldID)
	} else {
		// Attacker becomes new garrison.
		if _, err := tx.Exec(ctx,
			`UPDATE units SET
			   size         = $2,
			   status       = 'garrison',
			   q            = $3,
			   r            = $4,
			   settlement_id = $5,
			   target_q     = NULL,
			   target_r     = NULL,
			   departs_at   = NULL,
			   arrives_at   = NULL,
			   updated_at   = now()
			 WHERE id = $1`,
			u.id, attSizeAfter, destQ, destR, dest.settlementID,
		); err != nil {
			return fmt.Errorf("place attacker garrison: %w", err)
		}
	}

	// Demographic cost: attacker's origin settlement loses the dead men.
	// We look up the attacker's home settlement by owner.
	if attPopLost > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE settlements SET
			   population = GREATEST(50, population - $2)
			 WHERE owner_id = $1 AND world_id = $3 AND is_capital = true`,
			u.ownerID, attPopLost, worldID,
		); err != nil {
			slog.Warn("could not apply attacker pop loss", "unit", u.id, "lost", attPopLost, "err", err)
		}
	}

	// Defender garrison units: apply losses and disband zeroed units.
	if err := h.applyDefenderUnitLosses(ctx, tx, *dest.settlementID, result.DefenderLosses, worldID); err != nil {
		return err
	}

	// Transfer settlement ownership.
	var fallenName string
	_ = tx.QueryRow(ctx, `SELECT name FROM settlements WHERE id = $1`, *dest.settlementID).Scan(&fallenName)
	if _, err := tx.Exec(ctx,
		`UPDATE settlements SET
		   owner_id     = $2,
		   control_type = 'occupied',
		   kingdom_id   = NULL,
		   updated_at   = now()
		 WHERE id = $1`,
		*dest.settlementID, u.ownerID,
	); err != nil {
		return fmt.Errorf("transfer settlement ownership: %w", err)
	}

	// Rumor: a settlement falling to conquest is major news — hearsay, several
	// hops (temenos_gossip.md PASS 2b). Best-effort — never fail the conquest
	// over gossip.
	if fallenName != "" {
		if err := gossip.Broadcast(ctx, tx, worldID, *dest.settlementID, "military",
			fallenName+" has fallen to conquest.", 6,
			gossip.ImportanceMajor, *dest.settlementID, ""); err != nil {
			slog.Warn("applyAttackerWins: broadcast gossip", "settlement", *dest.settlementID, "err", err)
		}
	}

	// Update province territory state.
	if _, err := tx.Exec(ctx,
		`UPDATE provinces SET territory_state = 'controlled' WHERE id = $1`, dest.provinceID,
	); err != nil {
		return fmt.Errorf("update territory state: %w", err)
	}

	// Mark old owner dispossessed.
	if dest.ownerID != nil {
		_, _ = tx.Exec(ctx,
			`UPDATE player_world_records SET status = 'dispossessed', settlement_id = NULL
			 WHERE player_id = $1 AND world_id = $2`,
			*dest.ownerID, worldID,
		)
		// Recompute production for defender (pop changed).
		var defSett uuid.UUID
		if err := tx.QueryRow(ctx,
			`SELECT id FROM settlements WHERE owner_id = $1 AND world_id = $2 AND is_capital = true`,
			*dest.ownerID, worldID,
		).Scan(&defSett); err == nil {
			_ = economy.RecomputeProduction(ctx, tx, defSett)
		}
	}

	_, _ = h.eventStore.Append(ctx, dest.provinceID, events.StreamType(unit.StreamUnit), unit.EventUnitArrived,
		unit.UnitArrivedPayload{
			UnitID:    u.id,
			Q:         destQ,
			R:         destR,
			NewStatus: "garrison",
		}, worldID, nil)

	return nil
}

// applyDefenderWins: arriving unit is beaten back, takes losses.
// If the unit routed (result.AttackerRouted), it survives with remaining men and
// marches back to its origin. If destroyed (size after losses = 0), it is disbanded.
func (h *UnitArrivalHandler) applyDefenderWins(
	ctx context.Context, tx pgx.Tx,
	u unitRow, dest destSettlement,
	attSizeAfter, attPopLost int,
	result CombatResult,
	destQ, destR int,
	worldID uuid.UUID,
) error {
	if attSizeAfter <= 0 {
		// Unit destroyed: demographic loss, unit disbanded.
		if _, err := tx.Exec(ctx,
			`UPDATE units SET status = 'disbanded', updated_at = now() WHERE id = $1`, u.id,
		); err != nil {
			return fmt.Errorf("disband zeroed attacker: %w", err)
		}
		h.disbandCargoIfPresent(ctx, tx, u, worldID)
	} else if result.AttackerRouted && h.scheduler != nil {
		// Rout (W5): unit survives with remaining men, retreats to origin.
		// origin = (u.q, u.r) — the hex the unit marched FROM (set by march handler).
		// Route via A* (same passability graph the forward march used) instead of a
		// straight line, so a routing unit cannot teleport across impassable terrain.
		_, pathHours, pathOK, pathErr := province.FindPath(ctx, tx, worldID,
			province.MapPosition{Q: destQ, R: destR},
			province.MapPosition{Q: u.q, R: u.r},
			u.category,
		)
		var moveHours float64
		if pathErr == nil && pathOK {
			moveHours = pathHours
		} else {
			// Defensive fallback: the forward march already proved a route exists,
			// so this should not happen. Straight-line estimate keeps the rout from
			// stalling if it ever does.
			slog.Warn("rout retreat: FindPath failed, falling back to straight line",
				"unit", u.id, "err", pathErr)
			dist := province.HexDistance(
				province.MapPosition{Q: destQ, R: destR},
				province.MapPosition{Q: u.q, R: u.r},
			)
			if dist < 1 {
				dist = 1
			}
			var originTerrain string
			_ = tx.QueryRow(ctx,
				`SELECT terrain_type FROM provinces WHERE world_id = $1 AND map_q = $2 AND map_r = $3`,
				worldID, u.q, u.r,
			).Scan(&originTerrain)
			if originTerrain == "" {
				originTerrain = "plains"
			}
			moveHours = province.TerrainMoveHours(originTerrain) * float64(dist)
		}
		arrivesAt := h.clk.Now().Add(time.Duration(moveHours * float64(time.Hour)))

		var currentTick int
		_ = tx.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
		travelTicks := int(math.Round(moveHours))
		if travelTicks < 1 {
			travelTicks = 1
		}

		if _, err := tx.Exec(ctx,
			`UPDATE units SET
			   size        = $2,
			   status      = 'marching',
			   q           = $3,
			   r           = $4,
			   target_q    = $5,
			   target_r    = $6,
			   departs_at  = now(),
			   arrives_at  = $7,
			   settlement_id = NULL,
			   updated_at  = now()
			 WHERE id = $1`,
			u.id, attSizeAfter, destQ, destR, u.q, u.r, arrivesAt,
		); err != nil {
			return fmt.Errorf("route unit back to origin: %w", err)
		}
		arrPayload := unit.ScheduledUnitArrivalPayload{UnitID: u.id, WorldID: worldID}
		if err := h.scheduler.EnqueueTickTx(ctx, tx, worldID, events.ScheduledUnitArrival, arrPayload, currentTick+travelTicks); err != nil {
			return fmt.Errorf("schedule rout return march: %w", err)
		}
		slog.Info("unit routed, returning to origin", "unit", u.id, "origin_q", u.q, "origin_r", u.r, "size", attSizeAfter)
	} else {
		// Beaten (not routed) — unit disbanded, treat as destroyed.
		if _, err := tx.Exec(ctx,
			`UPDATE units SET status = 'disbanded', updated_at = now() WHERE id = $1`, u.id,
		); err != nil {
			return fmt.Errorf("disband beaten attacker: %w", err)
		}
		h.disbandCargoIfPresent(ctx, tx, u, worldID)
	}

	// Permanent demographic loss for attacker.
	if attPopLost > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE settlements SET
			   population = GREATEST(50, population - $2)
			 WHERE owner_id = $1 AND world_id = $3 AND is_capital = true`,
			u.ownerID, attPopLost, worldID,
		); err != nil {
			slog.Warn("could not apply attacker pop loss (def win)", "unit", u.id, "err", err)
		}
	}

	// Defender unit losses.
	if dest.settlementID != nil {
		if err := h.applyDefenderUnitLosses(ctx, tx, *dest.settlementID, result.DefenderLosses, worldID); err != nil {
			return err
		}
	}

	return nil
}

// applyDefenderUnitLosses reduces sizes of garrison units at a settlement and
// disbands those that reach 0. Also applies demographic cost to defender pop.
func (h *UnitArrivalHandler) applyDefenderUnitLosses(
	ctx context.Context, tx pgx.Tx,
	settlementID uuid.UUID, lossRate float64, worldID uuid.UUID,
) error {
	rows, err := tx.Query(ctx,
		`SELECT id, type, size, cargo_unit_id FROM units WHERE settlement_id = $1 AND status = 'garrison'`,
		settlementID,
	)
	if err != nil {
		return fmt.Errorf("load defender units: %w", err)
	}
	type defUnit struct {
		id          uuid.UUID
		utype       string
		size        int
		cargoUnitID *uuid.UUID
	}
	var defUnits []defUnit
	for rows.Next() {
		var du defUnit
		if scanErr := rows.Scan(&du.id, &du.utype, &du.size, &du.cargoUnitID); scanErr == nil {
			defUnits = append(defUnits, du)
		}
	}
	rows.Close()

	// Load owner for demographic loss.
	var defOwnerID uuid.UUID
	_ = tx.QueryRow(ctx, `SELECT COALESCE(owner_id, gen_random_uuid()) FROM settlements WHERE id = $1`, settlementID).Scan(&defOwnerID)

	totalPopLost := 0
	for _, du := range defUnits {
		newSize := int(float64(du.size) * (1 - lossRate))
		lost := du.size - newSize
		totalPopLost += lost

		if newSize <= 0 {
			if _, err := tx.Exec(ctx,
				`UPDATE units SET status = 'disbanded', size = 0, updated_at = now() WHERE id = $1`, du.id,
			); err != nil {
				slog.Warn("could not disband defender unit", "unit", du.id, "err", err)
			}
			// C6: if this defender ship had cargo, disband the cargo too.
			if du.cargoUnitID != nil {
				shipRow := unitRow{id: du.id, ownerID: defOwnerID, cargoUnitID: du.cargoUnitID}
				h.disbandCargoIfPresent(ctx, tx, shipRow, worldID)
			}
		} else {
			if _, err := tx.Exec(ctx,
				`UPDATE units SET size = $2, updated_at = now() WHERE id = $1`, du.id, newSize,
			); err != nil {
				slog.Warn("could not reduce defender unit size", "unit", du.id, "err", err)
			}
		}
	}

	// Defender demographic loss.
	if totalPopLost > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE settlements SET population = GREATEST(50, population - $1) WHERE id = $2`,
			totalPopLost, settlementID,
		); err != nil {
			slog.Warn("could not apply defender pop loss", "settlement", settlementID, "err", err)
		}
		if err := economy.RecomputeProduction(ctx, tx, settlementID); err != nil {
			slog.Warn("recompute production after defender losses", "settlement", settlementID, "err", err)
		}
	}

	return nil
}

// ── Internal types ─────────────────────────────────────────────────────────────

type unitRow struct {
	id          uuid.UUID
	ownerID     uuid.UUID
	utype       string
	category    string
	size        int
	crew        int
	cargoUnitID *uuid.UUID
	status      string
	q           int    // origin hex (set by march handler; used for rout routing)
	r           int
	targetQ     *int
	targetR     *int
	stance      *string // C5: fortify/storm/sentry or nil
	marchIntent *string // "colonize" or nil (plain march)
	colonyName  *string // chosen colony name or nil
}

type destSettlement struct {
	provinceID   uuid.UUID
	settlementID *uuid.UUID
	ownerID      *uuid.UUID
	wallLevel    int
	terrain      string
}

// ── Strength helpers ───────────────────────────────────────────────────────────

// unitStrength computes the combat contribution of one unit row.
// Multipliers (per man / per vessel): spearman ×1, elite ×3, war_chariot ×4,
// galley ×1, war_galley ×3, priest/merchantman ×0.
func unitStrength(utype string, size int) float64 {
	switch utype {
	case "spearman":
		return float64(size) * 1
	case "elite_infantry":
		return float64(size) * 3
	case "war_chariot":
		return float64(size) * 4
	case "galley", "ship":
		return float64(size) * 1
	case "war_galley":
		return float64(size) * 3
	case "priest", "merchantman":
		return 0
	default:
		return float64(size) * 1
	}
}


