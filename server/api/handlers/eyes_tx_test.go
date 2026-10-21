package handlers

import (
	"context"
	"reflect"
	"testing"
	"time"

	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
)

func TestPlacementLiveEyesTransaction(t *testing.T) {
	for _, kind := range []string{"marching-unit", "outbound", "returning", "combined"} {
		t.Run(kind, func(t *testing.T) {
			terrain := map[[2]int]string{}
			for q := 1; q <= 8; q++ {
				terrain[[2]int{q, 0}] = "plains"
			}
			f := setupPlacementFixture(t, terrain)
			pool := p10TestPool(t)
			ctx := context.Background()
			var owner uuid.UUID
			if err := pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, f.settlementID).Scan(&owner); err != nil {
				t.Fatal(err)
			}
			now := time.Now()
			departs, arrives := now.Add(-time.Hour), now.Add(time.Hour)
			if kind == "marching-unit" || kind == "combined" {
				if _, err := pool.Exec(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,q,r,target_q,target_r,departs_at,arrives_at) VALUES($1,$2,'spearman','land',100,'marching',4,0,8,0,$3,$4)`, f.worldID, owner, departs, arrives); err != nil {
					t.Fatal(err)
				}
			}
			for _, status := range []string{"outbound", "returning"} {
				if kind != status && kind != "combined" {
					continue
				}
				if _, err := pool.Exec(ctx, `INSERT INTO messengers(world_id,sender_id,origin_id,dest_q,dest_r,message_text,status,hex_q,hex_r,sent_at,arrives_at,return_departs_at) VALUES($1,$2,$3,8,0,'eyes tx probe',$4,8,0,$5,$6,$5)`, f.worldID, owner, f.settlementID, status, departs, arrives); err != nil {
					t.Fatal(err)
				}
			}
			code, body := f.do(t, "POST", f.placementsPath(), map[string]any{"target_kind": "hex", "hex_q": 1, "hex_r": 0, "good_key": "grain"})
			if code != 201 {
				t.Fatalf("place with %s must succeed on one TX, got HTTP%d: %v", kind, code, body)
			}
			// Inspect the actual TX before checking the one-connection pool;
			// otherwise the old pool starvation would mask the TX regression.
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			fromTX := loadLiveEyes(ctx, tx, f.worldID, owner, now)
			var probe int
			if err = tx.QueryRow(ctx, `SELECT 1`).Scan(&probe); err != nil {
				t.Fatalf("eyes poisoned transaction: %v", err)
			}
			want := map[province.MapPosition]int{{Q: 4, R: 0}: 1}
			if kind == "marching-unit" {
				want = map[province.MapPosition]int{{Q: 6, R: 0}: 1}
			}
			if kind == "combined" {
				want = map[province.MapPosition]int{{Q: 6, R: 0}: 1, {Q: 4, R: 0}: 2}
			}
			actual := map[province.MapPosition]int{}
			for _, eye := range fromTX {
				if eye.Kind == province.EyeLandUnit {
					actual[eye.Pos]++
				}
			}
			if !reflect.DeepEqual(actual, want) {
				t.Fatalf("interpolated eyes differ: got%v want%v", actual, want)
			}
			if err = tx.Rollback(ctx); err != nil {
				t.Fatal(err)
			}
			cfg, err := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
			if err != nil {
				t.Fatal(err)
			}
			cfg.MaxConns = 1
			single, err := pgxpool.NewWithConfig(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer single.Close()
			limited, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			fromPool := loadLiveEyes(limited, single, f.worldID, owner, now)
			if err = limited.Err(); err != nil {
				t.Fatalf("one-connection pool starved: %v", err)
			}
			if !reflect.DeepEqual(fromPool, fromTX) {
				t.Fatalf("pool/TX vision differs: pool%+v tx%+v", fromPool, fromTX)
			}
			t.Logf("%s placement201, pool/TX same eyes and TX healthy", kind)
		})
	}
}
