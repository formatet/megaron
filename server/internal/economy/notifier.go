package economy

import "github.com/google/uuid"

// Broadcaster is the push-notification interface used by economy handlers.
// *notify.Hub satisfies this via its BroadcastEvent method.
type Broadcaster interface {
	BroadcastEvent(worldID uuid.UUID, kind string, payload any)
}
