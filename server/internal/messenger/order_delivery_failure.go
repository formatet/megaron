package messenger

import (
	"context"
	"fmt"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/unit"
)

// OrderDeliveryFailed is an audit outcome, not a new notification kind or a
// reinterpretation of ScheduledOrderDelivery. The old messenger claim still
// permits only its first delivery to execute or report a terminal failure.
const OrderDeliveryFailed = "OrderDeliveryFailed"

func (h *OrderDeliveryHandler) failSingleRecall(ctx context.Context, p OrderDeliveryPayload, reason string) error {
	name := unit.LoadDisplayName(ctx, h.pool, p.UnitID)
	if name == "" {
		name = p.UnitID.String()
	}
	payload := map[string]any{"world_id": p.WorldID, "player_id": p.PlayerID, "unit_id": p.UnitID, "messenger_id": p.MessengerID, "name": name, "verb": p.Verb, "reason": reason}
	_, auditErr := h.eventStore.Append(ctx, p.UnitID, events.StreamType(unit.StreamUnit), OrderDeliveryFailed, payload, p.WorldID, nil)
	// Notify even if audit storage fails: the player must hear the terminal
	// outcome. Keep execution's existing one-way messenger claim unchanged.
	if h.hub != nil {
		_ = h.hub.NotifyPlayer(ctx, p.WorldID, p.PlayerID, "OrderFailed", 2, payload)
	}
	if auditErr != nil {
		return fmt.Errorf("record failed %s delivery: %w", p.Verb, auditErr)
	}
	return nil
}
