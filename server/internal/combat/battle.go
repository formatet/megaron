package combat

// KR3 stridssystemet — the persistent, multi-tick battle substrate
// (megaron_plan_kr3_stridssystem.md). This file holds:
//   - the §4 participation formula (named-constant rattar, mechanic locked)
//   - §3's reproducible per-(seed,tick,round) dice derivation
//   - initiateOrJoinBattle, the replacement for the old one-shot resolve
//   - BattleTickHandler, the ScheduledBattleTick state machine (§2)
//
// Scope, updated across two slices (megaron_todo.md ⚔ BYGGORDNING):
//   - 2026-08-06: only the field-arrival entry point
//     (unit_arrival_field.go:resolveFieldCombat) is wired to
//     initiateOrJoinBattle. The settlement resolveCombat, amphibious assault
//     and avsiktslagret's unit_intercept_scan.go entry points still use their
//     old one-shot resolve — rewired in a later slice (§8's own list).
//   - 2026-08-07: §5 rout is implemented (sideRouts/markSideRouted below) —
//     a side at/below its rout threshold breaks and leaves the battle WITH
//     its survivors (termination_reason "rout", not "annihilation"). The
//     per-unit standing_orders override (battle_participants.standing_orders)
//     is READ — see sideRouts' doc comment.
//   - 2026-08-07: §5's reträttorder-mid-battle-via-messenger half (a NEW order
//     arriving mid-fight, as opposed to a pre-set standing order) is now built
//     — SetStandingOrders (standing_orders_set.go), courier-delivered exactly
//     like SetStance for a field unit, instant for a besieged garrison.
//     termination_reason "retreat_order" is still unused (sideRouts always
//     labels a threshold breach "rout" regardless of whether the threshold
//     was the default threshold or a player override — not touched by this
//     slice, see sideRouts' own comment) — same as "no_enemy_left".
//   - 2026-08-07 (§8 cutover): the settlement siege (unit_arrival.go
//     resolveCombat) and amphibious assault (unit_arrival.go
//     resolveAmphibiousAssault) entry points are now wired to
//     initiateOrJoinBattle too, mirroring resolveFieldCombat — ALL garrison
//     units are sent in as separate defender participants (multi-garrison
//     was never a blocker, see battleParticipant).
//   - 2026-08-07 (§7 cutover, Timothy's march-interruption decision): the last
//     remaining entry point, avsiktslagret's unit_intercept_scan.go (S3), is
//     now also wired to initiateOrJoinBattle for reaction_policy.foreign=
//     "intercept" — the intercepted marching unit HALTS (status='positioned')
//     at its interpolated position instead of fighting one immediate roll and
//     continuing on its course; see that file's own header comment.
//     "alert" is notify-only (no battle); "escort" never triggers the scan.
//     All four §8 entry points now go through this substrate — the old
//     one-shot resolve (unitStrength/rollFortune/ResolveStrengthsWithRout)
//     has no more live combat-resolution callers, only battle.go's own
//     internal use of the same primitives (participation, rout threshold).
//     ⚠️ KNOWN GAP, deliberately left open by this slice: neither entry point
//     performs settlement CAPTURE on an attacker win anymore — the old
//     one-shot applyAttackerWins/applyDefenderWins/sackSettlement (still in
//     unit_arrival.go, still covered by their own direct-call unit tests)
//     are no longer reachable from any production code path. A besieging or
//     landing force can now only annihilate/rout a garrison; taking the city
//     itself is exactly the still-unused "attacker_reached_city" termination
//     reason below — hooking that up is unbuilt, unscoped follow-up work,
//     not a bug in this slice.
//   - 2026-08-07 (§8 mur-modellen, beslut 7): the wall is a SHIELD —
//     wallAbsorbBudget below absorbs the first N hits/battle-tick on the
//     defending side before losses are applied. wall_level and storm are
//     snapshotted onto the battles row at startBattle (migration 116).

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"

	"formatet/megaron/server/internal/carrier"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── §4 participation formula — mechanic LOCKED, values are named-constant rattar ──

const (
	// battleRoundsPerTick is R (§2/§10 default 3): rounds resolved simultaneously
	// within a single battle-tick, using the SAME sampled participation.
	battleRoundsPerTick = 3
	// battleDiceSides is T12; a hit is rolling exactly this value (p=1/12).
	battleDiceSides = 12
	// battleTickIntervalTicks: one battle-tick per world tick (a tick is a day —
	// CLAUDE.md tick/day rule — so this is the "how many ticks between battle-ticks"
	// cadence, always 1, same shape as events.MacroTickInterval).
	battleTickIntervalTicks = 1

	// hullMax is the ceiling of a naval vessel's graded damage track
	// (megaron_plan_skeppsreparation.md §B2): 5 = untouched, 0 = sunk. Used
	// both to scale a damaged ship's combat output (rollSide, below — B3
	// point 4, "hull skalar stridsstyrkan") and as the denominator of the
	// casualty-fraction → hull-point mapping (applyNavalHullDamage).
	hullMax = 5
	// HullMax is hullMax's exported alias — Slice C's repair handler
	// (api/handlers, outside this package) needs the same ceiling to report
	// a repair job's target hull. Same value, never diverge.
	HullMax = hullMax

	combatParticipation = 0.90

	// wallAbsorbPerLevel/stormWallAbsorbDivisor/stormAttackerLossMultiplier —
	// §8 mur-modellen (megaron_kr3_stridsutvardering.md beslut 7, Timothy
	// 2026-08-07). The wall is a SHIELD, not a strength multiplier: each
	// battle-tick it absorbs the first N = wallAbsorbPerLevel × wall_level
	// incoming hits on the defending side before losses are applied —
	// strawman 5/10/15 for level 1/2/3 (ratt, kalibreras vid speltest, §10).
	// A besieging force that lands more hits than N in a tick breaks the
	// wall anyway — intentional (a resolute siege can crack any wall).
	// storm halves N (stormWallAbsorbDivisor) but raises the storming
	// attacker's own losses (stormAttackerLossMultiplier) — same spirit as
	// the old one-shot model's stormWallDivisor, now expressed as a shield
	// term instead of a strength multiplier (there is no strength to
	// multiply in the dice model). Both wall_level and storm are snapshotted
	// once at startBattle, same stability guarantee as seed.
	wallAbsorbPerLevel          = 20
	stormWallAbsorbDivisor      = 2
	stormAttackerLossMultiplier = 1.5
)

// battleDiceMultiplier is how many T12 each ACTIVE combatant of utype rolls
// per round: elite 3/man, chariot 5/vagn, war galley 5/ship.
// Spearmen use ceil(active/2) in rollSide; other types use 1.
func battleDiceMultiplier(utype string) int {
	switch utype {
	case "elite_infantry":
		return 3
	case "war_chariot", "war_galley":
		return 5
	default:
		return 1
	}
}

