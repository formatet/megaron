package auth

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

const activityInterval = 5 * time.Minute

// recordActivity observes authenticated server contact, including background
// polling. This wall-clock operational timestamp is neither game time nor proof
// of human attention, and is never exposed by public player views.
func (s *Service) recordActivity(ctx context.Context, playerID uuid.UUID) {
	if s.pool == nil {
		return
	}
	now := time.Now()
	s.activityMu.Lock()
	if next, ok := s.activityNext[playerID]; ok && now.Before(next) {
		s.activityMu.Unlock()
		return
	}
	// Expire entries lazily so inactive accounts do not accumulate in this process.
	if !now.Before(s.activitySweep) {
		for id, next := range s.activityNext {
			if !now.Before(next) {
				delete(s.activityNext, id)
			}
		}
		s.activitySweep = now.Add(activityInterval)
	}
	if s.activityNext == nil {
		s.activityNext = make(map[uuid.UUID]time.Time)
	}
	s.activityNext[playerID] = now.Add(activityInterval)
	s.activityMu.Unlock()

	// Bound telemetry latency; authentication still succeeds on any failure.
	updateCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_, err := s.pool.Exec(updateCtx, `UPDATE players SET last_seen_at=$2
 WHERE id=$1 AND (last_seen_at IS NULL OR last_seen_at <= $2::timestamptz - interval '5 minutes')`, playerID, now)
	if err != nil {
		// A canceled/failed observation must not suppress the next successful one.
		s.activityMu.Lock()
		delete(s.activityNext, playerID)
		s.activityMu.Unlock()
		slog.Warn("could not record authenticated activity", "player_id", playerID, "error", err)
	}
}
