package economy

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool connects to a real Postgres instance for integration tests that
// exercise real SQL (ON CONFLICT upserts, recency gates) a mock can't stand
// in for. Skips (not fails) when DATABASE_URL isn't set, so `go test ./...`
// stays green in environments without a database.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// gossipTestWorld creates a throwaway world + N players for a contact-gossip
// test and registers cleanup in FK-safe order (world cascades provinces/
// settlements; players are deleted last since market_snapshots/gossip_events
// cascade from them).
func gossipTestWorld(t *testing.T, pool *pgxpool.Pool, playerNames ...string) (worldID uuid.UUID, playerIDs []uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'archived') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}

	for _, name := range playerNames {
		var pid uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
			name+"-"+uuid.New().String(), name+"-"+uuid.New().String()+"@test.invalid",
		).Scan(&pid); err != nil {
			t.Fatalf("create test player %s: %v", name, err)
		}
		playerIDs = append(playerIDs, pid)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM player_scouted_provinces WHERE world_id = $1`, worldID)
		_, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID)
		for _, pid := range playerIDs {
			_, _ = pool.Exec(ctx, `DELETE FROM players WHERE id = $1`, pid)
		}
	})
	return worldID, playerIDs
}

// gossipTestSettlement creates a province + settlement owned by ownerID.
func gossipTestSettlement(t *testing.T, pool *pgxpool.Pool, worldID, ownerID uuid.UUID, q, r int, name string) (settlementID, provinceID uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, $3, 'plains') RETURNING id`,
		worldID, q, r,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create test province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, $3, 'achaean', $4, 'capital', true) RETURNING id`,
		worldID, provinceID, name, ownerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create test settlement: %v", err)
	}
	return settlementID, provinceID
}

// TestPropagateMarketKnowledge_ContactSpread verifies mechanism 1: A's
// messenger reaching B's settlement teaches A about C's market (which B
// already knew about), seeds A's map memory with C's province, and never
// overwrites a fresher firsthand snapshot A already has.
func TestPropagateMarketKnowledge_ContactSpread(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	worldID, players := gossipTestWorld(t, pool, "learner-a", "teacher-b", "owner-c")
	learnerA, teacherB, ownerC := players[0], players[1], players[2]

	settlementB, _ := gossipTestSettlement(t, pool, worldID, teacherB, 1, 1, "Teacherton")
	settlementC, provinceC := gossipTestSettlement(t, pool, worldID, ownerC, 5, 5, "Farland")

	// B already observed C's market directly (firsthand).
	teacherObservedAt := time.Now().Add(-time.Hour)
	if _, err := pool.Exec(ctx,
		`INSERT INTO market_snapshots (player_id, settlement_id, good_key, stock, price, observed_at, secondhand)
		 VALUES ($1, $2, 'grain', 10, 5, $3, false)`,
		teacherB, settlementC, teacherObservedAt,
	); err != nil {
		t.Fatalf("seed teacher snapshot: %v", err)
	}

	// A's messenger reaches B's settlement.
	if err := PropagateMarketKnowledge(ctx, pool, learnerA, settlementB); err != nil {
		t.Fatalf("PropagateMarketKnowledge: %v", err)
	}

	var stock, price float64
	var secondhand bool
	var observedAt time.Time
	if err := pool.QueryRow(ctx,
		`SELECT stock, price, secondhand, observed_at FROM market_snapshots
		 WHERE player_id = $1 AND settlement_id = $2 AND good_key = 'grain'`,
		learnerA, settlementC,
	).Scan(&stock, &price, &secondhand, &observedAt); err != nil {
		t.Fatalf("expected secondhand snapshot for learner: %v", err)
	}
	if !secondhand {
		t.Errorf("expected secondhand=true, got false")
	}
	if stock != 10 || price != 5 {
		t.Errorf("expected copied stock=10 price=5, got stock=%v price=%v", stock, price)
	}
	if diff := observedAt.Sub(teacherObservedAt); diff > time.Millisecond || diff < -time.Millisecond {
		t.Errorf("expected teacher's observed_at %v preserved, got %v", teacherObservedAt, observedAt)
	}

	var scoutedCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM player_scouted_provinces WHERE world_id = $1 AND player_id = $2 AND province_id = $3`,
		worldID, learnerA, provinceC,
	).Scan(&scoutedCount); err != nil {
		t.Fatalf("check scouted province: %v", err)
	}
	if scoutedCount != 1 {
		t.Errorf("expected C's province seeded into learner's map memory, got count=%d", scoutedCount)
	}

	// Now A has its own fresher firsthand snapshot of C — it must not be
	// clobbered by a later (older) secondhand propagation.
	freshFirsthand := time.Now()
	if _, err := pool.Exec(ctx,
		`UPDATE market_snapshots SET price = 999, secondhand = false, observed_at = $3
		 WHERE player_id = $1 AND settlement_id = $2 AND good_key = 'grain'`,
		learnerA, settlementC, freshFirsthand,
	); err != nil {
		t.Fatalf("seed learner firsthand snapshot: %v", err)
	}

	if err := PropagateMarketKnowledge(ctx, pool, learnerA, settlementB); err != nil {
		t.Fatalf("PropagateMarketKnowledge (2nd contact): %v", err)
	}

	if err := pool.QueryRow(ctx,
		`SELECT price, secondhand FROM market_snapshots
		 WHERE player_id = $1 AND settlement_id = $2 AND good_key = 'grain'`,
		learnerA, settlementC,
	).Scan(&price, &secondhand); err != nil {
		t.Fatalf("re-check snapshot: %v", err)
	}
	if secondhand || price != 999 {
		t.Errorf("firsthand snapshot must survive a secondhand propagation, got price=%v secondhand=%v", price, secondhand)
	}
}
