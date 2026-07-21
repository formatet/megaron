package handlers

// Return-leg hemerodromos eye (temenos_orderlopare_plan.md §(b) returbens-ögat):
// once a recipient replies, the courier runs HOME, and the sender's own eye must
// interpolate that homeward leg too — not sit blind until it arrives. Mirrors
// courier_eye_test.go but for status='returning' with a fresh return window.
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

func TestLoadLiveEyes_CourierSeesOnReturnLeg(t *testing.T) {
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
	sender := "ret-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, sender, sender+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register sender: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}
	senderID := claims.PlayerID

	other := "dst-" + uuid.New().String()
	_, _, err = authSvc.Register(ctx, other, other+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register recipient: %v", err)
	}
	var otherID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM players WHERE username = $1`, other).Scan(&otherID); err != nil {
		t.Fatalf("load recipient id: %v", err)
	}

	// Home city at (0,0), recipient city at (4,0), plains strip between for the route.
	mkCity := func(q int, owner uuid.UUID, name string) uuid.UUID {
		var provID uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, 0, 'plains') RETURNING id`,
			worldID, q,
		).Scan(&provID); err != nil {
			t.Fatalf("create province (%d,0): %v", q, err)
		}
		var settID uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
			 VALUES ($1, $2, $3, 'achaean', $4, 'capital', true) RETURNING id`,
			worldID, provID, name, owner,
		).Scan(&settID); err != nil {
			t.Fatalf("create settlement %s: %v", name, err)
		}
		return settID
	}
	homeID := mkCity(0, senderID, "Home")
	destID := mkCity(4, otherID, "Recipient")
	for q := 0; q <= 4; q++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'plains')`,
			worldID, q,
		); err != nil {
			t.Fatalf("create tile (%d,0): %v", q, err)
		}
	}

	// A replied courier halfway through its homeward (4,0)→(0,0) run.
	now := time.Now()
	if _, err := pool.Exec(ctx,
		`INSERT INTO messengers
		     (world_id, sender_id, origin_id, destination_id, message_text, reply_text, status, kind,
		      hex_q, hex_r, sent_at, return_departs_at, arrives_at)
		 VALUES ($1,$2,$3,$4,'Message.','Reply.','returning','diplomatic',4,0,$5,$6,$7)`,
		worldID, senderID, homeID, destID,
		now.Add(-90*time.Minute), now.Add(-30*time.Minute), now.Add(30*time.Minute),
	); err != nil {
		t.Fatalf("create returning messenger: %v", err)
	}

	eyes := loadLiveEyes(ctx, pool, worldID, senderID, now)
	foundMidRoute := false
	for _, e := range eyes {
		if e.Kind == province.EyeLandUnit && e.Pos.Q == 2 && e.Pos.R == 0 {
			foundMidRoute = true
		}
	}
	if !foundMidRoute {
		t.Fatalf("no land-unit eye at the returning courier's mid-route hex (2,0); eyes = %+v", eyes)
	}
}
