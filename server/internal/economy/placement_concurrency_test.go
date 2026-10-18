package economy

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestPlacementGrowthConcurrentRespectsCapacity(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	sid := seedFullRingFixture(t, 100, 800, "plains")
	const key = 982736451
	blocker, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Release()
	if _, err = blocker.Exec(ctx, `SELECT pg_advisory_lock($1)`, int64(key)); err != nil {
		t.Fatal(err)
	}
	defer blocker.Exec(ctx, `SELECT pg_advisory_unlock_all()`)
	// Fixture-only insert barrier: hold every old-code INSERT after its stale
	// capacity read. Fixed code reaches it once; the others wait on the city.
	fn := fmt.Sprintf(`CREATE FUNCTION placement_test_barrier() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.settlement_id='%s'::uuid THEN PERFORM pg_advisory_xact_lock(%d); END IF; RETURN NEW; END $$; CREATE TRIGGER placement_test_barrier BEFORE INSERT ON settlement_placement FOR EACH ROW EXECUTE FUNCTION placement_test_barrier()`, sid, key)
	if _, err = pool.Exec(ctx, fn); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = blocker.Exec(ctx, `SELECT pg_advisory_unlock_all()`)
		_, _ = pool.Exec(ctx, `DROP TRIGGER placement_test_barrier ON settlement_placement;DROP FUNCTION placement_test_barrier()`)
	}()
	const n = 8
	results := make(chan error, n)
	start := make(chan struct{})
	for ordinal := 1; ordinal <= n; ordinal++ {
		go func(o int) {
			<-start
			placed, err := PlaceNextGubbeOnBestFoodHex(ctx, pool, sid, o)
			if err == nil && !placed {
				err = fmt.Errorf("worker%d unexpectedly idle", o)
			}
			results <- err
		}(ordinal)
	}
	close(start)
	deadline := time.Now().Add(8 * time.Second)
	for {
		var count int
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock'`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count >= n {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("only%d/%d reached DB barrier", count, n)
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, _ = blocker.Exec(ctx, `SELECT pg_advisory_unlock_all()`)
	for i := 0; i < n; i++ {
		select {
		case err := <-results:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(8 * time.Second):
			t.Fatal("growth placement deadlock")
		}
	}
	var max, count int
	if err = pool.QueryRow(ctx, `SELECT COALESCE(max(n),0),COALESCE(sum(n),0) FROM (SELECT count(*) n FROM settlement_placement WHERE settlement_id=$1 GROUP BY hex_q,hex_r,good_key) x`, sid).Scan(&max, &count); err != nil {
		t.Fatal(err)
	}
	if max > 4 || count != n {
		t.Fatalf("concurrent growth exceeded real hex cap: max%d count%d", max, count)
	}
	if placed, err := PlaceNextGubbeOnBestFoodHex(ctx, pool, sid, 9); err != nil || placed {
		t.Fatalf("stale future ordinal accepted %v %v", placed, err)
	}
}

func TestPlacementGrowthRejectsStaleOrdinalAndFullPool(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	sid := seedFullRingFixture(t, 100, 100, "plains")
	if placed, err := PlaceNextGubbeOnBestFoodHex(ctx, pool, sid, 2); err != nil || placed {
		t.Fatalf("future ordinal2 at population100: %v %v", placed, err)
	}
	// A survivor above the current ordinal range can remain until pruning.
	// Filling a lower hole must still respect total population, not just ordinal.
	if _, err := pool.Exec(ctx, `INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) VALUES($1,2,'hex',1,0,'grain')`, sid); err != nil {
		t.Fatal(err)
	}
	if placed, err := PlaceNextGubbeOnBestFoodHex(ctx, pool, sid, 1); err != nil || placed {
		t.Fatalf("already full population pool: %v %v", placed, err)
	}
}
