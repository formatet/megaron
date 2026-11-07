package combat

// Fågeloraklet (bönesystemet v3 steg 5d): the closing sweep. The cast already wrote into the caster's
// memory what the eye saw; one game day later this writes it once more, so what happened during the
// day is remembered even if the caster never read the map. Idempotent: the same hexes, the same
// snapshot (province.SweepLiveRadius is ON CONFLICT DO NOTHING plus a rewritten city snapshot).

import (
	"context"
	"encoding/json"
	"fmt"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BirdOracleClosePayload is the ScheduledBirdOracleClose payload.
type BirdOracleClosePayload struct {
	WorldID  uuid.UUID `json:"world_id"`
	PlayerID uuid.UUID `json:"player_id"`
	Q        int       `json:"q"`
	R        int       `json:"r"`
}

// BirdOracleCloseHandler processes ScheduledBirdOracleClose.
type BirdOracleCloseHandler struct{ pool *pgxpool.Pool }

func NewBirdOracleCloseHandler(pool *pgxpool.Pool) *BirdOracleCloseHandler {
	return &BirdOracleCloseHandler{pool: pool}
}

func (h *BirdOracleCloseHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var p BirdOracleClosePayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal bird oracle payload: %w", err)
	}
	return province.SweepLiveRadius(ctx, h.pool, p.WorldID, p.PlayerID, province.MapPosition{Q: p.Q, R: p.R}, province.EyeOracle)
}
