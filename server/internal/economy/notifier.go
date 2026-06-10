package economy

import (
	"context"

	"github.com/google/uuid"
)

// Broadcaster is the push-notification interface used by economy handlers.
// *notify.Hub satisfies this via its BroadcastEvent and NotifyPlayer methods.
type Broadcaster interface {
	BroadcastEvent(worldID uuid.UUID, kind string, payload any)
	NotifyPlayer(ctx context.Context, worldID, playerID uuid.UUID, kind string, level int, payload any) error
}
