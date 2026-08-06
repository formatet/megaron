package combat

// KR3 stridssystemet — the persistent, multi-tick battle substrate
// (megaron_plan_kr3_stridssystem.md). This file holds:
//   - the §4 participation formula (named-constant rattar, mechanic locked)
//   - §3's reproducible per-(seed,tick,round) dice derivation
//   - initiateOrJoinBattle, the replacement for the old one-shot resolve
//   - BattleTickHandler, the ScheduledBattleTick state machine (§2)
//
// Scope of THIS slice (megaron_todo.md ⚔ BYGGORDNING, 2026-08-06): only the
// field-arrival entry point (unit_arrival_field.go:resolveFieldCombat) is
// wired to initiateOrJoinBattle. The settlement resolveCombat, amphibious
// assault and avsiktslagret's unit_intercept_scan.go entry points still use
// their old one-shot resolve — rewired in a later slice (§2's own list).
// Rout, stood reträttorder and the avsiktslagret reaction_policy verbs
// (escort/alert) — plan §5/§6/§7 — are NOT implemented here either; the only
// termination this handler ever produces is "annihilation" (one side's
// participants reach 0 combined size). The other termination_reason enum
// values are reserved in the schema (migration 114) for those later slices.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/loyalty"
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

	participationMin = 0.40
	participationMax = 1.00

	// kharisParticipationCap/Weight: same normalisation reference (75) as
	// rollFortune's kharis bias, capped at ±10pp (§10 strawman). Kharis here
	// only ever HELPS: every kharis read in this file is GREATEST(0, ...) —
	// there is no negative kharis to penalise participation with.
	kharisParticipationCap    = 75
	kharisParticipationWeight = 0.10

	// fortifyParticipationBonus fills the formula's stance_modifier slot (§4):
	// a defending side holding fortify stance commits more of its strength.
	// Only checked for the defending side's participants (fortify is a
	// garrison/positioned stance, not something an attacking march holds).
	fortifyParticipationBonus = 0.10
)

// loyaltyParticipationBase is the loyalty_base term (§4 strawman 65/80/90/100%).
var loyaltyParticipationBase = map[int]float64{
	1: 0.65,
	2: 0.80,
	3: 0.90,
	4: 1.00,
}

// battleDiceMultiplier is how many T12 each ACTIVE combatant of utype rolls
// per round: elite 3/man, war_chariot ("vagn") 5/vagn, everyone else 1.
func battleDiceMultiplier(utype string) int {
	switch utype {
	case "elite_infantry":
		return 3
	case "war_chariot":
		return 5
	default:
		return 1
	}
}

func loyaltyBaseForParticipation(loyaltyLevel int) float64 {
	if v, ok := loyaltyParticipationBase[loyaltyLevel]; ok {
		return v
	}
	return loyaltyParticipationBase[2]
}

func clampParticipation(p float64) float64 {
	if p < participationMin {
		return participationMin
	}
	if p > participationMax {
		return participationMax
	}
	return p
}