// ── §3 Dice-söm: reproducible per-(seed,tick,round) derivation ──────────────

// battleRoundSeed derives an independent seed for one exact
// (battle, tick_index, round_index) triple from the battle's own seed. Two
// calls with the same three inputs always return the same value — this is
// what lets a battle_rounds row be replayed exactly from battles.seed plus
// its own tick_index/round_index, in any process, without any other state.
// A simple, well-mixed integer combination — reproducibility plumbing, not a
// cryptographic requirement.
func battleRoundSeed(battleSeed int64, tickIndex, roundIndex int) int64 {
	const (
		mul = 6364136223846793005
		inc = 1442695040888963407
	)
	h := uint64(battleSeed)
	h = h*mul + uint64(int64(tickIndex))*inc + 1
	h = h*mul + uint64(int64(roundIndex))*inc + 1
	return int64(h)
}

// randomSeed draws a fresh int64 battle seed from d — called exactly once,
// at battle initiation, never again for that battle (every later round
// re-derives from the stored seed via battleRoundSeed, not from d).
func randomSeed(d economy.Dice) int64 {
	hi := int64(d.Intn(1 << 31))
	lo := int64(d.Intn(1 << 31))
	return hi<<31 | lo
}

// ── initiateOrJoinBattle ──────────────────────────────────────────────────

// battleParticipant is the generic shape initiateOrJoinBattle accepts from
// any entry point — deliberately not unitRow/fieldDefender, so later slices
// can call this from the other three entry points named in the plan without
// changing its signature.
type battleParticipant struct {
	unitID       uuid.UUID
	ownerID      uuid.UUID
	utype        string
	side         string // "attacker" | "defender"
	currentSize  int
	defenseBoost bool
	// stance is optional (nil for callers that don't care — field arrival and
	// interception never set it, so storm below is always false for them,
	// same as before this field existed). Read only by startBattle, only off
	// the ARRIVING participant, to snapshot §8's storm flag onto the new
	// battles row: settlement siege sets it from the marching attacker's own
	// stance; amphibious assault sets it from the SHIP's stance (the cargo
	// does the fighting but the ship is what a player marks "storm" on) —
	// same source each used before this cutover. Defenders' stance does not
	// affect participation.
	stance *string
	// hull (megaron_plan_skeppsreparation.md Slice B) is only meaningful for a
	// naval participant — loaded fresh each battle-tick in resolveTick's own
	// query (below), NOT populated by initiateOrJoinBattle's callers (they
	// only ever create battle_participants rows, never roll dice, so they
	// have no need of it). Zero for every battleParticipant built anywhere
	// else in this file.
	hull int
	// cargoUnitID mirrors units.cargo_unit_id for a naval participant — the
	// land unit riding it, if any. Same "only resolveTick's own query
	// populates this" scope as hull above.
	cargoUnitID *uuid.UUID
}

// initiateOrJoinBattle is the KR3 replacement for the old one-shot combat
// resolve (§1). It never resolves combat itself — it only establishes (or
// extends) the persistent battles/battle_participants rows and ensures a
// ScheduledBattleTick is pending. Actual dice rolling happens later, in
// BattleTickHandler.
//
// defenders is only consulted when NO active battle exists yet at (q,r) — an
// arrival joining an already-active battle only ever registers itself
// (arriving); the other side's participants were registered when the battle
// started (or by their own later joins, once other entry points are wired).
func (h *UnitArrivalHandler) initiateOrJoinBattle(
	ctx context.Context, tx pgx.Tx, worldID uuid.UUID, q, r int,
	arriving battleParticipant, defenders []battleParticipant,
) error {
	var currentTick int
	if err := tx.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick); err != nil {
		return fmt.Errorf("initiate/join battle: read current tick: %w", err)
	}

	var battleID uuid.UUID
	err := tx.QueryRow(ctx,
		`SELECT id FROM battles WHERE world_id = $1 AND q = $2 AND r = $3 AND status = 'active' FOR UPDATE`,
		worldID, q, r,
	).Scan(&battleID)
	switch {
	case err == pgx.ErrNoRows:
		return h.startBattle(ctx, tx, worldID, q, r, currentTick, arriving, defenders)
	case err != nil:
		return fmt.Errorf("initiate/join battle: check existing: %w", err)
	default:
		return h.joinBattle(ctx, tx, worldID, battleID, currentTick, arriving)
	}
}

func (h *UnitArrivalHandler) startBattle(
	ctx context.Context, tx pgx.Tx, worldID uuid.UUID, q, r, currentTick int,
	arriving battleParticipant, defenders []battleParticipant,
) error {
	dice := h.Dice
	if dice == nil {
		dice = economy.NewWallDice()
	}
	seed := randomSeed(dice)

	// §8 mur-modellen: wall_level is a per-battle SNAPSHOT, same stability
	// guarantee as seed — a besieged settlement's wall level must not drift
	// mid-battle. Looked up directly from the hex rather than threaded through
	// every initiateOrJoinBattle caller: field arrival and interception never
	// have a settlement at (q,r), so this best-effort lookup naturally yields
	// 0 for them (no settlements row matches) — the absorption term becomes a
	// no-op without either of those entry points needing to know walls exist.
	var wallLevel int
	_ = tx.QueryRow(ctx,
		`SELECT s.wall_level FROM settlements s JOIN provinces p ON p.id = s.province_id
		 WHERE p.world_id = $1 AND p.map_q = $2 AND p.map_r = $3`,
		worldID, q, r,
	).Scan(&wallLevel)
	storm := arriving.stance != nil && *arriving.stance == "storm"

	var battleID uuid.UUID
	if err := tx.QueryRow(ctx,
		`INSERT INTO battles (world_id, q, r, started_tick, current_tick, status, seed, wall_level, storm)
		 VALUES ($1, $2, $3, $4, $4, 'active', $5, $6, $7) RETURNING id`,
		worldID, q, r, currentTick, seed, wallLevel, storm,
	).Scan(&battleID); err != nil {
		return fmt.Errorf("start battle: insert battles row: %w", err)
	}

	all := append([]battleParticipant{arriving}, defenders...)
	var attackerRefs, defenderRefs []BattleParticipantRef
	for _, p := range all {
		if _, err := tx.Exec(ctx,
			`INSERT INTO battle_participants (battle_id, unit_id, owner_id, side, joined_tick, initial_size, current_size, standing_orders)
			 VALUES ($1,$2,$3,$4,$5,$6,$6, `+seedStandingOrdersSQL+`)`,
			battleID, p.unitID, p.ownerID, p.side, currentTick, p.currentSize, worldID,
		); err != nil {
			return fmt.Errorf("start battle: insert participant %s: %w", p.unitID, err)
		}
		ref := BattleParticipantRef{UnitID: p.unitID, OwnerID: p.ownerID, UnitType: p.utype, Side: p.side, InitialSize: p.currentSize}
		if p.side == "attacker" {
			attackerRefs = append(attackerRefs, ref)
		} else {
			defenderRefs = append(defenderRefs, ref)
		}
	}

	event, err := h.eventStore.AppendTx(ctx, tx, battleID, events.StreamCombat, EventBattleStarted,
		BattleStartedPayload{
			BattleID: battleID, WorldID: worldID, Q: q, R: r,
			StartedTick: currentTick, Seed: seed,
			WallLevel: wallLevel, Storm: storm,
			Attackers: attackerRefs, Defenders: defenderRefs,
		}, worldID, nil,
	)
	if err != nil {
		return fmt.Errorf("start battle: append BattleStarted: %w", err)
	}
	if h.battleOutcomes != nil {
		h.battleOutcomes.events = append(h.battleOutcomes.events, event)
	}

	if err := h.breakHiketeiaTx(ctx, tx, worldID, battleID, arriving); err != nil {
		return err
	}
	if err := h.scheduler.EnqueueTickTx(ctx, tx, worldID, events.ScheduledBattleTick,
		battleTickPayload{BattleID: battleID}, currentTick,
	); err != nil {
		return fmt.Errorf("start battle: enqueue first tick: %w", err)
	}
	return h.persistBattleBegun(ctx, tx, worldID, battleID, q, r, false)
}

