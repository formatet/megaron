package ai

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The passive hook used to consume silver on every tick without
// producing any action or outcome, and consumed it again on worker replay.
func TestPassiveGovernorTickPreservesStockWithoutAction(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE worlds SET status = 'archived' WHERE status = 'active'`)
	world, owner := uuid.New(), uuid.New()
	exec(`INSERT INTO worlds (id, name, status, current_tick) VALUES ($1, $2, 'active', 50)`, world, "governor-"+world.String())
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE worlds SET status = 'archived' WHERE id = $1`, world)
	})
	exec(`INSERT INTO players (id, username, password_hash) VALUES ($1, $2, 'x')`, owner, "governor-"+owner.String())
	for i, tc := range []struct {
		name   string
		ai     bool
		amount float64
		rate   float64
		stock  bool
	}{
		{"ai-low", true, 100, 0, true},
		{"ai-lowest-lazy", true, 10, 1, true},
		{"ai-threshold", true, 20, 0, true},
		{"ai-poor", true, 19, 0, true},
		{"ai-loyal", true, 100, 0, true},
		{"human-low", false, 100, 0, true},
		{"ai-no-stock", true, 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			province, city := uuid.New(), uuid.New()
			exec(`INSERT INTO provinces (id, world_id, map_q, map_r, terrain_type) VALUES ($1, $2, $3, 0, 'plains')`, province, world, i)
			exec(`INSERT INTO settlements (id, world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population, governor_is_ai)
			 VALUES ($1,$2,$3,'Governor test','achaean',$4,'colony',false,'active',1000,$5)`, city, world, province, owner, tc.ai)
			if tc.stock {
				exec(`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick) VALUES ($1,'silver',$2,$3,1000,40)`, city, tc.amount, tc.rate)
			}
			snapshot := func() string {
				t.Helper()
				var result string
				if err := pool.QueryRow(ctx, `SELECT jsonb_build_object(
				 'stock', (SELECT to_jsonb(g) FROM settlement_goods g WHERE settlement_id=$1 AND good_key='silver'),
				 'events', (SELECT count(*) FROM events WHERE world_id=$2))::text
				 FROM settlements s WHERE id=$1`, city, world).Scan(&result); err != nil {
					t.Fatal(err)
				}
				return result
			}
			before := snapshot()
			for replay := 0; replay < 3; replay++ {
				if err := PassiveGovernorTick(ctx, pool, city, world); err != nil {
					t.Fatal(err)
				}
				if after := snapshot(); after != before {
					t.Fatalf("pass %d changed stock/events without an action:\nbefore %s\nafter  %s", replay+1, before, after)
				}
			}
		})
	}
	if err := PassiveGovernorTick(ctx, pool, uuid.New(), world); err == nil {
		t.Fatal("missing settlement must still return an error")
	}
}
