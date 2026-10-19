package religion

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
	"os"
	"testing"
)

func TestSilverCalibrationLaborIndexNeverSmoothsHistoricalPrices(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE worlds SET status='archived' WHERE status='active'`); err != nil {
		t.Fatal(err)
	}
	var wid uuid.UUID
	if err := tx.QueryRow(ctx, `INSERT INTO worlds(name,status,current_tick) VALUES('labor-index-v2','active',5) RETURNING id`).Scan(&wid); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO divine_valuations(world_id,good_key,divine_value) VALUES($1,'grain',999999)`, wid); err != nil {
		t.Fatal(err)
	}
	if err := InitializeLaborValuations(ctx, tx, wid); err != nil {
		t.Fatal(err)
	}
	values, err := LoadDivineValues(ctx, tx, wid)
	if err != nil {
		t.Fatal(err)
	}
	base, err := LoadLaborValues(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]float64{"grain": 1, "fish": 1, "livestock": 1, "timber": 1, "cedar": 1, "copper": 1, "tin": 1, "silver": 1, "oil": 1, "wine": 1, "stone": 2, "bronze": 11, "incense": 4} {
		if base[key] != want {
			t.Errorf("%s labor_value=%g want%g", key, base[key], want)
		}
		if math.Abs(values[key]-want*1.7) > 1e-8 {
			t.Errorf("%s v2=%g want%g; historical price cannot seed smoothing", key, values[key], want*1.7)
		}
	}
	if len(values) != 13 {
		t.Fatalf("v2 values=%v: parked/non-goods must be excluded", values)
	}
	if got := OfferingWorth(map[string]float64{"silver": 2}, values, nil); math.Abs(got-3.4) > 1e-9 {
		t.Fatalf("silver offering=%g want3.4 worker_ticks, no floor", got)
	}
	var old float64
	if err := tx.QueryRow(ctx, `SELECT divine_value FROM divine_valuations WHERE world_id=$1 AND good_key='grain'`, wid).Scan(&old); err != nil {
		t.Fatal(err)
	}
	if old != 999999 {
		t.Fatalf("historical index rewritten: %g", old)
	}
	if _, err := tx.Exec(ctx, `UPDATE divine_labor_valuations SET divine_value=2 WHERE world_id=$1 AND good_key='grain'`, wid); err != nil {
		t.Fatal(err)
	}
	if err := InitializeLaborValuations(ctx, tx, wid); err != nil {
		t.Fatal(err)
	}
	v, _ := LoadDivineValues(ctx, tx, wid)
	if v["grain"] != 2 {
		t.Fatal("restart initialized an existing v2 history twice")
	}
	if err := RecomputeDivineValuations(ctx, tx, wid); err != nil {
		t.Fatal(err)
	}
	v, _ = LoadDivineValues(ctx, tx, wid)
	if math.Abs(v["grain"]-1.898) > 1e-9 {
		t.Fatalf("v2 smoothing=%g want1.898", v["grain"])
	}
}
