package kharis

import (
	"context"
	"time"

	"formatet/megaron/server/internal/religion"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
)

// The punishment curve — R2 mild (Timothy 2026-10-30, megaron_religion_talforslag_20261030 §R2):
// risk begins below WrathThreshold (Suspicious) at WrathDailyProbability and
// rises to WrathfulDailyProbability below WrathfulThreshold. Before 2026-10-30: 3 % below 15.
const WrathThreshold = religion.MoodIndifferent
const WrathDailyProbability = .02
const WrathfulThreshold = religion.MoodSuspicious
const WrathfulDailyProbability = .04
const WrathProtectionTicks = 7
const BlessThreshold = blessThreshold

// WrathStatus is owner-private. Protection is tied to the realm, not a temple.
type WrathStatus struct {
	Threshold               float64    `json:"threshold"`
	DailyProbability        float64    `json:"daily_probability"`
	WrathfulThreshold       float64    `json:"wrathful_threshold"`
	WrathfulProbability     float64    `json:"wrathful_probability"`
	ProtectionUntilTick     int        `json:"protection_until_tick,omitempty"`
	ProtectionUntil         *time.Time `json:"protection_until,omitempty"`
	ProtectionRemainingDays int        `json:"protection_remaining_days"`
}

// LoadWrathStatus reads the same durable owner-scoped shield the punishment locks.
// No founding grace is introduced; expiry is an absolute tick boundary.
func LoadWrathStatus(ctx context.Context, db tick.Queryer, player, world uuid.UUID) (WrathStatus, error) {
	s := WrathStatus{Threshold: WrathThreshold, DailyProbability: WrathDailyProbability, WrathfulThreshold: WrathfulThreshold, WrathfulProbability: WrathfulDailyProbability}
	if err := db.QueryRow(ctx, `SELECT COALESCE((SELECT (context->>'until_tick')::int FROM kharis_notice_state
 WHERE world_id=$1 AND player_id=$2 AND scope_id=$2 AND condition='divine_punishment'),0)`, world, player).Scan(&s.ProtectionUntilTick); err != nil {
		return s, err
	}
	anchor, err := tick.LoadAnchor(ctx, db, world)
	if err != nil {
		return s, err
	}
	s.ProtectionRemainingDays = max(0, s.ProtectionUntilTick-anchor.Tick)
	if s.ProtectionRemainingDays > 0 {
		at := anchor.WallAt(int64(s.ProtectionUntilTick) * 1000)
		s.ProtectionUntil = &at
	}
	return s, nil
}
