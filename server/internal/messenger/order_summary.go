package messenger

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// OwnOrderSummary reads only an envelope owned by the requesting Wanax. It is
// presentation of a sealed order, never a request to execute or reinterpret it.
func OwnOrderSummary(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, world, sender, messengerID uuid.UUID) string {
	var text string
	var raw []byte
	if err := db.QueryRow(ctx, `SELECT message_text,order_payload FROM messengers WHERE id=$1 AND world_id=$2 AND sender_id=$3 AND kind='order'`, messengerID, world, sender).Scan(&text, &raw); err != nil {
		return ""
	}
	var p OrderDeliveryPayload
	if len(raw) > 0 && json.Unmarshal(raw, &p) != nil {
		return ""
	}
	if p.PlayerID != uuid.Nil && p.PlayerID != sender {
		return ""
	}
	subject := "Your order"
	if p.UnitID != uuid.Nil {
		var actualOwner uuid.UUID
		if db.QueryRow(ctx, `SELECT owner_id FROM units WHERE id=$1 AND world_id=$2`, p.UnitID, world).Scan(&actualOwner) == nil && actualOwner == sender {
			if name := unit.LoadDisplayName(ctx, db, p.UnitID); name != "" {
				subject = name
			}
		}
	}
	detail := strings.TrimSpace(text)
	if p.March != nil {
		detail = fmt.Sprintf("March to (%d,%d)", p.March.TargetQ, p.March.TargetR)
		if p.March.Intent != "" {
			detail += "; mission: " + p.March.Intent
		}
		if p.March.Name != "" {
			detail += "; colony: " + p.March.Name
		}
	} else if p.Recall != nil && p.Recall.Mode == "redirect" && p.Recall.NewTargetQ != nil && p.Recall.NewTargetR != nil {
		detail = fmt.Sprintf("Redirect to (%d,%d)", *p.Recall.NewTargetQ, *p.Recall.NewTargetR)
	} else if p.Stance != nil {
		detail = "Set stance: " + p.Stance.Stance
	}
	if detail == "" {
		return ""
	}
	return subject + ": " + detail
}
