package handlers

// Hemerodromos live-eye (temenos_orderlopare_plan.md Fas 4, temenos_synlighet.md
// §Nivå 1): the player's own outbound messenger is a tier-1 eye interpolated
// along its courier route, seeing as a land unit.
//
// DB integration test (real Postgres, gated by DATABASE_URL).

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/province"
)

func TestLoadLiveEyes_CourierSeesAlongRoute(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'archived') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID) })

	authSvc := auth.NewService(pool, "test-secret")
	username := "eye-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}
	playerID := claims.PlayerID

	// Origin city at (0,0); a straight plains strip to (4,0) for the route.
	var provID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&provID); err != nil {
		t.Fatalf("create province: %v", err)
	}
	var settID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Capital', 'achaean', $3, 'capital', true) RETURNING id`,
		worldID, provID, playerID,
	).Scan(&settID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}
	for q := 0; q <= 4; q++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'plains')`,
			worldID, q,
		); err != nil {
			t.Fatalf("create tile (%d,0): %v", q, err)
		}
	}

	// An outbound hemerodromos halfway through its (0,0)→(4,0) run.
	now := time.Now()
	if _, err := pool.Exec(ctx,
		`INSERT INTO messengers
		     (world_id, sender_id, origin_id, destination_id, message_text, status, kind, hex_q, hex_r, dest_q, dest_r, sent_at, arrives_at)
		 VALUES ($1,$2,$3,NULL,'Hemerodromos — march order.','outbound','order',0,0,4,0,$4,$5)`,
		worldID, playerID, settID, now.Add(-30*time.Minute), now.Add(30*time.Minute),
	); err != nil {
		t.Fatalf("create outbound messenger: %v", err)
	}

	eyes := loadLiveEyes(ctx, pool, worldID, playerID, now)
	foundMidRoute := false
	for _, e := range eyes {
		if e.Kind == province.EyeLandUnit && e.Pos.Q == 2 && e.Pos.R == 0 {
			foundMidRoute = true
		}
	}
	if !foundMidRoute {
		t.Fatalf("no land-unit eye at the courier's interpolated mid-route hex (2,0); eyes = %+v", eyes)
	}
}
