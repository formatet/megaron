package kharis

import (
	"context"
	"time"

	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
)

const WrathThreshold = 15.0
const WrathDailyProbability = .03
const WrathProtectionTicks = 7
const BlessThreshold = blessThreshold

// WrathStatus is owner-private. Protection is tied to the realm, not a temple.
type WrathStatus struct {
	Threshold               float64    `json:"threshold"`
	DailyProbability        float64    `json:"daily_probability"`
	ProtectionUntilTick     int        `json:"protection_until_tick,omitempty"`
	ProtectionUntil         *time.Time `json:"protection_until,omitempty"`
	ProtectionRemainingDays int        `json:"protection_remaining_days"`
}

// LoadWrathStatus reads the same durable owner-scoped shield the punishment locks.
// No founding grace is introduced; expiry is an absolute tick boundary.
func LoadWrathStatus(ctx context.Context, db tick.Queryer, player, world uuid.UUID) (WrathStatus, error) {
	s := WrathStatus{Threshold: WrathThreshold, DailyProbability: WrathDailyProbability}
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