func (h *UnitArrivalHandler) joinBattle(
	ctx context.Context, tx pgx.Tx, worldID, battleID uuid.UUID, currentTick int, arriving battleParticipant,
) error {
	tag, err := tx.Exec(ctx,
		`INSERT INTO battle_participants (battle_id, unit_id, owner_id, side, joined_tick, initial_size, current_size, standing_orders)
		 VALUES ($1,$2,$3,$4,$5,$6,$6, `+seedStandingOrdersSQL+`)
		 ON CONFLICT (battle_id, unit_id) DO NOTHING`,
		battleID, arriving.unitID, arriving.ownerID, arriving.side, currentTick, arriving.currentSize, worldID,
	)
	if err != nil {
		return fmt.Errorf("join battle: insert participant: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil // already a participant (idempotent re-run)
	}
	if err := h.breakHiketeiaTx(ctx, tx, worldID, battleID, arriving); err != nil {
		return err
	}

	event, err := h.eventStore.AppendTx(ctx, tx, battleID, events.StreamCombat, EventUnitJoinedBattle,
		UnitJoinedBattlePayload{
			BattleID: battleID, UnitID: arriving.unitID, OwnerID: arriving.ownerID,
			UnitType: arriving.utype, Side: arriving.side, Size: arriving.currentSize, JoinedTick: currentTick,
		}, worldID, nil,
	)
	if err != nil {
		return fmt.Errorf("join battle: append event: %w", err)
	}
	if h.battleOutcomes != nil {
		h.battleOutcomes.events = append(h.battleOutcomes.events, event)
	}
	if arriving.side == "defender" {
		var q, r int
		if err := tx.QueryRow(ctx, `SELECT q,r FROM battles WHERE id=$1`, battleID).Scan(&q, &r); err != nil {
			return err
		}
		return h.persistBattleBegun(ctx, tx, worldID, battleID, q, r, true)
	}
	return nil
}

// ── ScheduledBattleTick state machine (§2) ──────────────────────────────────

// battleTickPayload is the scheduled_events payload for ScheduledBattleTick.
type battleTickPayload struct {
	BattleID uuid.UUID `json:"battle_id"`
}

// BattleTickHandler processes ScheduledBattleTick: one battle-tick of one
// active KR3 battle. Idempotent (G2) — all reads/writes for a tick happen in
// one transaction; a crash before commit leaves nothing partially applied, so
// a retry recomputes from the same persisted state and (because every dice
// draw is derived from battles.seed + tick_index + round_index, never from
// handler-instance state) reaches the identical result. The battle_rounds PK
// (battle_id, tick_index, round_index) is a belt-and-suspenders guard on top.
type BattleTickHandler struct {
	carrierOutcomes *[]*events.Event
	pool            *pgxpool.Pool
	eventStore      *events.Store
	scheduler       *events.Scheduler
	hub             Broadcaster
	// clk (megaron_plan_skeppsreparation.md Slice B) is only needed for the
	// routed side's damaged-ship home march (sendDamagedShipHome's arrives_at
	// display stamp — the authoritative depart_tick/arrive_tick pair is
	// tick-native and needs no clock at all, same K4 split as every other
	// march dispatch in this codebase). Never call time.Now() directly here.
	clk clock.Clock
}

// NewBattleTickHandler creates a BattleTickHandler. hub may be nil in tests
// (every NotifyPlayer call below is nil-guarded, matching the other combat
// handlers — see collapse.go).
func NewBattleTickHandler(pool *pgxpool.Pool, store *events.Store, scheduler *events.Scheduler, hub Broadcaster, clk clock.Clock) *BattleTickHandler {
	return &BattleTickHandler{pool: pool, eventStore: store, scheduler: scheduler, hub: hub, clk: clk}
}

// Handle processes one ScheduledBattleTick event.
func (h *BattleTickHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var payload battleTickPayload
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal battle tick payload: %w", err)
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var witnesses []*events.Event
	local := *h
	local.carrierOutcomes = &witnesses
	ended, err := local.resolveTick(ctx, tx, payload.BattleID, e.WorldID, e.DueTick)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	for _, w := range witnesses {
		h.eventStore.RecordCommitted(ctx, w)
	}
	if ended {
		return nil
	}
	return h.scheduler.EnqueueTickRecurring(ctx, e.WorldID, events.ScheduledBattleTick,
		battleTickPayload{BattleID: payload.BattleID}, e.DueTick, battleTickIntervalTicks)
}

// resolveTick runs the whole per-battle-tick sequence (§2): sample
// participation once, resolve battleRoundsPerTick rounds, apply losses,
// check termination. Returns ended=true if the battle just concluded.
func (h *BattleTickHandler) resolveTick(ctx context.Context, tx pgx.Tx, battleID, worldID uuid.UUID, tickIndex int) (bool, error) {
	var status string
	var q, r int
	var seed int64
	var wallLevel int
	var storm bool
	if err := tx.QueryRow(ctx,
		`SELECT status, q, r, seed, wall_level, storm FROM battles WHERE id = $1 FOR UPDATE`, battleID,
	).Scan(&status, &q, &r, &seed, &wallLevel, &storm); err != nil {
		return false, fmt.Errorf("battle tick: load battle: %w", err)
	}
	if status != "active" {
		return true, nil // already ended — stale re-enqueue race, idempotent no-op
	}

	var defenseTick int
	if err := tx.QueryRow(ctx, `SELECT current_tick FROM worlds WHERE id=$1`, worldID).Scan(&defenseTick); err != nil {
		return false, err
	}
	rows, err := tx.Query(ctx,
		`SELECT bp.unit_id, bp.owner_id, bp.side, bp.current_size, bp.initial_size, bp.standing_orders, u.type, u.hull, (SELECT cargo.id FROM units cargo WHERE cargo.carried_by_ship_id=u.id ORDER BY cargo.created_at,cargo.id LIMIT 1),
 COALESCE((SELECT s.garrison_defense_until_tick > $2 AND s.garrison_defense_owner_id=u.owner_id AND s.owner_id=u.owner_id
 FROM settlements s JOIN provinces sp ON sp.id=s.province_id
 WHERE s.id=u.settlement_id AND s.state='active' AND bp.side='defender' AND sp.world_id=$3 AND sp.map_q=$4 AND sp.map_r=$5),false)
		 FROM battle_participants bp JOIN units u ON u.id = bp.unit_id
		 WHERE bp.battle_id = $1 AND bp.left_tick IS NULL
		 FOR UPDATE OF bp`,
		battleID, defenseTick, worldID, q, r,
	)
	if err != nil {
		return false, fmt.Errorf("battle tick: load participants: %w", err)
	}
	type row struct {
		p              battleParticipant
		initialSize    int
		standingOrders []byte
	}
	var loaded []row
	for rows.Next() {
		var rr row
		if scanErr := rows.Scan(&rr.p.unitID, &rr.p.ownerID, &rr.p.side, &rr.p.currentSize, &rr.initialSize, &rr.standingOrders, &rr.p.utype, &rr.p.hull, &rr.p.cargoUnitID, &rr.p.defenseBoost); scanErr != nil {
			rows.Close()
			return false, fmt.Errorf("battle tick: scan participant: %w", scanErr)
		}
		loaded = append(loaded, rr)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return false, err
	}

	participants := make([]battleParticipant, len(loaded))
	sizes := make([]int, len(loaded))
	initialSizes := make([]int, len(loaded))
	standingOrders := make([][]byte, len(loaded))
	bySide := map[string][]int{"attacker": nil, "defender": nil}
	for i, rr := range loaded {
		participants[i] = rr.p
		sizes[i] = rr.p.currentSize
		initialSizes[i] = rr.initialSize
		standingOrders[i] = rr.standingOrders
		bySide[rr.p.side] = append(bySide[rr.p.side], i)
	}

	// Every side commits the same fraction regardless of settlement, kharis or stance.
	participationBySide := map[string]float64{"attacker": combatParticipation, "defender": combatParticipation}

	// §8 mur-modellen: the wall's absorption budget is per BATTLE-TICK, not
	// per round — it is drawn down across this tick's battleRoundsPerTick
	// rounds and not refilled until the next tick. wallLevel=0 (field
	// battles, interception, or an unwalled settlement) makes this an
	// unconditional no-op. storm only ever matters together with an actual
	// wall (wallLevel>0) — gating on both keeps a field battle's attacking
	// unit holding storm stance (a general stance, not settlement-specific)
	// completely unaffected, same behaviour as before this slice.
	wallAbsorbBudget := wallAbsorbPerLevel * wallLevel
	stormActive := storm && wallLevel > 0
	if stormActive {
		wallAbsorbBudget /= stormWallAbsorbDivisor
	}

	for round := 1; round <= battleRoundsPerTick; round++ {
		dice := economy.NewSeededDice(battleRoundSeed(seed, tickIndex, round))

		attActive, attDice, attHits := rollSide(participants, sizes, bySide["attacker"], participationBySide["attacker"], dice)
		defActive, defDice, defHits := rollSide(participants, sizes, bySide["defender"], participationBySide["defender"], dice)

		// Wall absorption: the wall soaks the first wallAbsorbBudget of this
		// tick's remaining budget out of attHits before they become defender
		// losses — a shield, not a strength multiplier (beslut 7). HitsCaused
		// below still reports the RAW dice outcome (attHits); LossesReceived
		// reflects what the wall let through, so the log itself shows the
		// wall's effect rather than hiding it.
		effectiveAttHits := attHits
		if wallAbsorbBudget > 0 {
			absorbed := attHits
			if absorbed > wallAbsorbBudget {
				absorbed = wallAbsorbBudget
			}
			effectiveAttHits -= absorbed
			wallAbsorbBudget -= absorbed
		}
		// Storm bleeds the storming attacker harder in exchange for the
		// halved wall budget above — applied to the defender's hits against
		// the attacker, never the other direction (behåll andemeningen i
		// gamla stormWallDivisor + att storm blöder mer).
		effectiveDefHits := defHits
		if stormActive {
			effectiveDefHits = int(math.Round(float64(defHits) * stormAttackerLossMultiplier))
		}

		defLosses := distributeLosses(sizes, bySide["defender"], effectiveAttHits)
		attLosses := distributeLosses(sizes, bySide["attacker"], effectiveDefHits)

		for idx, loss := range attLosses {
			sizes[idx] -= loss
		}
		for idx, loss := range defLosses {
			sizes[idx] -= loss
		}

		attResult := BattleSideRoundResult{ActiveCombatants: attActive, DiceRolled: attDice, HitsCaused: attHits, LossesReceived: sumIntMap(attLosses)}
		defResult := BattleSideRoundResult{ActiveCombatants: defActive, DiceRolled: defDice, HitsCaused: defHits, LossesReceived: sumIntMap(defLosses)}

		attJSON, _ := json.Marshal(attResult)
		defJSON, _ := json.Marshal(defResult)
		if _, err := tx.Exec(ctx,
			`INSERT INTO battle_rounds (battle_id, tick_index, round_index, attacker, defender)
			 VALUES ($1,$2,$3,$4::jsonb,$5::jsonb)
			 ON CONFLICT (battle_id, tick_index, round_index) DO NOTHING`,
			battleID, tickIndex, round, attJSON, defJSON,
		); err != nil {
			return false, fmt.Errorf("battle tick: insert round %d: %w", round, err)
		}

		if _, err := h.eventStore.Append(ctx, battleID, events.StreamCombat, EventBattleRoundResolved,
			BattleRoundResolvedPayload{BattleID: battleID, TickIndex: tickIndex, RoundIndex: round, Attacker: attResult, Defender: defResult},
			worldID, nil,
		); err != nil {
			slog.Warn("battle tick: append BattleRoundResolved failed", "battle", battleID, "round", round, "err", err)
		}

		if sumSizes(sizes, bySide["attacker"]) <= 0 || sumSizes(sizes, bySide["defender"]) <= 0 {
			break // no point rolling further rounds this tick — a side is wiped
		}
	}

	// ── Apply final sizes for this tick. ──
	for i, p := range participants {
		if sizes[i] == p.currentSize {
			continue
		}
		if sizes[i] <= 0 {
			if _, err := tx.Exec(ctx,
				`UPDATE units SET status = 'disbanded', size = 0, updated_at = now() WHERE id = $1`, p.unitID,
			); err != nil {
				return false, fmt.Errorf("battle tick: disband %s: %w", p.unitID, err)
			}
			if _, err := tx.Exec(ctx,
				`UPDATE battle_participants SET current_size = 0, left_tick = $3 WHERE battle_id = $1 AND unit_id = $2`,
				battleID, p.unitID, tickIndex,
			); err != nil {
				return false, fmt.Errorf("battle tick: mark %s left: %w", p.unitID, err)
			}
		} else {
			if _, err := tx.Exec(ctx,
				`UPDATE units SET size = $2, updated_at = now() WHERE id = $1`, p.unitID, sizes[i],
			); err != nil {
				return false, fmt.Errorf("battle tick: update size %s: %w", p.unitID, err)
			}
			if _, err := tx.Exec(ctx,
				`UPDATE battle_participants SET current_size = $3 WHERE battle_id = $1 AND unit_id = $2`,
				battleID, p.unitID, sizes[i],
			); err != nil {
				return false, fmt.Errorf("battle tick: update participant size %s: %w", p.unitID, err)
			}
		}
		// No settlement population is deducted here, deliberately. A soldier
		// leaves settlements.population at recruitment (C2, 52fa1c5,
		// 2026-06-15) and returns to it at disband (6e65b0a, 2026-09-01);
		// between those two moments he is counted in units.size/crew and
		// nowhere else. Dying means he simply never returns — the settlement
		// column must not move.
		//
		// Until 2026-09-01 this loop ran `UPDATE settlements SET population =
		// GREATEST(50, population - popLost) WHERE owner_id = $1 AND
		// is_capital = true`. That was Variant B's leftover (c618974,
		// 2026-06-05), written when recruitment did NOT cost population and
		// the demographic price of war therefore had to be charged at death
		// instead. C2 made the draft physical ten days later and this half was
		// never revisited, so every battle round billed the loss twice — the
		// second time against the owner's CAPITAL regardless of which
		// settlement raised the unit or where the fight was, and for a naval
		// participant against `size` (hulls) rather than `crew` (men).
		// Same root as the disband bug, opposite sign
		// (megaron_plan_disband_returnerar_folket.md §6).
		//
		// War's cost in civilians is not dropped, it is charged where it is
		// modelled: the sack (occupation.go's sackPopLossFraction) takes a
		// fraction of the SACKED city's population. Losses stay legible to the
		// player through the stridsrapport's pop_lost, which is
		// size_before - size_after (battleReportUnit below) — men lost from
		// the unit, which is what actually happened.
	}
	for side, part := range participationBySide {
		if _, err := tx.Exec(ctx,
			`UPDATE battle_participants SET last_participation = $3
			 WHERE battle_id = $1 AND side = $2 AND left_tick IS NULL`,
			battleID, side, part,
		); err != nil {
			slog.Warn("battle tick: could not persist last_participation", "battle", battleID, "side", side, "err", err)
		}
	}

	attTotal := sumSizes(sizes, bySide["attacker"])
	defTotal := sumSizes(sizes, bySide["defender"])
	wiped := attTotal <= 0 || defTotal <= 0

	// §5 rout (LOCKED mechanic): a side that has fallen to/below its rout
	// threshold breaks and leaves the battle WITH its survivors — the whole
	// point of rout is that it is NOT annihilation. Only checked when neither
	// side is already wiped (a wipe is unambiguously annihilation, not a
	// retreat). The side's representative supplies its standing order. Without
	// an override the threshold is fixed; hold_to_last_man disables rout.
	//
	// Write path: SetStandingOrders (standing_orders_set.go) — a mid-battle
	// retreat order via the HTTP handler/keryx/courier, same shape as
	// SetStance. Only reachable while the unit is an active participant.
	// A participant row is born with its owner's realm-wide retreat default
	// (retreat_default.go, mig 145); '{}' — the default's own default — still
	// falls through to the fixed threshold.
	attRouted, defRouted := false, false
	if !wiped {
		attRouted = sideRouts(bySide["attacker"], initialSizes, sizes, standingOrders)
		defRouted = sideRouts(bySide["defender"], initialSizes, sizes, standingOrders)
	}
	ended := wiped || attRouted || defRouted

	if !ended {
		if err := h.witnessSunkPassengers(ctx, tx, worldID, battleID, q, r, tickIndex, participants); err != nil {
			return false, err
		}
		if _, err := tx.Exec(ctx, `UPDATE battles SET current_tick = $2 WHERE id = $1`, battleID, tickIndex); err != nil {
			return false, fmt.Errorf("battle tick: update current_tick: %w", err)
		}
		return false, nil
	}

	terminationReason := "annihilation"
	winner := ""
	if wiped {
		if attTotal > 0 {
			winner = "attacker"
		} else if defTotal > 0 {
			winner = "defender"
		}
	} else {
		terminationReason = "rout"
		switch {
		case attRouted && !defRouted:
			winner = "defender"
		case defRouted && !attRouted:
			winner = "attacker"
		default:
			winner = "" // both routed the same tick — mutual retreat, no winner
		}
		// Mark the routed side's still-active participants as having left the
		// battle WITHOUT zeroing them — they keep this tick's sizes[i] (already
		// persisted to units.size/battle_participants.current_size above). A
		// participant that hit 0 THIS tick was a casualty, not a retreat —
		// its left_tick is already set by the apply-final-sizes loop, so
		// markSideRouted skips anyone with sizes[i] <= 0.
		if attRouted {
			if err := markSideRouted(ctx, tx, battleID, tickIndex, participants, sizes, bySide["attacker"]); err != nil {
				return false, fmt.Errorf("battle tick: mark attacker routed: %w", err)
			}
		}
		if defRouted {
			if err := markSideRouted(ctx, tx, battleID, tickIndex, participants, sizes, bySide["defender"]); err != nil {
				return false, fmt.Errorf("battle tick: mark defender routed: %w", err)
			}
		}
	}

	// §8 erövring (megaron_plan_erovring.md S1/S2): a battle against a
	// SETTLEMENT that ends with the attacker winning does not just
	// annihilate/rout the garrison — the city falls under occupation. This is
	// the cutover-mur gap flagged in this file's header comment. Looked up
	// here (not earlier) because it only matters once winner is final; field
	// battles and interception have no settlement at (q,r) and cityAtHex is
	// nil for them, leaving terminationReason/winner exactly as computed
	// above, unchanged from before this slice.
	cityAtHex, cityErr := loadCitySettlementForUpdate(ctx, tx, worldID, q, r)
	if cityErr != nil {
		return false, fmt.Errorf("battle tick: load city at hex: %w", cityErr)
	}
	if cityAtHex != nil && winner == "attacker" && (cityAtHex.state == "active" || cityAtHex.state == "occupied") {
		terminationReason = "attacker_reached_city"
	}

	if _, err := tx.Exec(ctx,
		`UPDATE battles SET status = 'ended', termination_reason = $2, current_tick = $3 WHERE id = $1`,
		battleID, terminationReason, tickIndex,
	); err != nil {
		return false, fmt.Errorf("battle tick: end battle: %w", err)
	}

	if _, err := h.eventStore.Append(ctx, battleID, events.StreamCombat, EventBattleEnded,
		BattleEndedPayload{
			BattleID: battleID, WorldID: worldID, Q: q, R: r, EndedTick: tickIndex,
			TerminationReason: terminationReason, Winner: winner,
			AttackerSurvivors: attTotal, DefenderSurvivors: defTotal,
		}, worldID, nil,
	); err != nil {
		slog.Warn("battle tick: append BattleEnded failed", "battle", battleID, "err", err)
	}

	// §8 erövring S1/S2 continued: attacker took the city → occupy it under
	// the winning side's owner. defender held an ALREADY-occupied city →
	// reset the annex countdown (the contestable async window). Neither
	// branch fires for a plain field/interception battle (cityAtHex nil) or
	// for a normal (non-occupied) settlement defence that simply held
	// (cityAtHex.state == "active" and winner == "defender" — nothing new to
	// do, same as before this slice).
	if cityAtHex != nil {
		switch {
		case terminationReason == "attacker_reached_city":
			var survivingAttackers []uuid.UUID
			for _, i := range bySide["attacker"] {
				if sizes[i] > 0 {
					survivingAttackers = append(survivingAttackers, participants[i].unitID)
				}
			}
			occupantOwnerID := participants[bySide["attacker"][0]].ownerID
			if err := h.occupySettlement(ctx, tx, worldID, q, r, tickIndex, cityAtHex, occupantOwnerID, survivingAttackers); err != nil {
				return false, fmt.Errorf("battle tick: occupy settlement: %w", err)
			}
		case winner == "defender" && cityAtHex.state == "occupied":
			if err := h.resetOccupationDefense(ctx, tx, worldID, tickIndex, cityAtHex); err != nil {
				return false, fmt.Errorf("battle tick: reset occupation defense: %w", err)
			}
		}
	}

	// megaron_plan_skeppsreparation.md Slice B point 2/3: drawn exactly once,
	// here, only on the branch that just flipped the battle to 'ended' (the
	// early `if !ended { return false, nil }` above already sent every
	// re-poll of a still-active battle home without reaching this line, and
	// the very top of resolveTick short-circuits a re-run of an
	// already-ended battle before it gets this far either — same "one
	// battle-end, one draw" idempotency guarantee as notifyBattleEnded below,
	// and for the same reason: this whole tx only commits once.
	h.applyNavalHullDamage(ctx, tx, battleID, worldID, q, r, tickIndex, participants, initialSizes, sizes, bySide, attRouted, defRouted)

	if err := h.witnessSunkPassengers(ctx, tx, worldID, battleID, q, r, tickIndex, participants); err != nil {
		return false, err
	}
	h.notifyBattleEnded(ctx, tx, battleID, worldID, q, r, tickIndex, winner)

	return true, nil
}

// battleReportUnit is one side's aggregated own_unit/enemy_unit shape in the
// stridsrapport notification payload (megaron_plan_stridsrapport.md §4,
// adapted to KR3's persistent battle: aggregated over ALL participants that
// were ever on that side, not a single unit — a battle can hold several
// units per side once other entry points are cut over, plan §8).
type battleReportUnit struct {
	Type       string `json:"type"`
	SizeBefore int    `json:"size_before"`
	SizeAfter  int    `json:"size_after"`
	PopLost    int    `json:"pop_lost,omitempty"`
}

type battleParticipantSummary struct {
	ownerID     uuid.UUID
	side        string
	utype       string
	initialSize int
	currentSize int
}

// notifyBattleEnded is stridsrapport's S1 (megaron_plan_stridsrapport.md),
// adapted: the plan's skeleton targeted the old one-shot
// unit_arrival_field.go NotifyPlayer calls, but KR3 removed those entirely —
// a field battle now resolves silently here, in BattleTickHandler, which is
// why the todo calls it urgent ("strider tysta live"). Every owner on BOTH
// sides gets a notification naming a representative opponent, their own
// aggregated losses and the opposing side's aggregated size — full symmetry
// per the plan's §3 default (no Timothy override on file). Kind is
// "BattleWon"/"BattleLost" rather than the plan's field-specific
// "FieldBattleWon"/"FieldBattleLost": this handler is shared by every
// initiateOrJoinBattle entry point, including the three not yet cut over
// (plan §8) — naming it generically avoids a rename once they are.
// Best-effort: a query failure here must never fail the battle-tick tx.
func (h *BattleTickHandler) notifyBattleEnded(ctx context.Context, tx pgx.Tx, battleID, worldID uuid.UUID, q, r, tickIndex int, winner string) {
	if h.hub == nil {
		return
	}

	rows, err := tx.Query(ctx,
		`SELECT bp.owner_id, bp.side, u.type, bp.initial_size, bp.current_size
		 FROM battle_participants bp JOIN units u ON u.id = bp.unit_id
		 WHERE bp.battle_id = $1
		 ORDER BY bp.joined_tick, bp.unit_id`,
		battleID,
	)
	if err != nil {
		slog.Warn("notify battle ended: load participants", "battle", battleID, "err", err)
		return
	}
	var summaries []battleParticipantSummary
	for rows.Next() {
		var s battleParticipantSummary
		if scanErr := rows.Scan(&s.ownerID, &s.side, &s.utype, &s.initialSize, &s.currentSize); scanErr == nil {
			summaries = append(summaries, s)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(summaries) == 0 {
		return
	}

	type ownerAgg struct {
		types      map[string]bool
		sizeBefore int
		sizeAfter  int
	}
	type sideAgg struct {
		ownerOrder []uuid.UUID
		byOwner    map[uuid.UUID]*ownerAgg
		types      map[string]bool
		sizeBefore int
		sizeAfter  int
	}
	sides := map[string]*sideAgg{
		"attacker": {byOwner: map[uuid.UUID]*ownerAgg{}, types: map[string]bool{}},
		"defender": {byOwner: map[uuid.UUID]*ownerAgg{}, types: map[string]bool{}},
	}
	for _, s := range summaries {
		side := sides[s.side]
		if side == nil {
			continue
		}
		side.sizeBefore += s.initialSize
		side.sizeAfter += s.currentSize
		side.types[s.utype] = true
		agg, ok := side.byOwner[s.ownerID]
		if !ok {
			agg = &ownerAgg{types: map[string]bool{}}
			side.byOwner[s.ownerID] = agg
			side.ownerOrder = append(side.ownerOrder, s.ownerID)
		}
		agg.sizeBefore += s.initialSize
		agg.sizeAfter += s.currentSize
		agg.types[s.utype] = true
	}

	typeLabel := func(types map[string]bool) string {
		if len(types) == 1 {
			for t := range types {
				return t
			}
		}
		return "mixed"
	}

	var place *string
	if name, ok := settlementNameAt(ctx, tx, worldID, q, r); ok {
		place = &name
	}

	nameByOwner := loadOwnerNames(ctx, tx, summaries)

	for side, agg := range sides {
		opposing := "defender"
		if side == "defender" {
			opposing = "attacker"
		}
		oppSide := sides[opposing]
		if len(agg.ownerOrder) == 0 || len(oppSide.ownerOrder) == 0 {
			continue // one side had no participants at all — nothing to report to/about
		}
		opponentName := nameByOwner[oppSide.ownerOrder[0]]
		enemyUnit := battleReportUnit{Type: typeLabel(oppSide.types), SizeBefore: oppSide.sizeBefore, SizeAfter: oppSide.sizeAfter}

		outcome := "mutual_wipe"
		if winner != "" {
			outcome = "attacker_wins"
			if winner == "defender" {
				outcome = "defender_holds"
			}
		}
		kind, level := "BattleLost", 2
		if winner == side {
			kind, level = "BattleWon", 3
		}

		for _, ownerID := range agg.ownerOrder {
			own := agg.byOwner[ownerID]
			payload := map[string]any{
				"role":          side,
				"outcome":       outcome,
				"opponent_name": opponentName,
				"own_unit":      battleReportUnit{Type: typeLabel(own.types), SizeBefore: own.sizeBefore, SizeAfter: own.sizeAfter, PopLost: own.sizeBefore - own.sizeAfter},
				"enemy_unit":    enemyUnit,
				"q":             q,
				"r":             r,
			}
			if place != nil {
				payload["place"] = *place
			}
			if err := h.hub.NotifyPlayer(ctx, worldID, ownerID, kind, level, payload); err != nil {
				slog.Warn("notify battle ended", "battle", battleID, "owner", ownerID, "err", err)
			}
		}
	}
}

// queryer is satisfied by both pgx.Tx and *pgxpool.Pool — settlementNameAt is
// called from within an open battle-tick tx (battle.go) and, for avsiktslagret
// S4's interception notification (unit_intercept_scan.go), after that
// handler's own tx has already committed, so it needs to work with either.
type queryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// settlementNameAt best-effort resolves a hex to its settlement's name — a
// battle can also happen on open ground (unit_arrival_field.go), where there
// is none; ok=false then and the caller falls back to bare q/r.
func settlementNameAt(ctx context.Context, q queryer, worldID uuid.UUID, hq, hr int) (string, bool) {
	var name string
	err := q.QueryRow(ctx,
		`SELECT s.name FROM settlements s JOIN provinces p ON p.id = s.province_id
		 WHERE p.world_id = $1 AND p.map_q = $2 AND p.map_r = $3`,
		worldID, hq, hr,
	).Scan(&name)
	return name, err == nil
}

// ownerNameOf resolves one player's public display name — COALESCE(wanax_name,
// username), same pattern as kingdom.go and loadOwnerNames below.
func ownerNameOf(ctx context.Context, q queryer, ownerID uuid.UUID) string {
	var name string
	if err := q.QueryRow(ctx,
		`SELECT COALESCE(wanax_name, username) FROM players WHERE id = $1`, ownerID,
	).Scan(&name); err != nil {
		return ""
	}
	return name
}

// loadOwnerNames resolves every distinct owner_id in summaries to
// COALESCE(wanax_name, username) — same pattern as kingdom.go.
func loadOwnerNames(ctx context.Context, tx pgx.Tx, summaries []battleParticipantSummary) map[uuid.UUID]string {
	seen := map[uuid.UUID]bool{}
	var ids []uuid.UUID
	for _, s := range summaries {
		if !seen[s.ownerID] {
			seen[s.ownerID] = true
			ids = append(ids, s.ownerID)
		}
	}
	out := map[uuid.UUID]string{}
	rows, err := tx.Query(ctx,
		`SELECT id, COALESCE(wanax_name, username) FROM players WHERE id = ANY($1)`, ids,
	)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var name string
		if scanErr := rows.Scan(&id, &name); scanErr == nil {
			out[id] = name
		}
	}
	return out
}

// standingOrders is one participant's battle_participants.standing_orders
// (§5/§7, migration 114) parsed. Both fields are optional — an empty/missing
// JSONB object ('{}', the column's DB default) means "no override, use the
// fixed default and never hold to the last man".
type standingOrdersFields struct {
	RetreatAtLoss *float64 `json:"retreat_at_loss"`
	HoldToLastMan bool     `json:"hold_to_last_man"`
}

func parseStandingOrders(raw []byte) standingOrdersFields {
	var so standingOrdersFields
	if len(raw) == 0 {
		return so
	}
	_ = json.Unmarshal(raw, &so) // malformed/empty → zero value, same as "no override"
	return so
}

// sideRouts checks remaining strength after this tick's losses. The side's
// representative participant (idx[0]) supplies its standing order; an absent
// override uses the fixed remaining-strength threshold.
// hold_to_last_man on the representative disables rout for the WHOLE side —
// a side either has a standing order to break, or it doesn't; this is not a
// per-unit partial-secession model.
func sideRouts(idx []int, initialSizes, sizes []int, standingOrders [][]byte) bool {
	if len(idx) == 0 {
		return false
	}
	orders := parseStandingOrders(standingOrders[idx[0]])
	if orders.HoldToLastMan {
		return false
	}
	threshold := combatRoutFraction
	if orders.RetreatAtLoss != nil {
		threshold = *orders.RetreatAtLoss
	}
	startTotal, curTotal := 0, 0
	for _, i := range idx {
		startTotal += initialSizes[i]
		curTotal += sizes[i]
	}
	if startTotal <= 0 || curTotal <= 0 {
		return false // curTotal<=0 is a wipe, handled separately — not a rout
	}
	return float64(curTotal)/float64(startTotal) <= threshold
}

// markSideRouted takes every still-active (sizes[i] > 0) participant of idx
// out of the battle — left_tick set, size UNCHANGED (they survive and are no
// longer part of this battle; §5 "reträtt med överlevande, inte förintelse").
// A participant with sizes[i] <= 0 is skipped: it hit zero as a casualty this
// same tick (apply-final-sizes above already disbanded it and set its
// left_tick), not as part of the rout.
func markSideRouted(ctx context.Context, tx pgx.Tx, battleID uuid.UUID, tickIndex int, participants []battleParticipant, sizes []int, idx []int) error {
	for _, i := range idx {
		if sizes[i] <= 0 {
			continue
		}
		if _, err := tx.Exec(ctx,
			`UPDATE battle_participants SET left_tick = $3 WHERE battle_id = $1 AND unit_id = $2 AND left_tick IS NULL`,
			battleID, participants[i].unitID, tickIndex,
		); err != nil {
			return fmt.Errorf("mark %s routed: %w", participants[i].unitID, err)
		}
	}
	return nil
}

// rollSide rolls one side's dice for one round: each side's PARTICIPATION-sampled
// active combatants roll battleDiceMultiplier(utype) T12 each; a 12 (p=1/12) is a hit.
//
// A naval combatant's dice are additionally scaled by hull/hullMax
// (megaron_plan_skeppsreparation.md §Slice B point 4, "hull skalar
// stridsstyrkan") — a ship carrying damage into a fight hits proportionally
// softer, which is what makes the repair sink (Slice C) worth spending
// timber on. activeCombatants itself is NOT scaled (it still reflects hulls
// present, not damage — sideRouts and the participation formula both read
// sizes/initialSizes directly, never this return value's dice count).
func rollSide(participants []battleParticipant, sizes []int, idx []int, part float64, dice economy.Dice) (activeCombatants, diceTotal, hits int) {
	for _, i := range idx {
		active := int(math.Round(float64(sizes[i]) * part))
		if active > sizes[i] {
			active = sizes[i]
		}
		if active < 0 {
			active = 0
		}
		activeCombatants += active
		diceCount := active * battleDiceMultiplier(participants[i].utype)
		if participants[i].utype == "spearman" {
			diceCount = (active + 1) / 2
		}
		if participants[i].defenseBoost {
			diceCount = int(math.Round(float64(diceCount) * GarrisonDefenseFactor))
		}
		if unit.CategoryOf(unit.Type(participants[i].utype)) == unit.CategoryNaval {
			diceCount = int(math.Round(float64(diceCount) * float64(participants[i].hull) / float64(hullMax)))
		}
		diceTotal += diceCount
		for d := 0; d < diceCount; d++ {
			if dice.Intn(battleDiceSides) == battleDiceSides-1 { // rolled a 12
				hits++
			}
		}
	}
	return
}

// distributeLosses spreads totalHits casualties across idx's participants
// proportionally to their live size share (largest-remainder rounding, so the
// sum of returned losses is exactly min(totalHits, Σsize) — a side can never
// lose more men than it has).
func distributeLosses(sizes []int, idx []int, totalHits int) map[int]int {
	losses := map[int]int{}
	if totalHits <= 0 || len(idx) == 0 {
		return losses
	}
	total := 0
	for _, i := range idx {
		total += sizes[i]
	}
	if total <= 0 {
		return losses
	}
	if totalHits > total {
		totalHits = total
	}

	type share struct {
		idx  int
		frac float64
	}
	shares := make([]share, 0, len(idx))
	assigned := 0
	for _, i := range idx {
		exact := float64(totalHits) * float64(sizes[i]) / float64(total)
		base := int(math.Floor(exact))
		losses[i] = base
		assigned += base
		shares = append(shares, share{idx: i, frac: exact - float64(base)})
	}
	remainder := totalHits - assigned
	sort.Slice(shares, func(a, b int) bool { return shares[a].frac > shares[b].frac })
	for k := 0; k < remainder && k < len(shares); k++ {
		losses[shares[k].idx]++
	}
	return losses
}

func sumIntMap(m map[int]int) int {
	s := 0
	for _, v := range m {
		s += v
	}
	return s
}

func sumSizes(sizes []int, idx []int) int {
	s := 0
	for _, i := range idx {
		s += sizes[i]
	}
	return s
}

// Select rescue ships after ALL final hull draws, so the chosen survivor cannot
// sink later in this same resolution. Ongoing battles also witness size deaths.
func (h *BattleTickHandler) witnessSunkPassengers(ctx context.Context, tx pgx.Tx, world, battle uuid.UUID, q, r, tick int, participants []battleParticipant) error {
	for _, p := range participants {
		if unit.CategoryOf(unit.Type(p.utype)) != unit.CategoryNaval {
			continue
		}
		var dead bool
		if err := tx.QueryRow(ctx, `SELECT status='disbanded' OR size<=0 OR hull<=0 FROM units WHERE id=$1`, p.unitID).Scan(&dead); err != nil {
			return err
		}
		if !dead {
			continue
		}
		var rescue uuid.UUID
		err := tx.QueryRow(ctx, `SELECT u.id FROM battle_participants bp JOIN units u ON u.id=bp.unit_id WHERE bp.battle_id=$1 AND bp.side!=$2 AND (bp.left_tick IS NULL OR bp.left_tick=$3) AND u.category='naval' AND u.status!='disbanded' AND u.size>0 AND u.hull>0 ORDER BY u.id LIMIT 1 FOR UPDATE OF u`, battle, p.side, tick).Scan(&rescue)
		var chosen *uuid.UUID
		if err == nil {
			chosen = &rescue
		} else if err != pgx.ErrNoRows {
			return err
		}
		witnesses, err := carrier.OutcomeTx(ctx, tx, h.eventStore, world, p.unitID, chosen, "battle", q, r, tick)
		if err != nil {
			return err
		}
		if h.carrierOutcomes != nil {
			*h.carrierOutcomes = append(*h.carrierOutcomes, witnesses...)
		}
	}
	return nil
}
