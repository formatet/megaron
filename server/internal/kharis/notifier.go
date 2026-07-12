package kharis

import (
	"context"

	"github.com/google/uuid"
)

// Broadcaster is the push-notification interface used by the kharis tick.
// Defined in the CONSUMING package (G1): kharis never imports notify upward —
// the concrete *notify.Hub is injected into NewTickHandler from main.go and
// satisfies this via its NotifyPlayer method. Mirrors combat.Broadcaster; the
// tick only needs the persistent per-player channel, so the interface is kept
// to that single method.
type Broadcaster interface {
	NotifyPlayer(ctx context.Context, worldID, playerID uuid.UUID, kind string, level int, payload any) error
}
