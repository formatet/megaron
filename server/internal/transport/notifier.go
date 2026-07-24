package transport

import (
	"context"

	"github.com/google/uuid"
)

// Broadcaster is the push-notification interface used by transport handlers.
// *notify.Hub satisfies this via its BroadcastEvent and NotifyPlayer methods.
// transport sits below notify in the G1 dependency order (CLAUDE.md), so it
// cannot import notify directly — same pattern as economy.Broadcaster and
// combat.Broadcaster (internal/economy/notifier.go, internal/combat/notifier.go).
type Broadcaster interface {
	BroadcastEvent(worldID uuid.UUID, kind string, payload any)
	NotifyPlayer(ctx context.Context, worldID, playerID uuid.UUID, kind string, level int, payload any) error
}
