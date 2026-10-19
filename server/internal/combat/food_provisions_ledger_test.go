package combat

import (
	"context"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"os"
	"testing"
)

func TestProvisionMigrationBackfillsExistingGrainWithoutMinting(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSupportFixture(t, pool, "old-provisions")
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, q := range []string{`ALTER TABLE units DROP COLUMN provision_goods`, `ALTER TABLE settlements DROP COLUMN food_consumed`, `ALTER TABLE settlements DROP COLUMN food_consumed_tick`} {
		if _, err := tx.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	var id uuid.UUID
	if err := tx.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id,provisions) VALUES($1,$2,'galley','naval',1,20,'garrison',$3,42.5) RETURNING id`, f.worldID, f.owner, f.capitalID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../db/migrations/190_equal_food.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	goods, err := storedProvisions(ctx, tx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(goods) != 1 || goods["grain"] != 42.5 {
		t.Fatalf("backfilled %v", goods)
	}
}

func TestStoredProvisionsMismatchRefusesWithoutSilentConversion(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSupportFixture(t, pool, "bad-ledger")
	var id uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id,provisions,provision_goods) VALUES($1,$2,'galley','naval',1,20,'garrison',$3,10,'{"fish":9}') RETURNING id`, f.worldID, f.owner, f.capitalID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	h := NewUpkeepHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil, nil)
	if fed, err := h.eatProvisions(ctx, id, 1); fed || err == nil {
		t.Fatalf("fed=%v err=%v", fed, err)
	}
	var total float64
	var goods map[string]float64
	if err := pool.QueryRow(ctx, `SELECT provisions,provision_goods FROM units WHERE id=$1`, id).Scan(&total, &goods); err != nil {
		t.Fatal(err)
	}
	if total != 10 || goods["fish"] != 9 {
		t.Fatalf("failed draw mutated ledger %g/%v", total, goods)
	}
}
