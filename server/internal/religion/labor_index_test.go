package religion

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"testing"
)

func TestFixedDivineValuesCoverActiveGoodsAndIgnoreTheWorld(t *testing.T) {
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
	base, err := LoadLaborValues(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	for key := range base {
		if FixedDivineValues[key] <= 0 {
			t.Errorf("active good %s has no fixed divine value", key)
		}
	}
	for key := range FixedDivineValues {
		if _, ok := base[key]; !ok {
			t.Errorf("fixed divine value for %s, which is not an active good", key)
		}
	}
	var wid uuid.UUID
	if err := tx.QueryRow(ctx, `INSERT INTO worlds(name,status,current_tick) VALUES('fixed-divine','archived',5) RETURNING id`).Scan(&wid); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO divine_labor_valuations(world_id,good_key,rarity_spread,rarity_volume,divine_value,calc_tick) VALUES($1,'grain',1,1,999,5)`, wid); err != nil {
		t.Fatal(err)
	}
	values, err := LoadDivineValues(ctx, tx, wid)
	if err != nil {
		t.Fatal(err)
	}
	if values["grain"] != 1 || values["bronze"] != 33 {
		t.Fatalf("world scarcity leaked into the fixed list: grain=%g bronze=%g", values["grain"], values["bronze"])
	}
	values["grain"] = 7
	if FixedDivineValues["grain"] != 1 {
		t.Fatal("a caller's map must not write through to the fixed list")
	}
}
