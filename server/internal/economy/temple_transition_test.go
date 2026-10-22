package economy

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"strings"
	"testing"
)

func TestTempleTransitionIdleFirstThenNonFoodAndIdempotent(t *testing.T) {
	sid := recomputeFixture(t, 10, 500, 100, 0)
	pool := testPool(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	// Isolate the transition scope without changing the shared fixture database.
	if _, err = transitionSeed(ctx, tx, `DELETE FROM settlement_labor; UPDATE temple_staffing_transition SET completed=false; INSERT INTO buildings(settlement_id,building_type,level) VALUES($1,'temple',1); INSERT INTO settlement_labor(settlement_id,good_key,weight) VALUES($1,'cult',.45); INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) VALUES($1,1,'hex',0,0,'grain'),($1,2,'hex',0,0,'timber'),($1,3,'hex',0,0,'wine'),($1,4,'hex',0,0,'wine')`, sid); err != nil {
		t.Fatal(err)
	}
	if err = transitionTempleStaffing(ctx, tx); err != nil {
		t.Fatal(err)
	}
	var workers []int
	if err = tx.QueryRow(ctx, `SELECT array_agg(gubbe_ordinal ORDER BY gubbe_ordinal) FROM settlement_placement WHERE settlement_id=$1 AND good_key='cult'`, sid).Scan(&workers); err != nil {
		t.Fatal(err)
	}
	if len(workers) != 2 || workers[0] != 4 || workers[1] != 5 {
		t.Fatalf("want idle5 then wine4; got%v", workers)
	}
	var food string
	if err = tx.QueryRow(ctx, `SELECT good_key FROM settlement_placement WHERE settlement_id=$1 AND gubbe_ordinal=1`, sid).Scan(&food); err != nil || food != "grain" {
		t.Fatalf("stole food %q %v", food, err)
	}
	var table *string
	if err = tx.QueryRow(ctx, `SELECT to_regclass('settlement_labor')::text`).Scan(&table); err != nil || table != nil {
		t.Fatalf("legacy table remains %v %v", table, err)
	}
	if err = transitionTempleStaffing(ctx, tx); err != nil {
		t.Fatalf("replay %v", err)
	}
}
func TestTempleTransitionNeverStealsFood(t *testing.T) {
	sid := recomputeFixture(t, 10, 100, 100, 0)
	pool := testPool(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = transitionSeed(ctx, tx, `DELETE FROM settlement_labor; UPDATE temple_staffing_transition SET completed=false; INSERT INTO buildings(settlement_id,building_type,level) VALUES($1,'temple',1); INSERT INTO settlement_labor(settlement_id,good_key,weight) VALUES($1,'cult',.15); INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) VALUES($1,1,'hex',0,0,'grain')`, sid); err != nil {
		t.Fatal(err)
	}
	if err = transitionTempleStaffing(ctx, tx); err == nil {
		t.Fatal("transition should refuse food-only workforce")
	}
	var good string
	if err = tx.QueryRow(ctx, `SELECT good_key FROM settlement_placement WHERE settlement_id=$1`, sid).Scan(&good); err != nil || good != "grain" {
		t.Fatalf("changed food %q %v", good, err)
	}
}

func transitionSeed(ctx context.Context, tx pgx.Tx, sql string, sid uuid.UUID) (any, error) {
	for _, stmt := range strings.Split(sql, ";") {
		var err error
		if strings.Contains(stmt, "$1") {
			_, err = tx.Exec(ctx, stmt, sid)
		} else {
			_, err = tx.Exec(ctx, stmt)
		}
		if err != nil {
			return nil, err
		}
	}
	return nil, nil
}
