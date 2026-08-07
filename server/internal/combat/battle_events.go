package combat

import "github.com/google/uuid"

// KR3 event family (megaron_plan_kr3_stridssystem.md §8) — a NEW, frozen set
// of event types for the persistent multi-tick battle model. The old
// UnitCombatResolved / FieldBattleWon|Lost one-shot events are left exactly
// as they are for old data and for avsiktslagret's unit_intercept_scan.go,
// which is deliberately NOT rewired to initiateOrJoinBattle (it resolves
// synchronously by design — see its own file comment). Settlement siege and
// amphibious assault were the last two entry points still on the old
// one-shot events; both now go through this family too (2026-08-07).
// Semantics here are frozen forever per CLAUDE.md: never reinterpret a field,
// add a V2 type instead.
//
// Stream: events.StreamCombat, stream ID = battles.id (one stream per battle,
// distinct from the per-unit unit.StreamUnit streams the old events use).
// All payloads record OUTCOMES of an already-happened dice roll, never a
// pending check (CLAUDE.md events rule).
const (
	EventBattleStarted         = "BattleStarted"
	EventUnitJoinedBattle      = "UnitJoinedBattle"
	EventBattleRoundResolved   = "BattleRoundResolved"
	EventBattleEnded           = "BattleEnded"
	EventStandingOrdersChanged = "StandingOrdersChanged"
)

// BattleParticipantRef identifies one unit's contribution at the moment it
// enters a battle (initiation or a later join) — the outcome of loading that
// unit's row, not a live reference.
type BattleParticipantRef struct {
	UnitID      uuid.UUID `json:"unit_id"`
	OwnerID     uuid.UUID `json:"owner_id"`
	UnitType    string    `json:"unit_type"`
	Side        string    `json:"side"` // attacker|defender
	InitialSize int       `json:"initial_size"`
}

// BattleStartedPayload is emitted once, when initiateOrJoinBattle creates a
// new battles row (as opposed to an arrival joining an already-active one —
// that emits UnitJoinedBattlePayload instead).
type BattleStartedPayload struct {
	BattleID    uuid.UUID `json:"battle_id"`
	WorldID     uuid.UUID `json:"world_id"`
	Q           int       `json:"q"`
	R           int       `json:"r"`
	StartedTick int       `json:"started_tick"`
	Seed        int64     `json:"seed"`
	// WallLevel/Storm (migration 116, §8 mur-modellen): snapshotted once here,
	// same stability guarantee as Seed. WallLevel is 0 for field battles and
	// interception (no settlement at their hex) — additive fields, old
	// BattleStarted rows simply decode them as the zero value.
	WallLevel int                    `json:"wall_level"`
	Storm     bool                   `json:"storm"`
	Attackers []BattleParticipantRef `json:"attackers"`
	Defenders []BattleParticipantRef `json:"defenders"`
}

// UnitJoinedBattlePayload is emitted when a unit joins an ALREADY ACTIVE
// battle (§6) rather than starting a new one.
type UnitJoinedBattlePayload struct {
	BattleID   uuid.UUID `json:"battle_id"`
	UnitID     uuid.UUID `json:"unit_id"`
	OwnerID    uuid.UUID `json:"owner_id"`
	UnitType   string    `json:"unit_type"`
	Side       string    `json:"side"` // attacker|defender
	Size       int       `json:"size"`
	JoinedTick int       `json:"joined_tick"`
}

// BattleSideRoundResult is one side's outcome for one round — the same shape
// stored in battle_rounds.attacker/defender (JSONB).
type BattleSideRoundResult struct {
	ActiveCombatants int `json:"active_combatants"` // Σ participation-sampled size on this side
	DiceRolled       int `json:"dice_rolled"`       // Σ T12 rolled (elite ×3/man, chariot ×5/vagn)
	HitsCaused       int `json:"hits_caused"`       // Σ rolls of exactly 12 (p=1/12) — casualties inflicted on the OTHER side
	LossesReceived   int `json:"losses_received"`   // casualties this side actually took this round
}

// BattleRoundResolvedPayload is emitted once per resolved round (battleRoundsPerTick
// times per battle-tick that runs to completion). Both sides' dice are rolled
// and applied simultaneously (§4), so one event covers both sides of the round.
type BattleRoundResolvedPayload struct {
	BattleID   uuid.UUID             `json:"battle_id"`
	TickIndex  int                   `json:"tick_index"`
	RoundIndex int                   `json:"round_index"`
	Attacker   BattleSideRoundResult `json:"attacker"`
	Defender   BattleSideRoundResult `json:"defender"`
}

// BattleEndedPayload is emitted exactly once, when a battle's status flips to
// 'ended'. TerminationReason is one of the frozen enum values checked by the
// battles.termination_reason CHECK constraint (migration 114); this slice
// only ever produces "annihilation" — the other reasons (rout, retreat_order,
// attacker_reached_city, no_enemy_left) are reserved for later slices (§5/§6/§7).
type BattleEndedPayload struct {
	BattleID          uuid.UUID `json:"battle_id"`
	WorldID           uuid.UUID `json:"world_id"`
	Q                 int       `json:"q"`
	R                 int       `json:"r"`
	EndedTick         int       `json:"ended_tick"`
	TerminationReason string    `json:"termination_reason"`
	Winner            string    `json:"winner"` // "attacker" | "defender" | "" (no survivors on either side)
	AttackerSurvivors int       `json:"attacker_survivors"`
	DefenderSurvivors int       `json:"defender_survivors"`
}

// StandingOrdersChangedPayload is emitted whenever a unit's mid-battle
// standing order changes (§5) — either applied instantly (garrison/distance-0)
// or on Runner delivery. Records the OUTCOME (the values now in effect), not
// the request: RetreatAtLoss nil means "no override, use the loyalty-derived
// default" (battle.go's routFractionForLoyalty), same meaning as an empty
// standing_orders JSONB object.
type StandingOrdersChangedPayload struct {
	BattleID      uuid.UUID `json:"battle_id"`
	UnitID        uuid.UUID `json:"unit_id"`
	WorldID       uuid.UUID `json:"world_id"`
	RetreatAtLoss *float64  `json:"retreat_at_loss"`
	HoldToLastMan bool      `json:"hold_to_last_man"`
}