// participation computes the §4 formula for one side:
//
//	participation = clamp(loyalty_base + kharis_modifier + stance_modifier, MIN, MAX)
//
// kharis is the side's own (never-negative) kharis balance; fortify is true
// when at least one of the side's participants currently holds fortify stance.
// transient_modifier (§4/§10) is a reserved future slot, always 0 here.
func participation(loyaltyLevel int, kharis float64, fortify bool) float64 {
	base := loyaltyBaseForParticipation(loyaltyLevel)
	kharisMod := math.Min(kharis/kharisParticipationCap, 1.0) * kharisParticipationWeight
	stanceMod := 0.0
	if fortify {
		stanceMod = fortifyParticipationBonus
	}
	return clampParticipation(base + kharisMod + stanceMod)
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
	unitID      uuid.UUID
	ownerID     uuid.UUID
	utype       string
	side        string // "attacker" | "defender"
	currentSize int
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

	var battleID uuid.UUID
	if err := tx.QueryRow(ctx,
		`INSERT INTO battles (world_id, q, r, started_tick, current_tick, status, seed)
		 VALUES ($1, $2, $3, $4, $4, 'active', $5) RETURNING id`,
		worldID, q, r, currentTick, seed,
	).Scan(&battleID); err != nil {
		return fmt.Errorf("start battle: insert battles row: %w", err)
	}

	all := append([]battleParticipant{arriving}, defenders...)
	var attackerRefs, defenderRefs []BattleParticipantRef
	for _, p := range all {
		if _, err := tx.Exec(ctx,
			`INSERT INTO battle_participants (battle_id, unit_id, owner_id, side, joined_tick, initial_size, current_size)
			 VALUES ($1,$2,$3,$4,$5,$6,$6)`,
			battleID, p.unitID, p.ownerID, p.side, currentTick, p.currentSize,
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

	if _, err := h.eventStore.Append(ctx, battleID, events.StreamCombat, EventBattleStarted,
		BattleStartedPayload{
			BattleID: battleID, WorldID: worldID, Q: q, R: r,
			StartedTick: currentTick, Seed: seed,
			Attackers: attackerRefs, Defenders: defenderRefs,
		}, worldID, nil,
	); err != nil {
		slog.Warn("start battle: append BattleStarted failed", "battle", battleID, "err", err)
	}

	if err := h.scheduler.EnqueueTickTx(ctx, tx, worldID, events.ScheduledBattleTick,
		battleTickPayload{BattleID: battleID}, currentTick,
	); err != nil {
		return fmt.Errorf("start battle: enqueue first tick: %w", err)
	}
	return nil
}

func (h *UnitArrivalHandler) joinBattle(
	ctx context.Context, tx pgx.Tx, worldID, battleID uuid.UUID, currentTick int, arriving battleParticipant,
) error {
	tag, err := tx.Exec(ctx,
		`INSERT INTO battle_participants (battle_id, unit_id, owner_id, side, joined_tick, initial_size, current_size)
		 VALUES ($1,$2,$3,$4,$5,$6,$6)
		 ON CONFLICT (battle_id, unit_id) DO NOTHING`,
		battleID, arriving.unitID, arriving.ownerID, arriving.side, currentTick, arriving.currentSize,
	)
	if err != nil {
		return fmt.Errorf("join battle: insert participant: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil // already a participant (idempotent re-run)
	}

	if _, err := h.eventStore.Append(ctx, battleID, events.StreamCombat, EventUnitJoinedBattle,
		UnitJoinedBattlePayload{
			BattleID: battleID, UnitID: arriving.unitID, OwnerID: arriving.ownerID,
			UnitType: arriving.utype, Side: arriving.side, Size: arriving.currentSize, JoinedTick: currentTick,
		}, worldID, nil,
	); err != nil {
		slog.Warn("join battle: append UnitJoinedBattle failed", "battle", battleID, "err", err)
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
	pool       *pgxpool.Pool
	eventStore *events.Store
	scheduler  *events.Scheduler
}

// NewBattleTickHandler creates a BattleTickHandler.
func NewBattleTickHandler(pool *pgxpool.Pool, store *events.Store, scheduler *events.Scheduler) *BattleTickHandler {
	return &BattleTickHandler{pool: pool, eventStore: store, scheduler: scheduler}
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

	ended, err := h.resolveTick(ctx, tx, payload.BattleID, e.WorldID, e.DueTick)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
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
	if err := tx.QueryRow(ctx,
		`SELECT status, q, r, seed FROM battles WHERE id = $1 FOR UPDATE`, battleID,
	).Scan(&status, &q, &r, &seed); err != nil {
		return false, fmt.Errorf("battle tick: load battle: %w", err)
	}
	if status != "active" {
		return true, nil // already ended — stale re-enqueue race, idempotent no-op
	}

	rows, err := tx.Query(ctx,
		`SELECT bp.unit_id, bp.owner_id, bp.side, bp.current_size, u.type, u.stance
		 FROM battle_participants bp JOIN units u ON u.id = bp.unit_id
		 WHERE bp.battle_id = $1 AND bp.left_tick IS NULL
		 FOR UPDATE OF bp`,
		battleID,
	)
	if err != nil {
		return false, fmt.Errorf("battle tick: load participants: %w", err)
	}
	type row struct {
		p      battleParticipant
		stance *string
	}
	var loaded []row
	for rows.Next() {
		var rr row
		if scanErr := rows.Scan(&rr.p.unitID, &rr.p.ownerID, &rr.p.side, &rr.p.currentSize, &rr.p.utype, &rr.stance); scanErr != nil {
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
	bySide := map[string][]int{"attacker": nil, "defender": nil}
	for i, rr := range loaded {
		participants[i] = rr.p
		sizes[i] = rr.p.currentSize
		bySide[rr.p.side] = append(bySide[rr.p.side], i)
	}

	// §4: participation sampled ONCE per battle-tick, stable across all
	// rounds. Representative owner per side = the first participant found for
	// it (same "defenders[0]" convention unit_arrival_field.go already used
	// for the fortune/loyalty bias).
	participationBySide := map[string]float64{}
	repOwnerBySide := map[string]uuid.UUID{}
	for _, side := range []string{"attacker", "defender"} {
		idx := bySide[side]
		if len(idx) == 0 {
			continue
		}
		ownerID := participants[idx[0]].ownerID
		repOwnerBySide[side] = ownerID

		_, loyaltyLevel, _ := supplyingSettlement(ctx, tx, ownerID, nil, worldID)

		var kharis float64
		_ = tx.QueryRow(ctx,
			`SELECT GREATEST(0, settled(kharis_amount, kharis_rate, kharis_calc_tick))
			 FROM player_world_records WHERE player_id = $1 AND world_id = $2`,
			ownerID, worldID,
		).Scan(&kharis)

		fortify := false
		for _, i := range idx {
			if loaded[i].stance != nil && *loaded[i].stance == "fortify" {
				fortify = true
				break
			}
		}
		participationBySide[side] = participation(loyaltyLevel, kharis, fortify)
	}

	for round := 1; round <= battleRoundsPerTick; round++ {
		dice := economy.NewSeededDice(battleRoundSeed(seed, tickIndex, round))

		attActive, attDice, attHits := rollSide(participants, sizes, bySide["attacker"], participationBySide["attacker"], dice)
		defActive, defDice, defHits := rollSide(participants, sizes, bySide["defender"], participationBySide["defender"], dice)

		defLosses := distributeLosses(sizes, bySide["defender"], attHits)
		attLosses := distributeLosses(sizes, bySide["attacker"], defHits)

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
		popLost := p.currentSize - sizes[i]
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
		if popLost > 0 {
			if _, err := tx.Exec(ctx,
				`UPDATE settlements SET population = GREATEST(50, population - $2)
				 WHERE owner_id = $1 AND world_id = $3 AND is_capital = true`,
				p.ownerID, popLost, worldID,
			); err != nil {
				slog.Warn("battle tick: could not apply pop loss", "unit", p.unitID, "err", err)
			}
		}
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
	ended := attTotal <= 0 || defTotal <= 0

	if !ended {
		if _, err := tx.Exec(ctx, `UPDATE battles SET current_tick = $2 WHERE id = $1`, battleID, tickIndex); err != nil {
			return false, fmt.Errorf("battle tick: update current_tick: %w", err)
		}
		return false, nil
	}

	winner := ""
	if attTotal > 0 {
		winner = "attacker"
	} else if defTotal > 0 {
		winner = "defender"
	}
	if _, err := tx.Exec(ctx,
		`UPDATE battles SET status = 'ended', termination_reason = 'annihilation', current_tick = $2 WHERE id = $1`,
		battleID, tickIndex,
	); err != nil {
		return false, fmt.Errorf("battle tick: end battle: %w", err)
	}

	if _, err := h.eventStore.Append(ctx, battleID, events.StreamCombat, EventBattleEnded,
		BattleEndedPayload{
			BattleID: battleID, WorldID: worldID, Q: q, R: r, EndedTick: tickIndex,
			TerminationReason: "annihilation", Winner: winner,
			AttackerSurvivors: attTotal, DefenderSurvivors: defTotal,
		}, worldID, nil,
	); err != nil {
		slog.Warn("battle tick: append BattleEnded failed", "battle", battleID, "err", err)
	}

	// L2 battle loyalty, once, at battle end (mirrors the old
	// resolveFieldCombat's single applyBattleLoyalty call) — one representative
	// settlement per side, same "won_battle/lost_battle/defended_settlement"
	// vocabulary as the rest of L2.
	attackerWon := winner == "attacker"
	if ownerID, ok := repOwnerBySide["attacker"]; ok {
		if settleID, _, hasSettle := supplyingSettlement(ctx, tx, ownerID, nil, worldID); hasSettle {
			delta, evType, reason := -1, "battle_lost", "lost_battle"
			if attackerWon {
				delta, evType, reason = +1, "shared_victory", "won_battle"
			}
			if err := loyalty.AppendLoyaltyEventTx(ctx, tx, h.eventStore, settleID, worldID, evType, delta, reason); err != nil {
				slog.Warn("battle tick: attacker battle loyalty failed", "settlement", settleID, "err", err)
			}
		}
	}
	if !attackerWon {
		if ownerID, ok := repOwnerBySide["defender"]; ok {
			if settleID, _, hasSettle := supplyingSettlement(ctx, tx, ownerID, nil, worldID); hasSettle {
				if err := loyalty.AppendLoyaltyEventTx(ctx, tx, h.eventStore, settleID, worldID, "shared_victory", +1, "defended_settlement"); err != nil {
					slog.Warn("battle tick: defender battle loyalty failed", "settlement", settleID, "err", err)
				}
			}
		}
	}

	return true, nil
}

// rollSide rolls one side's dice for one round: each side's PARTICIPATION-sampled
// active combatants roll battleDiceMultiplier(utype) T12 each; a 12 (p=1/12) is a hit.
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
